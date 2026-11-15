# Merge train: verify finished tickets together

Status: **proposal for review.** Nothing here is built yet.

## Problem

A repository with one slow, scarce verification host (for example a Windows
build server) is limited by that host, not by agents. Measurements from one
board on 2026-11-15:

| | |
|---|---|
| One full verification run | 85 min: 19 min incremental build of two configurations, 63 min of tests (most suites run in both configurations), 3 min other |
| Wait before a run starts | up to 93 min (host runs 3 jobs at once; GUI tests take turns on one desktop session) |
| Runs that failed | 13 of the last 20 |
| Runs per merged ticket | often 3 or more: each change to the default branch meant another full run for every open ticket (fixed separately: a clean merge of a passed commit now skips the run) |

Each finished ticket pays for at least one full run, even when two tickets
finish 30 seconds apart and would be verified by a single run of both.

## Idea

This is the merge queue with batching used by large monorepos (Uber
SubmitQueue, OpenStack Zuul, Bors, GitHub merge queue). The slow check runs
once on a *stack* of finished tickets instead of once per ticket.

1. **Per-ticket check (fast).** Each ticket gets the cheap checks on its own:
   hosted CI, plus a *fast* host run (incremental build and the quick test
   suites, about 20–25 min, no GUI or full-QA suites). This catches most
   failures in the ticket's own code, so the train starts from changes that
   are likely to pass.
2. **Train (slow, shared).** Tickets that passed step 1 and review join a
   train. The board stacks them in order on top of the default branch,
   `main + A + B + C`, on a train branch. Then it queues **one** full host
   run on the train's head.
3. **Pass:** every ticket in the train merges in order, with no further runs.
4. **Fail:** split the train in half and run each half (bisect) until the
   breaking ticket is found. That ticket goes back to its worker with the
   failure. The rest of the train merges.

**Conflicts.** When B conflicts with A, a worker resolves it on top of A
before the run: one short model pass in B's context, with hosted CI only.
If it cannot, B leaves this train and waits for the next.

## Board changes

- A **train** stage, between "PR open with checks green" and merged.
- A train record: an ordered list of tickets, base commit, head commit,
  state (forming, verifying, splitting, merged), and the culprit if any.
- Build jobs of kind `fast` (per ticket) and `train` (one head commit, many
  tickets). The repository's verification script receives the mode, so the
  repository decides what "fast" and "full" mean.
- Settings per repository: train on/off, maximum train size (default 4), and
  how long to wait for more tickets before starting (default 10 min).
- Manager flags: a train verifying too long, a ticket kicked from two trains
  in a row, a train splitting more than twice.
- The worker treats "kicked from train with failure X" like a failed
  verification run today.

## Per repository, never per product

The board knows nothing about any one repository's build. Everything above
is a per-repository policy, and a repository with no verification host is
not affected at all.

- **Off by default.** A repository runs each ticket's verification alone
  until the owner turns trains on in its settings.
- **The repository defines the work.** Its verification entry point receives
  `mode=fast|full` and decides what each means: which suites, which
  configurations, which hosts. A repository that offers only `full` gets
  trains without a fast check.
- **Bounds from the owner, tuning by the manager.** The owner sets the limits
  per repository: trains allowed or not, the maximum size, and the longest
  wait to form a train. Within those limits the manager picks the working
  values from that repository's own history:
  - **Raising the train size:** when the fast check's pass rate is high and
    full runs are long. A train of 4 pays off when most trains pass.
  - **Shrinking toward 1 (one ticket at a time):** when trains keep
    splitting, or when the fast check is missing or weak.
  - **Pausing trains for a repository:** when two trains in a row split down
    to a single culprit and that repository's default branch is failing.

  Each change is a recorded decision with its numbers (pass rate, splits,
  run minutes) in the manager's session summary. It is never a silent
  setting change. The owner can pin any value, and the manager leaves pinned
  values alone.
- **Hosts are matched by capability, not name.** A train's full run goes to
  any approved host whose labels match the repository's target, as single
  jobs do today.

## Expected effect

With trains of 4 and a 70% train pass rate, the full runs per merged ticket
drop from 1 or more to about 0.4, including the extra runs for splits. The
fast check adds about 25 min per ticket. That is cheaper than a full run, and
it removes most of today's failed full runs.

## Risks

- **Low pass rate makes splitting expensive.** The fast check is what keeps
  trains passing. If trains still fail more than half the time, shrink the
  train size automatically.
- **A failure can depend on two tickets together** (A and B each pass alone,
  but fail together). Splitting then blames the later ticket. It gets the
  failure with "passes alone, fails on top of #A" in the feedback.
- **The default branch moves during a train.** The train keeps its base. The
  next train starts from the new default branch.

## Independent of the train (can ship first)

- **Run the slow suites once, not twice.** The quick suites run in Debug;
  GUI, diagnostics and full QA run in Release. That saves about 25 of 85
  minutes per run. It is a test-policy choice for each repository's
  verification script. (Approved and proposed for the first board,
  2026-11-15.)
- **GPU on the build host.** The Windows build host is getting a small GPU,
  and the Linux host already has two. If the GUI tests use offscreen GPU
  contexts instead of the shared desktop session, they no longer take turns,
  which shortens the queue wait. A GPU also makes a GUI-capable fast lane
  possible, so a train's full run could split GUI tests across hosts. This
  only helps where the product's GUI tests run on that platform.

## Rollout

1. Split each repository's verification script into fast and full modes.
2. Fast per-ticket runs, with full runs still per ticket. Measure the pass
   rate.
3. Trains of size 2, then 4, while watching the pass and split rates.
