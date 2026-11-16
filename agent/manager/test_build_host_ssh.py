import json
import subprocess
import tempfile
import unittest
from pathlib import Path

import build_host_ssh as ssh


class BuildHostSSHTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.key = Path(self.dir.name) / 'id_build'
        self.key.write_text('not a real key')
        self.log = Path(self.dir.name) / 'ssh.log'

    def tearDown(self):
        self.dir.cleanup()

    def env(self, **overrides):
        values = {ssh.HOST_VAR: 'build-1', ssh.USER_VAR: 'builder', ssh.KEY_VAR: str(self.key), ssh.LOG_VAR: str(self.log)}
        values.update(overrides)
        return values

    def run_command(self, command, env):
        calls = []

        def runner(argv, timeout):
            calls.append(argv)
            return subprocess.CompletedProcess(argv, 0)
        return ssh.run(command, env=env, runner=runner), calls

    def log_records(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_unset_means_repair_is_off(self):
        self.assertIsNone(ssh.build_host_access({}))
        self.assertIsNone(ssh.build_host_access({ssh.HOST_VAR: ' ', ssh.USER_VAR: '', ssh.KEY_VAR: ''}))

    def test_all_three_settings_turn_it_on(self):
        self.assertEqual(ssh.build_host_access(self.env()), {'host': 'build-1', 'user': 'builder', 'key': str(self.key)})

    def test_a_half_configured_channel_is_an_error_not_a_silent_off(self):
        with self.assertRaisesRegex(ssh.AccessError, ssh.KEY_VAR):
            ssh.build_host_access(self.env(**{ssh.KEY_VAR: ''}))

    def test_a_missing_key_file_is_an_error(self):
        with self.assertRaisesRegex(ssh.AccessError, 'key file'):
            ssh.build_host_access(self.env(**{ssh.KEY_VAR: str(self.key) + '.gone'}))

    def test_a_name_that_could_become_an_ssh_option_is_refused(self):
        for bad in ('-oProxyCommand=evil', 'host name', 'h;rm'):
            with self.assertRaises(ssh.AccessError, msg=bad):
                ssh.build_host_access(self.env(**{ssh.HOST_VAR: bad}))

    def test_an_admin_account_is_called_out(self):
        self.assertTrue(ssh.is_admin({'user': 'Administrator'}))
        self.assertFalse(ssh.is_admin({'user': 'builder'}))

    def test_every_command_is_logged_before_and_after(self):
        code, calls = self.run_command('Get-ChildItem C:/build/cache', self.env())
        self.assertEqual(code, 0)
        self.assertEqual(calls[0][-2:], ['build-1', 'Get-ChildItem C:/build/cache'])
        self.assertIn('BatchMode=yes', calls[0])
        self.assertEqual([(r['event'], r.get('exit')) for r in self.log_records()], [('start', None), ('end', 0)])

    def test_deletes_are_refused_and_logged(self):
        for command in ('rm -rf /build/cache', 'cd x && del /q *.obj', 'Remove-Item -Recurse C:/cache', 'git clean -fdx',
                        'docker system prune -af', 'find /tmp -name x -delete', 'sudo rm a', 'cmd /c rd /s x'):
            code, calls = self.run_command(command, self.env())
            self.assertEqual((code, calls), (2, []), command)
        records = self.log_records()
        self.assertEqual(len(records), 8)
        self.assertTrue(all(r['event'] == 'refused' for r in records))

    def test_parking_and_reading_are_allowed(self):
        for command in ('mv /build/cache /build/cache.quarantine', 'Move-Item C:/cache C:/cache.parked',
                        'docker ps --format x', 'git log --format=%H -1', 'cat /var/log/runner.log'):
            self.assertEqual(ssh.refusal(command), '', command)

    def test_nothing_connects_while_repair_is_off(self):
        code, calls = self.run_command('hostname', {ssh.LOG_VAR: str(self.log)})
        self.assertEqual((code, calls), (2, []))


if __name__ == '__main__':
    unittest.main()
