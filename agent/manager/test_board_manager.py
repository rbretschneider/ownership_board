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

    def test_a_flag_past_its_attempts_is_left_for_the_owner(self):
        state = {'a': {'attempts': m.MAX_ATTEMPTS, 'last': 0}}
        self.assertEqual(m.due_flags([self.flag('a')], state, 10**9), [])


if __name__ == '__main__':
    unittest.main()
