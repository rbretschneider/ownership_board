import ctypes
import os
from pathlib import Path
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch
from common import host

@unittest.skipUnless(os.name == 'nt', 'Windows native APIs')
class WindowsNativeTests(unittest.TestCase):
    def test_job_priority_and_failed_assignment_cannot_execute(self):
        with tempfile.TemporaryDirectory() as tmp:
            marker=Path(tmp)/'started'
            with host.ProcessTree(threading.Event()) as tree:
                with patch.object(tree.job,'assign',side_effect=OSError('fixture assignment denial')):
                    with self.assertRaises(OSError):
                        tree.run([sys.executable,'-c','import pathlib,sys; pathlib.Path(sys.argv[1]).touch()',str(marker)],Path(tmp),dict(os.environ))
            self.assertFalse(marker.exists())
            with host.ProcessTree(threading.Event()) as tree:
                code,log=tree.run([sys.executable,'-c','import ctypes; k=ctypes.WinDLL("kernel32"); k.GetCurrentProcess.restype=ctypes.c_void_p; k.GetPriorityClass.argtypes=[ctypes.c_void_p]; print(k.GetPriorityClass(k.GetCurrentProcess()))'],Path(tmp),dict(os.environ))
                self.assertEqual(tree.job.active_count(),0)
                handle=tree.job.handle
            self.assertEqual(code,0)
            self.assertEqual(log.strip(),str(0x4000))
            flags=ctypes.c_ulong()
            inspect=host.win_function(host.K32,'GetHandleInformation',host.wt.BOOL,host.wt.HANDLE,ctypes.POINTER(host.wt.DWORD))
            self.assertFalse(inspect(handle,ctypes.byref(flags)))

    def test_private_dacl_and_token_probe(self):
        with tempfile.TemporaryDirectory() as tmp:
            state=host.State(Path(tmp)/'identity')
            state.save({'host_id':1})
            state.create_key()
            host.check_private(state.root,directory=True)
            host.check_private(state.seed)
            self.assertEqual(len(state.key().encode()),32)
            self.assertIsInstance(host.is_elevated(),bool)
        with patch.object(host,'is_elevated',return_value=True), self.assertRaises(PermissionError):
            host.main(['run'])

    def test_desktop_lock_excludes_a_powershell_script_holding_the_same_file(self):
        # The legacy contract: a script given OWNERSHIP_DESKTOP_LOCK takes
        # FileStream.Lock(0,1) around its UI tests; the agent must wait for it.
        import subprocess
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            path=root/'desktop.lock'
            host.FileLock(path).create().close()
            ready=root/'ready'
            script=root/'hold.ps1'
            script.write_text('param($Lock,$Ready)\n$fs=[IO.File]::Open($Lock,"OpenOrCreate","ReadWrite","ReadWrite"); $fs.Lock(0,1)\nSet-Content -LiteralPath $Ready x\nStart-Sleep -Seconds 30\n')
            holder=subprocess.Popen(['powershell.exe','-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',str(script),str(path),str(ready)])
            try:
                deadline=time.monotonic()+20
                while not ready.exists() and time.monotonic()<deadline: time.sleep(.05)
                self.assertTrue(ready.exists())
                stop=threading.Event()
                timer=threading.Timer(.3,stop.set);timer.start()
                with self.assertRaises(host.Cancelled):
                    with host.HostLock(stop,path):
                        self.fail('lock admitted overlapping desktop execution')
                timer.join()
            finally:
                holder.kill();holder.wait()
            with host.HostLock(threading.Event(),path):
                pass
