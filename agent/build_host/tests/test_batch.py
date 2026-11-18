import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from common import host
from test_execution import SourceRepo, scripts


class JobBundles:
    """Serves each batch member's bundle by its job id, as the board does."""
    def __init__(self, bundles):
        self.bundles = bundles
        self.paths = []
    def open(self, method, path, *args, **kwargs):
        self.paths.append(path)
        return contextlib.closing(io.BytesIO(self.bundles[path.split('/')[-2]]))


class BatchTests(unittest.TestCase):
    def setUp(self):
        if os.name == 'nt':
            self.skipTest('POSIX fixture')
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.repo = SourceRepo(self.root)
        # The script fails unless every merged member's file is present.
        self.repo.commit(scripts(sh='for f in $(cat expect.txt); do test -f "$f" || { echo "missing $f"; exit 4; }; done\nprintf "verified\n"\n'))
        self.repo.git('branch','-M','main')
        self.bundles = {}

    def tearDown(self):
        self.tmp.cleanup()

    def branch(self, name, files, number):
        self.repo.git('checkout','-q','-b',name,'main')
        raw, job = self.repo.commit(files)
        job['id'] = format(number,'x')*64
        self.bundles[job['id']] = raw
        return {'id':job['id'],'ticket_id':number,'sha':job['sha'],'bundle_bytes':job['bundle_bytes'],'bundle_sha256':job['bundle_sha256']}

    def run_batch(self, members):
        lead = members[0]
        job = {'id':'f'*64,'attempt':1,'host_id':7,'repo':'acme/app','sha':lead['sha'],'bundle_bytes':lead['bundle_bytes'],
               'bundle_sha256':lead['bundle_sha256'],'preset':'script','target':{'os':host.OS,'required_capabilities':[]},
               'batch':{'members':members}}
        config = {'build_root':str(self.root/'builds'),'cache_root':str(self.root/'cache'),'host_id':7,'max_jobs':1}
        host.private_dir(self.root/'builds')
        host.private_dir(self.root/'cache')
        board = JobBundles(self.bundles)
        runner = host.JobRunner(board, config, host.Assignment(host.validate_job(job,7),None))
        return runner, board, json.loads(runner.execute())

    def test_members_merge_in_order_and_a_conflict_is_left_out(self):
        a = self.branch('a', {'a.txt':'a', 'shared.txt':'from a', 'expect.txt':'a.txt\n'}, 1)
        b = self.branch('b', {'b.txt':'b'}, 2)
        c = self.branch('c', {'shared.txt':'from c'}, 3)
        runner, board, payload = self.run_batch([a, b, c])
        self.assertEqual(payload['status'],'passed',payload)
        self.assertEqual(payload['result']['excluded'],[c['id']])
        # Every bundle came by its member's own job id; the batch id is never fetched.
        self.assertEqual(sorted(p.split('/')[-2] for p in board.paths), sorted([a['id'],b['id'],c['id']]))
        head = runner.checkout
        merged = subprocess.check_output(['git','-C',str(head),'log','--format=%P','-n','1']).decode().split()
        self.assertIn(b['sha'], merged)
        self.assertTrue((head/'b.txt').exists())
        self.assertEqual((head/'shared.txt').read_text(),'from a')
        labels = [cmd['label'] for cmd in payload['result']['commands']]
        self.assertIn('merge #2 '+b['sha'][:12], labels)
        self.assertTrue(all(cmd['exit_code'] == 0 for cmd in payload['result']['commands']))

    def test_a_red_candidate_fails_with_no_exclusions(self):
        a = self.branch('a', {'a.txt':'a', 'expect.txt':'a.txt\nb.txt\nnever.txt\n'}, 1)
        b = self.branch('b', {'b.txt':'b'}, 2)
        _, _, payload = self.run_batch([a, b])
        self.assertEqual(payload['status'],'failed')
        self.assertNotIn('excluded',payload['result'])
        self.assertNotIn('failure_kind',payload['result'])
        self.assertEqual(payload['result']['commands'][-1]['exit_code'],4)

    def test_validation_rejects_malformed_batches(self):
        a = self.branch('a', {'a.txt':'a'}, 1)
        b = self.branch('b', {'b.txt':'b'}, 2)
        base = {'id':'f'*64,'attempt':1,'host_id':7,'repo':'acme/app','sha':a['sha'],'bundle_bytes':a['bundle_bytes'],
                'bundle_sha256':a['bundle_sha256'],'preset':'script','target':{'os':host.OS,'required_capabilities':[]}}
        for members in ([a], [b, a], [a, dict(b, id='../x')], [a, dict(b, sha='HEAD')], [a, a], [a]+[dict(b, id=format(i,'x')*64) for i in range(2,10)]):
            with self.subTest(members=len(members)):
                with self.assertRaises(ValueError):
                    host.validate_job(dict(base, batch={'members':members}),7)
        self.assertEqual(host.validate_job(dict(base, batch={'members':[a,b]}),7)['batch']['members'][1]['ticket_id'],2)

    def test_report_lists_exclusions_after_failure_kind(self):
        raw = host.completion_bytes(1,[{'label':'b','exit_code':0,'log':''}],'',['a'*64])
        self.assertEqual(json.loads(raw)['result']['excluded'],['a'*64])


if __name__ == '__main__':
    unittest.main()
