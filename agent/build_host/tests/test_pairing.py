import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from common import host

class FakeBoard:
    def __init__(self):
        self.calls = []
        self.statuses = iter([{'status':'pending'}, {'status':'pending'}, {'status':'approved'}])
    def request(self, method, path, body=None, **kw):
        self.calls.append((method, path, body))
        return {'id':9} if path.endswith('/pair') else next(self.statuses)

class PairingTests(unittest.TestCase):
    def test_waits_and_preserves_identity(self):
        with tempfile.TemporaryDirectory() as tmp:
            state = host.State(Path(tmp) / 'state')
            board = FakeBoard()
            sleeps = []
            host.pair(state, board, 'SAMPLE-CODE', 'test', {}, sleep=sleeps.append)
            self.assertEqual(sleeps, [5,5])
            self.assertEqual(state.load()['host_id'], 9)
            self.assertNotIn('SAMPLE-CODE', ''.join(p.read_text(errors='ignore') for p in state.root.iterdir() if p.is_file()))
            seed = state.key().encode()
            board.statuses = iter([{'status':'approved'}])
            host.pair(state, board, 'unused', 'test', {}, sleep=sleeps.append)
            self.assertEqual(state.key().encode(), seed)
            self.assertEqual(sum(p.endswith('/pair') for _,p,_ in board.calls), 1)

    def test_lost_pair_is_never_automatically_retried(self):
        with tempfile.TemporaryDirectory() as tmp:
            state = host.State(Path(tmp) / 'state')
            board = FakeBoard()
            board.request = lambda *a, **k: (_ for _ in ()).throw(OSError('lost response'))
            with self.assertRaises(OSError):
                host.pair(state, board, 'code', 'test', {})
            board = FakeBoard()
            with self.assertRaisesRegex(RuntimeError, 'ambiguous'):
                host.pair(state, board, 'new-code', 'test', {})
            self.assertEqual(board.calls, [])

    @unittest.skipIf(os.name == 'nt', 'POSIX modes')
    def test_private_modes_symlinks_and_lock(self):
        with tempfile.TemporaryDirectory() as tmp:
            state = host.State(Path(tmp) / 'state')
            state.save({'host_id':1})
            state.create_key()
            self.assertEqual(state.root.stat().st_mode & 0o777, 0o700)
            self.assertEqual(state.config.stat().st_mode & 0o777, 0o600)
            with state.lock():
                with self.assertRaisesRegex(RuntimeError, 'instance'):
                    with state.lock():
                        pass
            state.seed.chmod(0o644)
            with self.assertRaises(PermissionError):
                state.key()
            state.seed.unlink()
            state.seed.symlink_to(state.config)
            with self.assertRaises(PermissionError):
                state.key()

    def test_refuses_privileged_before_creating_files(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(host, 'is_elevated', return_value=True):
            with self.assertRaisesRegex(PermissionError, 'elevated'):
                host.main(['run', '--config', str(Path(tmp)/'never/config.json')])
            self.assertEqual(list(Path(tmp).iterdir()), [])
