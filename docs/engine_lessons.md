# Engine lessons: how autonomous tickets stall, and what fixed it

A living record. Each entry is a failure mode seen on a real board running
autonomous agents against a repository with a slow, scarce verification host
(one Windows build host, about 1.5 h per full run). Each entry gives the
symptom, the cause, the fix, and whether that fix is **permanent** (in code),
**mitigated** (a setting or workaround), or **open**. The last section is the
backlog of flow changes to make once the current milestone ships.

The theme throughout: **the engine stalls at the seams between components**:
the worker and the merge sweep, the worker and the manager, the branch and the
default branch. It rarely stalls inside any one of them. Every fix below
closes a seam.

## Failure modes

### The manager parked a failing ticket on an unrelated CI fix

| | |
|---|---|
| Symptom | A ticket waited on a CI-fix ticket for hours while its own new performance test failed on the build host at twice its budget. The manager had filed that CI fix as a duplicate of an open PR. Its spec said "no test is changed" while the policy test asserted the bug, so the fix could only publish unresolved. The owner was told to "add a runner" every pass. |
| Cause | The manager read only the provider's CI, where every code job was green, and never saw the board's own failing verdict. Nothing checked its links, its filings or its specs, and each session started with no memory of what it had already told the owner. |
| Fix | **Permanent.** Every manager flag carries the ticket's newest board verdict and its failing tests (fingerprints unchanged, so no extra sessions). The board refuses a manager `depends_on` from a ticket whose newest board build failed in its own code, and expires the manager's links after 6 hours. A manager filing must name its files and is refused when an open ticket names any of them. The runbook requires fix specs to put tests that assert the defect in scope, and the loop drops owner asks repeated within 24 hours. |

### Verification results thrown away

| | |
|---|---|
| Symptom | A ticket passed host verification three times and never published. Each pass was followed by another full run. |
| Cause | While the host ran, the worker released the ticket. When it resumed for the verdict, it first merged the default branch, because the verifier there had changed. That moved HEAD, so it queued a new run and never read the passing verdict. |
| Fix | **Permanent.** On resume, collect the verdict for the commit that was submitted, before anything moves HEAD. Record the pass. A later clean merge of the default branch into a passed commit skips the host. |

### A pass recorded in one place and checked in another

| | |
|---|---|
| Symptom | The merge sweep queued a repair every 5 minutes for a PR the worker had already accepted. Each repair was a strong-model pass that changed nothing. |
| Cause | The worker skipped re-verifying a clean merge of a passed commit, but recorded only the original commit as passed. The sweep looks up the PR's exact head and found no pass. |
| Fix | **Permanent.** The board stores the verdict per repo, target and commit (`verdicts`), written in the same transaction as the build job's result. A clean merge of the default branch into a passed commit is recorded there with `report_clean_merge`, by the agent holding the ticket's lease. The worker, merge sweep and manager read that one record (`get_verdicts`, or the ticket's `verdicts`). Switching the deployment's worker and merge sweep off their local files follows the board change. |

### Questions answered, then asked again

| | |
|---|---|
| Symptom | A ticket asked the same question every 10 minutes, eight times. The manager answered each one within minutes. |
| Cause | After N identical failures, the worker stops retrying and asks. It counted those failures from the last time a *person* answered, so a manager answer never reset the count, and the worker re-asked at once without changing anything. |
| Fix | **Permanent.** Any answered question restarts the count. After an automated answer, the worker asks again after 2 identical failures instead of N, so oversight continues without a loop. |

### A half-updated verification harness

| | |
|---|---|
| Symptom | Every run failed the verifier's own self-test in a few minutes, with "property not found". |
| Cause | The verifier is a set of files: the entry script, helpers it dot-sources, and self-test fixtures. The branch had the default branch's entry script and fixtures, but an older helper. The staleness check compared only the entry script. |
| Fix | **Permanent.** Staleness is checked across every harness file. If any differs and the branch is behind, the worker merges the default branch before verifying. **Open:** see "verify with the default branch's harness" in the backlog. |

### Cold dependency builds per slot

| | |
|---|---|
| Symptom | One ticket's runs always ended at the host's 3-hour limit, as an infrastructure failure, and were retried with no code change. |
| Cause | A secondary build stage kept its dependency cache in a per-slot folder. A run on any other slot rebuilt large dependencies from source. |
| Fix | **Permanent** for that repository: the cache is shared across slots and survives a timeout, and the stage moved to CI. The rule that matters: **a timeout is never retried without a change**. Warm the cache or fix the stage instead. |

### A timing gate measured under load

| | |
|---|---|
| Symptom | Passing code failed "full QA exceeded 15 minutes". |
| Cause | The timed pass ran while the host ran two other verification jobs. |
| Fix | **Mitigated:** the host runs 2 jobs at a time instead of 3, and slow suites run once per ticket, not in every configuration. **Open:** a timing gate needs a quiet host, or must be measured outside per-ticket verification. |

### The same verification twice

| | |
|---|---|
| Symptom | Board PRs waited hours after their verification passed. |
| Cause | Hosted CI re-ran the same Windows build and GUI suites on the same single host, queued behind other jobs, as a required check. |
| Fix | **Permanent:** PRs from board branches skip the duplicate CI jobs, because their exact commit was already verified. Those jobs still run on the default branch and nightly. |

### Tests are most of a run

| | |
|---|---|
| Symptom | A full run took 85 min. The build took 19 min; tests took 63, and most suites ran in both configurations. |
| Fix | **Permanent:** quick suites run in Debug; slow suites (GUI, diagnostics, full QA) run once, in Release. The repository can still request every suite in every configuration. |

### One full run per queued ticket

| | |
|---|---|
| Symptom | About six makercad tickets waited in the Windows queue with rolling builds on. In a week the one host ran 233 failed runs (about 41 min each) against 68 passes (about 79 min each). |
| Cause | Rolling supersede only replaces a ticket's own older commit. Different tickets' commits each needed their own full run. |
| Fix | **Permanent, opt-in per repo:** batched verification merges up to the repo's batch size of queued commits into one run, passes each member for its exact commit, and bisects a red batch until the culprit runs alone ([amendment](amendments/batched-verification.md)). It pays only where red means a broken ticket. A new protocol field must reach only agents that understand it: an older agent would ignore `batch` and pass the lead commit alone, so the board gates batches on the agent version. |

### A watcher that only watches

| | |
|---|---|
| Symptom | A supervisor logged "find the root cause" for a ticket at 14 attempts, and nothing acted on it. The board manager diagnosed a host fault correctly and handed it to a person. |
| Fix | **Permanent:** one watcher, the board manager, with an opt-in repair mode. It fixes build-script and build-host causes itself, under a deployment runbook. Sensors that need privileged access (container logs, the runner API) run separately with no model and feed flags to the manager. The old supervisor's model was retired. |

### A watcher that looks once

| | |
|---|---|
| Symptom | The manager looked at a ticket at 10 build passes and not again until 15. |
| Cause | The build-cap flag changed only at multiples of the cap. |
| Fix | **Permanent:** every pass over the cap, and every repeat of the same failure, is a new flag, with one session each. |

### Automatic waivers

| | |
|---|---|
| Symptom | A story passed its gate as "owner waived", although no person had waived anything. Another waived its own test failure and looped through 22 builds. |
| Fix | **Permanent:** an automated decision never waives a generic error or a story's own new tests. Only a person can waive. |

### Publishing at the fix-round limit

| | |
|---|---|
| Symptom | Three stories reached their limit of 2 fix rounds and were published with open P0/P1 review findings. Each then cycled through repair. |
| Status | **Open.** Publishing keeps the work visible, but the PR cannot merge, and every repair costs a host run. See backlog. |

### Deploying worker code

| | |
|---|---|
| Symptom | A fix was deployed, but the worker that took the ticket next still ran the old code. |
| Cause | Workers load their code at start, and restarts waited for an idle moment. |
| Fix | **Mitigated:** a script restarts each worker as soon as it is idle. **Open:** the worker should reload itself between passes. |

### Infrastructure blamed on a stale PR head

| | |
|---|---|
| Symptom | Overnight, the manager steered tickets well but missed every infrastructure blocker: bundle storage refusing uploads, a release dry run failing, a CI job silently cancelled by a concurrency group's single pending slot. Each time it blamed a stale PR head, and once it cancelled CI on a PR's current head with a raw provider token. |
| Cause | The manager could not see CI or the build infrastructure through the board, so it guessed from the ticket's timeline. Its CI access was a hand-made token with no guards. |
| Fix | **Permanent:** CI through the board. `ci_status`, `ci_failed_log`, `ci_rerun` and `ci_cancel` use the board's own connection under server-side guards (never cancel a current head, re-run only completed runs, every action on the ticket's history), and CI sensors raise flags for queued, evicted and failed runs and offline runners. The runbook checks infrastructure first. Build-host ssh is opt in, logged, and refuses deletes. |

## Principles these taught

1. **One source of truth per fact.** A verdict, a waiver or an answer kept in two
   places (a worker file and the board, say) disagrees eventually, and the
   disagreement is a loop.
2. **A retry is never a fix.** A rerun without a change is waste; so is a
   question re-asked without new evidence. Every loop above was missing this
   check somewhere.
3. **Watchers must act.** A sensor that only logs is noise. The component that
   notices a problem should own fixing it, within written limits.
4. **Count everything from the last decision, whoever made it.**
5. **Measure the slow resource first.** Most delay was queue wait and duplicate
   verification, not agent speed.
6. **Slow and forward beats fast and looping.** Every ticket needs a live next
   step: a running job, a queued pass or a concrete direction. A ticket with
   none is stalled, whatever its speed.
7. **Quality is not traded for flow.** Speed comes from removing waste: duplicate
   runs, loops, cold caches. It never comes from waiving findings.

## Backlog for the flow overhaul

Ordered by expected effect. Each item is generic; the per-repository details
live in that repository's settings and verification script.

1. **Verdicts in the board.** *Built: the `verdicts` record and API; the
   deployment's scripts switch over next.* The board stores pass/fail per commit and target,
   plus "clean merge of a passed commit" equivalence. The worker, merge sweep,
   manager and merge reflex all read the same record. This removes the whole
   class of record-in-one-place loops.
2. **Verify with the default branch's harness.** Run the default branch's
   verification harness against the ticket's code, instead of the branch's own
   copy. A ticket that changes the harness becomes a harness ticket, verified
   by the harness's own self-tests first. This removes stale and mixed harness
   failures.
3. **Merge train** (see [merge_train.md](merge_train.md)): a fast per-ticket
   check, one full run per stack of tickets, and splitting on failure.
   Per-repository policy, tuned by the manager.
4. **Fix-round limit goes to the manager, not to a PR.** At the limit, the
   manager re-scopes. It narrows the ticket to its core acceptance criteria,
   splits off a follow-up, or names the one failure to fix. Publishing with
   open P0/P1 findings stops.
5. **Question protocol.** Every question has an owner: the manager for
   autonomous tickets. The board de-duplicates identical open questions, and an
   answer resets every counter that depends on it.
6. **Host capacity as data.** Queue wait, run time and contention per host are
   shown in the app (the card clock is the start). They drive the host's job
   limit and when to add a host or a GPU.
7. **Worker reload between passes**, so a deployed fix applies to the next pass
   without a restart.
8. **Cost per ticket**: model tokens and host minutes on every card and in the
   stats. Waste shows up as the cost of a ticket that did not move.
9. **Planning rules enforced at filing:** story size, dependency order and
   shared-file overlap. The tickets that looped longest all edited shared
   harness files.
