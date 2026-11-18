# Batched build-host verification

Migration 0027. Opt-in per repo (`batch_verify`, `batch_max` 2–8, default off
and 4). Repos that stay off keep one build-host job per commit.

## Why

On 2026-11-18 the one Windows host (idaho, 2 slots) had spent most of a week
on failures: 233 failed runs (about 41 min each) against 68 passes (about 79
min each). "Rolling" supersede only replaces a ticket's own older commit, so
six queued tickets meant six full runs. A batch verifies several tickets'
commits in one run. It pays off only where a red run means a broken ticket:
where flaky tests turn runs red, every red batch also costs its bisection
runs. That is why the setting is per repo.

## Design

- **The ticket job stays the record.** Workers submit and poll their own
  `build_jobs` row as before. A batch (`build_batches`) only holds queued
  jobs (`build_jobs.batch_id`) and settles them. Workers needed no change.
- **Assembly at claim.** When a host whose agent is 1.1 or later claims, the
  board first offers a queued bisection half, then picks the next job as
  today (hot tickets first). If that job's repo batches, and the job is not
  hot and not marked solo, the board adds the oldest free queued jobs of other
  tickets in the same repo with the same preset and target, up to
  `batch_max`. Hot tickets never batch: they are likely red and would bisect
  the rest. With fewer than two commits the job runs alone. A batch takes one
  host slot.
- **Merging on the host.** The board image has no Git, so the host merges.
  The assignment is job-shaped (the lead commit's sha and bundle) plus
  `batch.members` in queue order. The agent fetches each member's bundle by
  that member's job id, checks out the lead and merges the others with
  `git merge --no-ff`. A member that does not merge cleanly is undone
  (`git reset --merge`), left out, and listed in the result's `excluded`.
  An agent before 1.1 would ignore `batch` and verify only the lead, so the
  board never hands it a batch.
- **Pass fan-out.** A green batch passes every live, included member: its own
  job becomes `passed` with the batch's result, and the board records the
  verdict for that member's exact commit and job id (`reported_by` is
  `build-host/<id> batch <id12>`). The merge rules (green GitHub checks plus
  a Windows pass of the PR head) are unchanged.
- **Bisection.** A red batch gives no member a verdict. Its live members are
  split in half, in order. A half of two or more becomes a queued batch
  (`parent_id` links it). A single member is marked solo and runs alone,
  which gives it its own verdict and logs. One culprit among n commits is
  isolated in about log2(n) further runs, and the other halves still pass.
- **Conflicts.** An excluded member is marked solo and verified alone.
- **Supersede.** A new commit on a ticket supersedes its held job as before.
  The batch then counts that member as superseded: it never gets the batch's
  verdict, and the new commit's job is free to join a later batch. A batch
  whose members are all superseded is cancelled and its host told to stop.
- **No verdict.** A host failure (infrastructure or a stopped run) or a lost
  host re-queues the batch, up to three attempts. After that every member
  runs alone.
- **Owner cancel** of a batch id returns its members to the queue unmarked.

Each member's outcome (`passed`, `bisected`, `conflict`, `superseded`,
`returned`) is kept in `build_batch_members`. Each event is also a board
comment on the ticket's timeline, for example "Verification passed for commit
… in a batch with #198, #199". The runners strip shows a batch as its lead
ticket plus a count, with the members in the tooltip, and every member's card
shows it building.

Rolling the migration back drops the batch tables and the new repo and job
columns. A job still held by an open batch then loses its hold and runs
alone, which is safe.
