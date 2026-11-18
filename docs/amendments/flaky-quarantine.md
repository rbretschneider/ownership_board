# Flaky-test quarantine

Migration 0028. Applies to every repo. A repo whose verification script
ignores the quarantine file behaves exactly as before.

## Why

On the one Windows host, most failed runs in mid-November 2026 were GL and UI
tests that failed on one commit and passed on the next with no related change.
Each such failure cost an author repair pass and another full run. A test
that fails consistently on one ticket is a real failure; a test that fails on
unrelated tickets and then passes on the same code is flaky.

## Design

- **Failure record.** Every finished run with a verdict records the tests it
  failed (`test_failures`), per ticket commit it verified. For a batch run that
  is every member it verified. The board reads names from the command logs:
  CTest's "The following tests FAILED:" list, a numbered list under a
  "--- CTest failed tests" heading, "first failing case: [report.xml: ]name:
  Failed" lines and the board's own `ownership-test-failed: name` lines. At
  startup an empty record is backfilled from stored failed jobs.
- **Flaky.** A test is flaky when it failed on two tickets in two different
  runs, and a later run of a commit it had failed passed with the test run.
  The board then quarantines it (`test_quarantine`, source `detected`) and
  files an autonomous fix ticket (filed by the board, kind bug).
- **Flake check.** When a commit job fails a test that another ticket failed
  in a different run within 14 days, the board queues a flake check: a job of
  kind `flake_check` on the same commit, reusing its bundle, listing only
  those tests (`only_tests`). A pass makes them flaky; a failure is recorded
  like any other. A flake check never gives a verdict and never blocks the
  ticket's worker. Only agents 1.2 or later get one, since an older agent
  would run the whole suite. Batch bisection gives the same evidence without
  a check: a red batch records its failures for each member, and a member
  that later passes alone passed on a commit that had failed the test.
- **The list on every job.** Each assignment carries `quarantine`, the repo's
  active tests. Agent 1.2 writes it to `ownership-quarantine.txt` beside the
  bundle (one exact name per line, always written, empty when nothing is
  quarantined) and sets `OWNERSHIP_QUARANTINE_FILE` to its path. A flake check
  also gets `ownership-only-tests.txt` and `OWNERSHIP_ONLY_TESTS_FILE`, and
  `OWNERSHIP_JOB_KIND=flake_check`. The script decides: it should never skip a
  test the current ticket requires (MakerCAD lists those in
  `.ownership/required_tests/*.json`; the board cannot read repo files).
- **Release.** When a test's fix ticket merges, the test goes on probation: it
  leaves the list sent to jobs and runs again. After 3 passing full runs it is
  released. A failure on probation puts it back and files a new fix ticket.
  A person or delegate can release a test by hand.
- **Seeding.** A person (POST `/api/v1/repos/{id}/quarantine`) or a delegate or
  manager agent (MCP `quarantine_test`) can add a test, source `seeded` with
  their name, with the board's failure history as evidence. A fix ticket that
  already exists can be linked instead of filing a new one.

Settings → repo row → "Quarantined tests" lists each repo's entries with who
added them, the status and the fix ticket. The ticket timeline notes each flake
check and each test the board quarantined.

Rolling the migration back drops the tables and the job columns and deletes
flake-check jobs.
