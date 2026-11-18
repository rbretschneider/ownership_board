import json
import os
from pathlib import Path
import tempfile
import unittest
from common import host
from test_execution import BundleBoard, SourceRepo, scripts


class TestListTests(unittest.TestCase):
    """The job's quarantine and only-tests lists reach the script as files."""
    def setUp(self):
        if os.name == 'nt':
            self.skipTest('POSIX fixture')

    def run_job(self, **extra):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            sh = ('printf "q=%s\\n" "$(cat "$OWNERSHIP_QUARANTINE_FILE" | tr "\\n" "|")"\n'
                  'printf "only=%s\\n" "${OWNERSHIP_ONLY_TESTS_FILE:-none}"\n'
                  'if [ -n "$OWNERSHIP_ONLY_TESTS_FILE" ]; then printf "o=%s\\n" "$(cat "$OWNERSHIP_ONLY_TESTS_FILE" | tr "\\n" "|")"; fi\n'
                  'printf "kind=%s\\n" "${OWNERSHIP_JOB_KIND:-commit}"\n')
            raw, job = SourceRepo(root).commit(scripts(sh=sh))
            job.update(extra)
            config = {'build_root':str(root/'builds'),'cache_root':str(root/'cache'),'host_id':7,'max_jobs':1}
            host.private_dir(root/'builds')
            host.private_dir(root/'cache')
            runner = host.JobRunner(BundleBoard(raw), config, host.Assignment(host.validate_job(job,7),None))
            payload = json.loads(runner.execute())
            self.assertEqual(payload['status'],'passed',payload)
            return payload['result']['commands'][-1]['log']

    def test_quarantine_file_always_written(self):
        log = self.run_job()
        self.assertIn('q=\n', log)
        self.assertIn('only=none', log)
        self.assertIn('kind=commit', log)
        log = self.run_job(quarantine=['suite:a b', 'suite:c'])
        self.assertIn('q=suite:a b|suite:c|', log)

    def test_flake_check_lists_only_its_tests(self):
        log = self.run_job(kind='flake_check', only_tests=['suite:flaky'], quarantine=[])
        self.assertIn('o=suite:flaky|', log)
        self.assertIn('kind=flake_check', log)

    def test_validation_rejects_bad_lists(self):
        with tempfile.TemporaryDirectory() as tmp:
            _, job = SourceRepo(Path(tmp)).commit(scripts())
            for patch in ({'quarantine':'x'}, {'quarantine':['a\nb']}, {'only_tests':[' a']}, {'only_tests':['x'*301]},
                          {'quarantine':['a']*201}, {'kind':'other'}):
                with self.subTest(patch=patch):
                    with self.assertRaises(ValueError):
                        host.validate_job(dict(job, **patch),7)


if __name__ == '__main__':
    unittest.main()
