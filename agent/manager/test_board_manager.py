import unittest

import board_manager as m


class DueFlagsTests(unittest.TestCase):
    def flag(self, fingerprint):
        return {'fingerprint': fingerprint, 'ticket_id': 1, 'repo_id': 2, 'kind': 'cycling', 'title': 't',
                'stage': 'build', 'detail': 'd'}

    def test_new_flags_are_due_and_handled_ones_wait(self):
        now = 10_000.0
        state = {'a': {'attempts': 1, 'last': now - 60}, 'b': {'attempts': 1, 'last': now - m.RETRY_AFTER - 1}}
        due = {f['fingerprint']: f['attempts'] for f in m.due_flags([self.flag('a'), self.flag('b'), self.flag('c')], state, now)}
        self.assertEqual(due, {'b': 1, 'c': 0})

    def test_a_count_flag_gets_one_session_until_its_count_grows(self):
        cap = dict(self.flag('build_cap:49:build:3'), kind='build_cap')
        state = {'build_cap:49:build:3': {'attempts': 1, 'last': 0}}
        self.assertEqual(m.due_flags([cap], state, 10**9), [])
        grown = dict(self.flag('build_cap:49:build:4'), kind='build_cap')
        self.assertEqual(len(m.due_flags([grown], state, 10**9)), 1)

    def test_a_repeated_failure_gets_one_session_per_new_failure(self):
        state = {'cycling:64:failure:ab12:3': {'attempts': 1, 'last': 0}}
        self.assertEqual(m.due_flags([self.flag('cycling:64:failure:ab12:3')], state, 10**9), [])
        self.assertEqual(len(m.due_flags([self.flag('cycling:64:failure:ab12:4')], state, 10**9)), 1)

    def test_a_ticket_blocked_for_the_owner_costs_nothing(self):
        held = dict(self.flag('h'), kind='escalated', detail='blocked: Needs the owner: close it, the fix is on main')
        other = dict(self.flag('o'), kind='escalated', detail='blocked: waiting on CI')
        self.assertEqual([f['fingerprint'] for f in m.due_flags([held, other], {}, 0)], ['o'])

    def test_a_repair_in_progress_is_checked_hourly_on_the_base_model(self):
        held = dict(self.flag('r'), kind='escalated', detail='blocked: Board manager repairing: seeding the cache')
        state = {'r': {'attempts': m.MAX_ATTEMPTS + 2, 'last': 0}}
        due = m.due_flags([held], state, m.RETRY_AFTER + 1)
        self.assertEqual(len(due), 1)
        self.assertEqual(m.session_model(due), m.MODEL)
        self.assertEqual(m.due_flags([held], state, m.RETRY_AFTER - 1), [])

    def test_the_deployment_runbook_enables_repairs(self):
        flag = dict(self.flag('a'), attempts=0)
        self.assertNotIn('## This deployment', m.build_prompt(2, [flag], 5.0, local_runbook=''))
        prompt = m.build_prompt(2, [flag], 5.0, local_runbook='Build host: ssh builder')
        self.assertTrue(prompt.rstrip().endswith('Build host: ssh builder'))
        self.assertIn('Repairs are enabled', prompt)

    def test_deployment_sensors_add_flags_and_never_break_the_pass(self):
        original = m.SENSORS
        try:
            m.SENSORS = ('python3 -c "import json;print(json.dumps({\'flags\':[{\'kind\':\'infrastructure\','
                         '\'fingerprint\':\'runner:gl\',\'detail\':\'runner gl offline\'},{\'kind\':\'x\'}]}))"')
            flags = m.sensor_flags()
            self.assertEqual([(f['fingerprint'], f['repo_id']) for f in flags], [('sensor:runner:gl', 0)])
            self.assertIn('deployment [infrastructure]', m.flag_line(dict(flags[0], attempts=0)))
            m.SENSORS = 'echo not json'
            self.assertEqual(m.sensor_flags(), [])
        finally:
            m.SENSORS = original

    def test_a_flag_past_its_attempts_is_left_for_the_owner(self):
        state = {'a': {'attempts': m.MAX_ATTEMPTS, 'last': 0}}
        self.assertEqual(m.due_flags([self.flag('a')], state, 10**9), [])


if __name__ == '__main__':
    unittest.main()


class SummaryTests(unittest.TestCase):
    def test_the_summary_block_is_what_the_owner_reads(self):
        text = 'thinking...\n```\nMANAGER SUMMARY\n- #49 cycling: why: x | action: y | expect: z\n- owner: none\n```'
        self.assertEqual(m.session_summary(text), 'MANAGER SUMMARY\n- #49 cycling: why: x | action: y | expect: z\n- owner: none')
        self.assertEqual(m.session_summary(''), '')
