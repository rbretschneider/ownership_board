# Design

ownership_board is a private task board where an AI agent is the primary worker
and a human is the product owner. The board answers one question at a glance:
**whose move is it?**

## Lanes

The lanes are fixed. They are a domain enum, not configuration, so every agent
prompt, notification and API client can rely on them never changing. There
are four, so they fit side by side on a vertical 1080p monitor. The full
design, including the security decisions, is in [lanes.md](lanes.md).

| Id            | Label       | Meaning |
|---------------|-------------|---------|
| `todo`        | Todo        | Not started: being refined, or refined and waiting to be built. |
| `in_progress` | In Progress | Being designed or built. |
| `review`      | Review      | A pull request is open. The human reviews and merges. |
| `merged`      | Merged      | The pull request merged. Deployment is the repo's own CI/CD. |

## Stages

A lane says what kind of work a ticket is in. A stage says whose move it is.
The stage is the source of truth, and the server derives the lane from it.

| Lane | Stage | Whose move | Shown as |
|------|-------|------------|----------|
| todo | `refine` | agent | Refining |
| todo | `spec_review` | human | Spec ready, **Needs you** |
| todo | `ready` | agent | Ready to build |
| in_progress | `design` | agent | Designing |
| in_progress | `design_review` | human | Design ready, **Needs you** |
| in_progress | `build` | agent | Building |
| review | `pr` | human | PR open, **Needs you** |
| merged | `done` | nobody | none |

Two flags cut across stages:

- **Open questions.** Asking does not move the ticket. It keeps its lane and
  stage, shows **Needs you**, and cannot be claimed until every question is
  answered. The next claim resumes the same stage with the answers in the
  brief.
- **Blocked.** An agent that cannot continue marks the ticket blocked with a
  reason. It shows **Needs you** and cannot be claimed until a person
  unblocks it. A pull request closed without merging also blocks the ticket,
  back in `build`.

**Needs you** is true for open questions, a blocked ticket, or the stages
`spec_review`, `design_review` and `pr`.

## Checkpoints

Two settings per repo, changed only by people:

- **Approve spec** (default on). After refinement the ticket goes to
  `spec_review`. Off: straight to `ready`.
- **Approve design** (default off). Building starts with a design step. On:
  the ticket waits in `design_review`. Off: it goes straight to `build`.

A person resolves a checkpoint with **Approve** or **Request changes**. Request
changes needs a comment and sends the ticket back to `refine` or `design`; the
comment is in the next brief. An approval binds to the revision the person was
shown. If the text changed since, the server answers `409` and changes
nothing. Approvals are recorded as binding decisions. A ticket with no repo has
both checkpoints on.

## Who moves a ticket

The human can drag any card to any lane. The server picks the stage: the
first one in that lane whose checkpoints are met by approved revisions.
Dragging clears the lease and any block. Reordering inside a lane changes
nothing else.

Agents never choose a lane or stage. They hand a ticket on with `save_spec` or
`save_design` (with `submit`), `ask_questions` or `mark_blocked`. The one lane
move an agent makes is a finished build, with a PR URL set, into `review`.
`report_pr_state` moves a reviewed ticket to `merged` when its PR merged, or
back to `build`, blocked, when the PR closed without merging.

Claiming a `ready` ticket moves it to `design`.

The design is a markdown document on the ticket, written with `save_design`
and shown under the spec. It is private board content and never goes into a
repo, PR or commit.

## The question protocol

Free-form comment threads are slow on a phone and ambiguous for an agent. The
board uses structured question rounds instead.

The agent posts a round: a short summary of what it understood plus up to ten
questions. Each question can carry a list of options and a recommended option.

The human answers each question by tapping an option, typing free text, or
choosing **Your call**, which tells the agent to use its recommendation. One
button, **Accept recommendations**, answers a whole round in a single tap.

Rounds are kept forever, so the brief the agent reads later contains the full
history of what was asked and decided.

## Repos

The board is repo agnostic. Repos are synced from GitHub, Forgejo/Gitea and
Bitbucket Cloud (read-only tokens in the environment) or added by hand, and
the board filters by repo. A ticket may be created without one. Its brief
then lists every repo on the board, and working out the repo becomes part of
refinement: the agent sets it, or asks "Which repo is this for?" with the
likely repos as options. An agent cannot submit a spec until the ticket has
a repo.

## Agent leases

When an agent claims a card it holds a lease (30 minutes by default, renewable
with a heartbeat). Other agents skip leased cards. A lease that expires makes the
card claimable again, so a crashed session never strands work. A human moving a
card clears its lease. Every hand-off (submit, question, block, move to
review) also ends the lease.

Claim order is: `build` (finish what was started), then `design`, then
`refine` (refining is cheap and unblocks the human sooner), then `ready`.
A card is never claimable while it has open questions, is blocked, or waits
in a review stage. Within a stage, cards are taken top to bottom, so the
human prioritises by dragging.

| Claimed from | Action | Agent ends with |
|--------------|--------|-----------------|
| `refine` | `refine` | `ask_questions`, or `save_spec` with `submit` |
| `ready` or `design` | `design` | `save_design` with `submit` |
| `build` | `implement` | a PR, then `move_ticket` to `review` |

## The brief

Every claim returns a `brief`: one markdown document written for an LLM. It holds
the request, the refined spec, the design, every question round with its
answers, recent comments, and explicit instructions for the action the agent
should take. The spec and design are never cut; other sections shrink first. The
agent never has to stitch context together from several calls.

## Waking up

An agent loop does not need to poll on a timer. `POST /api/v1/agent/claim` with
`wait_seconds` blocks until work exists (or the wait expires), so an idle loop
costs one open HTTP request rather than repeated model turns. The same
operations are exposed as MCP tools at `/mcp`.

## Notifications

The board pushes to ntfy when the ball lands with the human:

| Event | Title | Message | Priority |
|-------|-------|---------|----------|
| Questions asked | `#<id> needs answers` | The agent has questions before it can continue. | high |
| Spec submitted for approval | `#<id> spec to approve` | A spec is ready for your approval. | high |
| Design submitted for approval | `#<id> design to approve` | A design is ready for your approval. | high |
| Ticket blocked | `#<id> blocked` | Work stopped and needs your decision. | high |
| PR ready | `#<id> ready for review` | A pull request is ready for your review. | default |
| PR merged | `#<id> merged` | The pull request was merged. | low |

The text is fixed board text. Nothing an agent wrote reaches a lock screen.
The board sends the same push for the same ticket and reason at most once in
10 minutes. Each push deep-links to the card and offers no action beyond
opening it. A person's own actions never notify them.
