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

    def test_the_board_review_runs_on_its_own_timer(self):
        original = m.REVIEW_INTERVAL
        try:
            m.REVIEW_INTERVAL = 0
            self.assertFalse(m.review_due({}, 10**9))
            m.REVIEW_INTERVAL = 3600
            self.assertTrue(m.review_due({}, 10**9))
            self.assertFalse(m.review_due({m.REVIEW_KEY: {'last': 10**9 - 60}}, 10**9))
            self.assertTrue(m.review_due({m.REVIEW_KEY: {'last': 10**9 - 3600}}, 10**9))
        finally:
            m.REVIEW_INTERVAL = original

    def test_the_review_prompt_covers_the_whole_board(self):
        prompt = m.build_prompt(0, [], 5.0, local_runbook='', review=True)
        self.assertIn('The board review', prompt)
        self.assertIn('- none', prompt)
        # The review lists raw flags, which have no attempt count (2026-11-16 KeyError).
        raw = self.flag('stalled:7:since:1')
        self.assertIn('stalled:7:since:1', m.build_prompt(0, [raw], 5.0, local_runbook='', review=True))

    def test_a_flag_past_its_attempts_is_left_for_the_owner(self):
        state = {'a': {'attempts': m.MAX_ATTEMPTS, 'last': 0}}
        self.assertEqual(m.due_flags([self.flag('a')], state, 10**9), [])


class CIFlagTests(unittest.TestCase):
    def call_with(self, answer):
        calls = []
        original = m.api

        def fake(method, path, body=None):
            calls.append(path)
            if isinstance(answer, Exception):
                raise answer
            return answer
        m.api = fake
        try:
            return m.ci_flags(), calls
        finally:
            m.api = original

    def test_ci_flags_join_the_pass_with_the_queue_limit(self):
        flag = {'ticket_id': 64, 'repo_id': 2, 'kind': 'ci_cancelled', 'fingerprint': 'ci_cancelled:r2:run:5', 'detail': 'd'}
        flags, calls = self.call_with({'flags': [flag, {'kind': 'x'}], 'notes': ['acme/api: CI runners not readable']})
        self.assertEqual(flags, [flag])
        self.assertEqual(calls, [f'/api/v1/manager/ci-flags?queue_minutes={m.CI_QUEUE_MINUTES}'])

    def test_a_failing_ci_sensor_never_breaks_the_pass(self):
        flags, _ = self.call_with(OSError('HTTP Error 404'))
        self.assertEqual(flags, [])

    def test_ci_sensors_can_be_turned_off(self):
        original = m.CI_SENSORS
        try:
            m.CI_SENSORS = False
            flags, calls = self.call_with({'flags': [{'kind': 'ci_failed', 'fingerprint': 'f', 'detail': 'd'}]})
        finally:
            m.CI_SENSORS = original
        self.assertEqual((flags, calls), ([], []))

    def test_a_repo_level_ci_flag_names_its_repo(self):
        line = m.flag_line({'repo_id': 2, 'title': 'acme/api', 'kind': 'ci_failed', 'detail': 'main is red', 'fingerprint': 'f'})
        self.assertIn('repo 2 acme/api [ci_failed]', line)


class BuildHostPromptTests(unittest.TestCase):
    def test_without_ssh_settings_the_session_may_only_read_and_file(self):
        prompt = m.build_prompt(2, [], 5.0, local_runbook='')
        self.assertIn('Build-host repair is off', prompt)
        self.assertNotIn('build_host_ssh.py', prompt)

    def test_with_ssh_settings_the_session_goes_through_the_logged_wrapper(self):
        prompt = m.build_prompt(2, [], 5.0, local_runbook='', build_host={'host': 'h', 'user': 'builder', 'key': 'k'})
        self.assertIn('Build-host repair is on, as the account `builder`', prompt)
        self.assertIn('build_host_ssh.py', prompt)
        self.assertIn('Never delete', prompt)


class RunbookRuleTests(unittest.TestCase):
    """The runbook rules every session must get. 2026-11-16: a direction deferred
    acceptance tests for code that still shipped, and the follow-up was never filed."""

    def test_a_split_never_defers_tests_for_code_that_ships(self):
        prompt = m.build_prompt(2, [], 5.0, local_runbook='')
        for rule in ('**Splitting rules**', 'Never defer a test for code\n  that ships',
                     '**File the follow-up in the same session**', '**Only splittable scope moves:**',
                     'under "Splitting rules"'):
            self.assertIn(rule, prompt)

    def test_infrastructure_is_checked_before_a_stale_head_is_blamed(self):
        prompt = m.build_prompt(2, [], 5.0, local_runbook='')
        self.assertIn('## Infrastructure checklist', prompt)
        self.assertIn('Never call a PR head stale without `ci_status`', prompt)


class SummaryTests(unittest.TestCase):
    def test_the_summary_block_is_what_the_owner_reads(self):
        text = 'thinking...\n```\nMANAGER SUMMARY\n- #49 cycling: why: x | action: y | expect: z\n- owner: none\n```'
        self.assertEqual(m.session_summary(text), 'MANAGER SUMMARY\n- #49 cycling: why: x | action: y | expect: z\n- owner: none')
        self.assertEqual(m.session_summary(''), '')


class HeadlineTests(unittest.TestCase):
    def test_the_headline_line_is_sent_with_the_summary(self):
        summary = 'MANAGER SUMMARY\nHEADLINE: Merged #12 and parked #13.\n- owner: none'
        self.assertEqual(m.session_headline(summary), 'Merged #12 and parked #13.')
        self.assertEqual(m.usage_body(0.123456, 'sonnet', summary),
                         {'cost_usd': 0.1235, 'model': 'sonnet', 'summary': summary, 'headline': 'Merged #12 and parked #13.'})

    def test_a_missing_or_long_headline(self):
        self.assertEqual(m.session_headline('MANAGER SUMMARY\n- owner: none'), '')
        self.assertEqual(m.session_headline('**HEADLINE:** Answered #64.'), 'Answered #64.')
        long = m.session_headline('HEADLINE: ' + 'word ' * 40)
        self.assertEqual(len(long), m.HEADLINE_LIMIT)
        self.assertTrue(long.endswith('…'))


class ActivityTests(unittest.TestCase):
    def test_targets_are_the_flagged_tickets_and_a_named_host(self):
        flags = [{'ticket_id': 64, 'detail': 'x', 'fingerprint': 'a'}, {'ticket_id': 55, 'detail': 'y', 'fingerprint': 'b'},
                 {'ticket_id': 0, 'detail': 'builder-win stopped taking jobs', 'fingerprint': 'sensor:host'}]
        self.assertEqual(m.activity_targets(flags, ['builder-win', 'builder']), ([55, 64], 'builder-win'))
        self.assertEqual(m.activity_targets([{'ticket_id': 7, 'detail': 'rebuilding', 'fingerprint': 'c'}], ['builder']), ([7], ''))
        self.assertEqual(m.activity_targets([{'host': 'h1', 'detail': '', 'fingerprint': 'd'}], []), ([], 'h1'))

    def test_a_failed_report_never_stops_the_loop(self):
        calls = []
        original = m.api

        def failing(method, path, body=None):
            calls.append((method, path, body))
            raise OSError('board down')
        m.api = failing
        try:
            m.report_activity('idle')
        finally:
            m.api = original
        self.assertEqual(calls, [('POST', '/api/v1/manager/activity', {'state': 'idle'})])



class NotesTests(unittest.TestCase):
    def test_housekeeping_before_the_block_is_logged_not_shown(self):
        text = 'job 9f3a cache 12 GB\n```\nMANAGER SUMMARY\nHEADLINE: Fixed it.\n### Owner\n- Nothing.\n```'
        self.assertEqual(m.session_notes(text), 'job 9f3a cache 12 GB')
        self.assertNotIn('9f3a', m.session_summary(text))
        self.assertEqual(m.session_notes('MANAGER SUMMARY\nHEADLINE: x'), '')


if __name__ == '__main__':
    unittest.main()


class OwnerRepeatTests(unittest.TestCase):
    SUMMARY = ('MANAGER SUMMARY\nHEADLINE: Waited on the runner.\n\n### #175 Threading\n- **Did:** nothing.\n\n'
               '### Owner\n- {ask}\n')

    def test_the_owner_hears_a_thing_once_a_day(self):
        state, now = {}, 1_000_000.0
        ask = 'Add a second Windows GL runner: jobs wait 2.5 hours for the only one.'
        first = m.dedupe_owner(self.SUMMARY.format(ask=ask), state, now)
        self.assertIn(ask, first)
        reworded = 'One Windows GL runner cannot keep up; add a second GL runner, jobs wait 3 hours.'
        second = m.dedupe_owner(self.SUMMARY.format(ask=reworded), state, now + 600)
        self.assertNotIn('second GL runner', second)
        self.assertIn(m.NOTHING_NEW, second)
        self.assertIn('### #175 Threading', second)
        later = m.dedupe_owner(self.SUMMARY.format(ask=reworded), state, now + m.OWNER_MEMORY + 1)
        self.assertIn('second GL runner', later)

    def test_nothing_is_never_remembered_and_new_asks_reach_the_prompt(self):
        state, now = {}, 5.0
        m.dedupe_owner(self.SUMMARY.format(ask='Nothing.'), state, now)
        self.assertEqual(m.recent_asks(state, now), [])
        m.dedupe_owner(self.SUMMARY.format(ask='Grant the app Actions write.'), state, now)
        prompt = m.build_prompt(31, [], 5.0, local_runbook='', told=m.recent_asks(state, now))
        self.assertIn('Already told the owner', prompt)
        self.assertIn('Grant the app Actions write.', prompt)

    def test_a_summary_without_an_owner_section_is_kept(self):
        self.assertEqual(m.dedupe_owner('MANAGER SUMMARY\nHEADLINE: x', {}, 0), 'MANAGER SUMMARY\nHEADLINE: x')
