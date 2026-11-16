# Overhaul plan: the board as an autonomous engineering team

Status: **draft for the owner to iron out.** Each section ends with the
decisions it needs. Builds on [engine_lessons.md](engine_lessons.md) (what
broke and why) and [merge_train.md](merge_train.md) (batched verification).

The board has become an autonomous engineering team. This plan makes it run
like one:
- every ticket passes through clear roles;
- verification costs match what a change actually needs;
- one manager runs the team;
- the owner can see all of it in one view.

What stays: the **Board** view and the **Flow** view are unchanged.

## 1. The manager runs the team

Today the manager reacts to flags and may not merge. It becomes the team
lead, with full authority over autonomous work. Its objectives, in order:

1. **Cost**: no pass, run or review that cannot change an outcome.
2. **Progress**: every ticket always has a live next step.
3. **Speed**: only after the first two.

**Hourly review**, in addition to the flag reactions every 10 minutes. It reads
the whole board: tickets, PRs, build queue, runners, open questions. Then it acts:
- merges PRs that are green and verified;
- cancels duplicate or obsolete runs;
- parks loops;
- re-scopes or splits oversized tickets (core now, the rest filed as
  follow-ups);
- answers questions;
- tunes the build host's job limit;
- files repair tickets for infrastructure faults it cannot fix itself.

**Limits**:
- autonomous tickets only;
- never waives a test, review finding or gate;
- changes to the deployment's own code go through a PR a person reviews;
- quality policy changes (moving a gate, changing coverage) are the owner's
  call.

**Visible**: each hourly review writes a short report: what moved, what it
did, what it costs. It shows in the Team view and in Settings, and the whole
log is kept.

> Decisions: hourly review cadence (hourly / every 2 h); daily budget
> (today $10).

## 2. A pipeline of roles per ticket

Refinement (the business-analyst work) happens before a ticket starts. Once a
ticket is **in progress**:

| Role | When | Does | Model |
|---|---|---|---|
| Engineer | always | writes the change and its tests | Sonnet; escalates on repeated failure |
| DBA | the change touches a schema, migrations or a file format (detected from paths and the spec) | reviews data changes, version bumps, migration safety | Sonnet |
| QA | always | maps every acceptance criterion to a test that ran, runs the verification the ticket needs (section 3), and approves or returns the ticket with the specific gap | Sonnet |

QA replaces today's generic adversarial review. A failed QA goes back to the
Engineer with the exact unmet criterion. At the fix-round limit, the manager
re-scopes the ticket instead of publishing a PR with known problems.

**One source of profiles.** The role profiles live in one place, versioned,
and every runner uses that version, whether its model is Claude or Codex.
Each runner reports the profile version it loaded. Today the Claude runners
carry an old copy, and the Codex runners carry none.

> Decisions: DBA triggers (schema, migrations, file format, anything else?);
> does QA run on Sonnet, or on a different model from the Engineer?

## 3. Verify what a change needs, and build the rest on a schedule

Each ticket gets a **verification class** from the files it changes and its
acceptance criteria:

| Class | Proven by | Before merge |
|---|---|---|
| Logic / headless | unit and integration tests on Linux CI | Linux CI + QA |
| Platform or visual (desktop UI, GPU rendering, installer) | a targeted run on the matching build host: only the affected suites, about 10–20 min | + targeted run |
| Everything | the **rolling full build** of the default branch | not before merge |

**Rolling full build.** The default branch gets a full build and test on each
platform every N hours, covering everything merged since the last build. When
it fails:
- the manager opens a failure ticket naming the commit range;
- it bisects inside that range when the culprit isn't obvious;
- it may revert the culprit to unblock others.

A failure ticket for a build-only problem is verified by a targeted run of the
failing suite on that platform.

This replaces "a full run per ticket per attempt", the main cost and
bottleneck on a board with one Windows host.

> Decisions: rolling build cadence (every 2 h / 4 h); may the manager
> auto-revert a culprit, or only open the ticket?

## 4. Verdicts in the board

Pass/fail per commit and target is stored by the board. That includes "a
clean merge of a passed commit". The worker, merge sweep, manager and QA all
read it. This removes the class of loops where a pass was recorded in one
place and checked in another (engine_lessons.md).

## 5. The Team view

A new top-level view beside Board and Flow: the team as an org chart.

- **Manager** on top: status, objective order, today's spend against budget,
  open flags, next review, and its latest decision in one line.
- **Runners**: one card each. Provider and model, profile version, state
  (working / waiting / idle / offline), the ticket it holds, and a one-line
  "doing now".
- **Tickets in flight**: the current role (Engineer / DBA / QA), where the
  ticket is (a runner, a build-host slot or the queue), time on host, and
  tokens spent.
- **Build hosts and CI runners**: slots with the job, phase and elapsed time;
  the queue with wait times; CI runners online or busy.
- **Links**: manager → runners → tickets → build slots, drawn as live wires.
  Waiting links are dashed; active ones animate.

The look is cyberpunk: dark ground, neon cyan, magenta and yellow, Orbitron
headings, monospace data, scanlines, and pulses on live work. A mockup
exists in the owner's design canvas.

Data: one `GET /api/v1/team` endpoint and live updates over the board's event
stream. Runners report model, profile version and "doing now" through the
heartbeat they already send.

## 6. New icon

An engine shooting lightning, drawn larger so it reads in the header and the
browser tab. Three concepts are in the mockup: a V-engine block, a bolt piston
and a jet nozzle.

> Decision: which concept.

## Order of work

1. Manager hourly review and authority (section 1). It pays for itself at once.
2. Verdicts in the board (section 4). The verification changes depend on it.
3. Team view and icon (sections 5–6). Independent; can run alongside step 2.
4. Verification classes and the rolling build (section 3).
5. The role pipeline with one source of profiles (section 2).
