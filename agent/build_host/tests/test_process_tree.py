import os
import ctypes
import signal
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path
from common import host


def alive(pid):
    if os.name == 'nt':
        open_process=host.win_function(host.K32,'OpenProcess',host.wt.HANDLE,host.wt.DWORD,host.wt.BOOL,host.wt.DWORD)
        exit_code=host.win_function(host.K32,'GetExitCodeProcess',host.wt.BOOL,host.wt.HANDLE,ctypes.POINTER(host.wt.DWORD))
        handle = open_process(0x1000, False, pid)
        if not handle:
            return False
        try:
            code = host.ctypes.c_ulong()
            if not exit_code(handle, host.ctypes.byref(code)):
                raise ctypes.WinError(ctypes.get_last_error())
            return code.value == 259
        finally:
            host.close_handle(handle)
    try:
        os.kill(pid, 0)
        path = Path(f'/proc/{pid}/stat')
        return not path.exists() or path.read_text().split(')')[1].strip()[0] != 'Z'
    except ProcessLookupError:
        return False


class ProcessTreeTests(unittest.TestCase):
    def test_real_tree_cancellation_with_term_ignoring_descendant(self):
        with tempfile.TemporaryDirectory() as tmp:
            pidfile = Path(tmp)/'pids'
            event = threading.Event()
            tree = host.ProcessTree(event)
            fixture = Path(__file__).parent/'fixtures/tree.py'
            result = []
            t = threading.Thread(target=lambda: result.append(tree.run([sys.executable,str(fixture),str(pidfile)], Path(tmp), dict(os.environ))))
            t.start()
            deadline = time.monotonic()+12
            while (not pidfile.exists() or len(pidfile.read_text().splitlines())<3) and time.monotonic()<deadline:
                time.sleep(.05)
            try:
                self.assertTrue(pidfile.exists(), 'tree never started')
                pids = [int(p) for p in pidfile.read_text().splitlines()]
                self.assertEqual(len(pids), 3)
                start = time.monotonic()
                event.set()
                t.join(8)
                self.assertFalse(t.is_alive())
                self.assertLess(time.monotonic()-start, 8)
                self.assertNotEqual(result[0][0], 0)
                self.assertFalse(any(alive(p) for p in pids), pids)
            finally:
                event.set()
                t.join(10)
                tree.close()

    @unittest.skipIf(os.name == 'nt', 'POSIX guardian')
    def test_parent_exit_does_not_leave_child(self):
        with tempfile.TemporaryDirectory() as tmp:
            pidfile = Path(tmp)/'pid'
            program = 'import subprocess,sys; p=subprocess.Popen([sys.executable,"-c","import time,signal; signal.signal(signal.SIGTERM,signal.SIG_IGN); time.sleep(90)"]); open(sys.argv[1],"w").write(str(p.pid))'
            with host.ProcessTree(threading.Event()) as tree:
                code, _ = tree.run([sys.executable,'-c',program,str(pidfile)], Path(tmp), dict(os.environ))
            self.assertEqual(code, 0)
            self.assertFalse(alive(int(pidfile.read_text())))

    def test_cancel_before_spawn_never_executes(self):
        event = threading.Event()
        event.set()
        with host.ProcessTree(event) as tree, self.assertRaises(host.Cancelled):
            tree.run([sys.executable,'-c','raise SystemExit(0)'], Path.cwd(), dict(os.environ))

    def test_large_output_is_drained_and_bounded(self):
        with host.ProcessTree(threading.Event()) as tree:
            code, log = tree.run([sys.executable,'-c','import sys; sys.stdout.buffer.write(("🌍\\\\\\\""*300000).encode("utf-8"))'], Path.cwd(), dict(os.environ))
        self.assertEqual(code, 0)
        self.assertLessEqual(len(log.encode()), 16384)
        self.assertIn('truncated', log)


class FakeTreeTests(unittest.TestCase):
    def test_stop_acknowledgment_after_tree_cleanup(self):
        events = []
        class FakeTree:
            def cancel(self):
                events.append('dead')
        class Board:
            def request(self, method, path, body):
                events.append(body['phase'])
                return {'stop':True, 'attempt':1}
        assignment = host.Assignment({'id':'a'*64,'attempt':1}, None)
        assignment.tree = FakeTree()
        host.poll_assignment(Board(), assignment)
        self.assertTrue(assignment.cancel.is_set())
        self.assertEqual(events, ['preparing','dead'])
        # Acknowledgment belongs to the controller after worker termination.
        self.assertNotIn('stopped', events)

class GuardianLifetimeTests(unittest.TestCase):
    @unittest.skipIf(os.name=='nt','POSIX guardian pipe')
    def test_controller_death_cleans_workload(self):
        import subprocess
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            pidfile=root/'pids'
            script=root/'owner.py'
            script.write_text('import sys,threading,os\nfrom pathlib import Path\nsys.path.insert(0,sys.argv[1])\nimport build_host as h\nt=h.ProcessTree(threading.Event())\nt.run([sys.executable,sys.argv[2],sys.argv[3]],Path.cwd(),dict(os.environ))\n')
            fixture=Path(__file__).parent/'fixtures/tree.py'
            owner=subprocess.Popen([sys.executable,str(script),str(Path(host.__file__).parent),str(fixture),str(pidfile)])
            try:
                deadline=time.monotonic()+10
                while (not pidfile.exists() or len(pidfile.read_text().splitlines())<3) and time.monotonic()<deadline: time.sleep(.05)
                self.assertTrue(pidfile.exists())
                pids=[int(p) for p in pidfile.read_text().splitlines()]
                self.assertEqual(len(pids),3)
                owner.kill();owner.wait()
                deadline=time.monotonic()+8
                while any(alive(p) for p in pids) and time.monotonic()<deadline: time.sleep(.05)
                self.assertFalse(any(alive(p) for p in pids))
            finally:
                if owner.poll() is None: owner.kill();owner.wait()

class DesktopCoordinationTests(unittest.TestCase):
    def test_lock_serializes_two_processes_and_releases_on_exit(self):
        import subprocess
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            ready,entered=root/'ready',root/'entered'
            lockpath=root/'desktop.lock'
            code='import sys,threading; from pathlib import Path; sys.path.insert(0,sys.argv[1]); import build_host as h; Path(sys.argv[3]).touch(); lock=h.HostLock(threading.Event(),Path(sys.argv[2])); lock.__enter__(); Path(sys.argv[4]).touch(); lock.close()'
            with host.HostLock(threading.Event(),lockpath):
                child=subprocess.Popen([sys.executable,'-c',code,str(Path(host.__file__).parent),str(lockpath),str(ready),str(entered)])
                try:
                    deadline=time.monotonic()+8
                    while not ready.exists() and time.monotonic()<deadline: time.sleep(.05)
                    self.assertTrue(ready.exists())
                    time.sleep(.2)
                    self.assertFalse(entered.exists())
                except BaseException:
                    child.kill();child.wait();raise
            try:
                self.assertEqual(child.wait(timeout=5),0)
                self.assertTrue(entered.exists())
            finally:
                if child.poll() is None: child.kill();child.wait()

    def test_desktop_wait_is_cancellable(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'desktop.lock'
            with host.HostLock(threading.Event(),path):
                event=threading.Event()
                timer=threading.Timer(.2,event.set);timer.start()
                try:
                    with self.assertRaises(host.Cancelled):
                        with host.HostLock(event,path): self.fail('overlapping execution')
                finally:
                    timer.join()
