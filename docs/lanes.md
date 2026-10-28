# Four lanes with stages (design)

Status: approved with changes, 2026-10-21. Replaces the six lanes in
docs/design.md. **The "Security decisions" section at the end overrides
anything above it that disagrees** (from docs/security/lanes-plan-review.md).

## Why

Three of the six lanes were not stages of work but states of the ball
("waiting on the owner", "refined, waiting to build"). They made the owner
scan columns to find their move, forced a ticket out of In Progress when an
agent asked a mid-build question, and needed horizontal scrolling. The owner
acts as product owner and architect, so what matters is: what needs me, what
is being worked on, what do I review.

## Lanes (fixed, as before)

| Lane | Id | Meaning |
|---|---|---|
| Todo | `todo` | Not started: being refined, or refined and waiting to be built |
| In Progress | `in_progress` | Being designed or built |
| Review | `review` | A pull request is open |
| Merged | `merged` | The pull request merged (deployment is the repo's own CI/CD) |

All four columns fit side by side at 1000 px wide or more (a vertical 1080p
monitor). Columns are at least 230 px wide. Phones keep the tab strip, now four tabs.

## Stages (sub-states)

| Lane | Stage | Whose move | Shown as |
|---|---|---|---|
| todo | `refine` | agent | `Refining` |
| todo | `spec_review` | owner | `Spec ready` + **Needs you** |
| todo | `ready` | agent | `Ready to build` |
| in_progress | `design` | agent | `Designing` |
| in_progress | `design_review` | owner | `Design ready` + **Needs you** |
| in_progress | `build` | agent | `Building` |
| review | `pr` | owner | `PR open` + **Needs you** |
| merged | `done` | nobody | none |

Two flags sit across stages:

- **Open questions.** Asking no longer moves a ticket. The ticket keeps its
  lane and stage, shows **Needs you**, and is not claimable until every
  question is answered; then the next claim resumes the same stage with the
  answers in the brief.
- **Blocked.** An agent that cannot continue (the security review keeps
  rejecting a design, QA fails twice) marks the ticket blocked with a reason.
  It shows `Blocked` + **Needs you** and is not claimable until a person
  unblocks it.

**Needs you** = open questions, or blocked, or stage `spec_review`,
`design_review` or `pr`. The "Your move" filter shows exactly these.

## Checkpoints

Two per-repo settings, changed only by people:

- **Approve spec** (default on). After refinement the ticket goes to
  `spec_review` and waits. Off: straight to `ready`.
- **Approve design** (default off). When building starts the agent first
  writes a design (stage `design`), saves it on the ticket, and waits in
  `design_review`. Off: the design step still runs inside the agent's own
  pipeline but goes straight to `build` without waiting.

A person resolves a checkpoint from the ticket sheet with **Approve** or
**Request changes** (a comment is required). Request changes sends the ticket
back to `refine` (spec) or `design` (design) and the comment is in the next
brief. Approvals are recorded as binding decisions ("Spec approved",
"Design approved") so the decision log shows when the owner signed off.

**Autonomous tickets** have no checkpoints at all. A delegate agent token
files one with a spec the owner agreed in a chat; it starts at `ready` with
that spec approved, and the board refuses questions on it. Only a person turns the flag on or off,
and only in `ready`, `design` or `build`; an agent cannot change an
autonomous ticket's repo. See [agents.md](agents.md#autonomous-tickets).

## The design document

Tickets gain a `design` field (markdown, same limits as the spec), written by
the agent with `save_design`. It is private board content: the loop keeps a
local copy outside the repo for its pipeline, and the public-repo rules forbid
copying it into a repo, PR or commit. It is shown on the ticket sheet under
the spec and in the brief.

## Agent flow

Claims, most urgent first: In Progress tickets in `design` or `build`, then
Todo `refine`, then Todo `ready`. Tickets with open questions, blocked, or in a
review stage are never claimable.

| Claimed from | Action | Agent ends with |
|---|---|---|
| todo / refine | `refine` | `ask_questions`, or `save_spec` with `submit: true` → `spec_review` or `ready` |
| todo / ready | `design` (ticket moves to in_progress / design) | `save_design` with `submit: true` → `design_review` or `build` |
| in_progress / design | `design` (resumed) | same |
| in_progress / build | `implement` | PR, work links, `move_ticket` to `review` |

Agents can no longer move tickets between lanes freely: the only lane move an
agent makes is in_progress/build → review. Everything else goes through
`save_spec`/`save_design` with `submit`, `ask_questions`, `mark_blocked` and
`report_pr_state` (review → merged, or review → in_progress/build when closed).

## People's moves

Dragging is still free. Dropping into a lane sets a sensible stage: Todo →
`refine` (or `ready` if the spec was approved), In Progress → `build` if a
design exists else `design`, Review → `pr`, Merged → `done`. Dragging also
clears Blocked. People resolve checkpoints with Approve / Request changes.

## Migration

| Old lane | New lane / stage |
|---|---|
| new | todo / refine |
| answers | todo / refine (questions stay open) |
| ready | todo / ready |
| in_progress | in_progress / build |
| review | review / pr |
| done | merged / done |

Existing repos get Approve spec on and Approve design off.

## Notifications

The owner is pushed when the ball lands with them: questions asked, a spec or
design submitted for approval, a ticket blocked, a PR ready for review. Merged
sends a quiet note.

## API summary

- `Lane` ids change as above; `TicketSummary` gains `stage`, `needs_owner`,
  `blocked_reason`; `TicketDetail` gains `design`.
- `Repo` gains `require_spec_approval`, `require_design_approval`
  (`PATCH /api/v1/repos/{id}`, people only).
- People: `POST /api/v1/tickets/{id}/approve {checkpoint: "spec"|"design", revision}`,
  `POST /api/v1/tickets/{id}/request-changes {checkpoint, revision, comment}`,
  `POST /api/v1/tickets/{id}/unblock {comment?}`.
- Agents (lease required): `PUT /spec {spec, submit}`,
  `PUT /design {design, submit}`, `POST /block {reason}`.
- MCP: `save_spec` and new `save_design` gain `submit`; new `mark_blocked`;
  `move_ticket` only allows → review for agents.

## Security decisions

These override everything above.

### State machine (LP-01, LP-03, LP-04, LP-05)

The stage is the source of truth; the lane is always derived from it in one
place and checked on every load. The server allows only these transitions and
refuses everything else:

| From stage | Operation | Who | To | Also |
|---|---|---|---|---|
| (new ticket) | create | anyone | refine | agents can never choose a lane or stage |
| refine | `save_spec` (no submit) | lease holder | refine | spec revision +1 |
| refine | `save_spec` submit | lease holder, no open questions | spec_review if the spec gate is on, else ready | lease cleared |
| refine, design, build | `ask_questions` | lease holder | same stage | lease cleared |
| refine, design, build | `mark_blocked` | lease holder | same stage, blocked | lease cleared |
| ready | claim | agent | design | lease taken |
| design | `save_design` (no submit) | lease holder | design | design revision +1 |
| design | `save_design` submit | lease holder, no open questions | design_review if the design gate is on, else build | lease cleared |
| build | move to review | lease holder, `pr_url` set, no open questions, not blocked | pr | lease cleared |
| pr | `report_pr_state` merged | agent | done | comment names the agent |
| pr | `report_pr_state` closed | agent | build, **blocked** ("PR closed without merging") | a person decides what happens next |
| spec_review | approve (revision) | person | ready | binding approval decision |
| spec_review | request changes (revision, comment) | person | refine | |
| design_review | approve (revision) | person | build | binding approval decision |
| design_review | request changes (revision, comment) | person | design | |
| any blocked | unblock (comment optional) | person | same stage | |
| any | drag to another lane | person | the first stage in that lane whose gates are met by **approved revisions** | lease cleared, blocked cleared |
| any | reorder within a lane | person | same stage | nothing else changes |

Agent field edits are limited by stage: title, body, kind and repo only in
refine; branch and `pr_url` only in build. Comments, work links and decisions
need the lease in any agent stage. A lease exists only in an agent stage
(refine, design, build) with no open questions and not blocked; the claim
query uses the same conditions.

### Approvals bind to a revision (LP-02, LP-13, LP-15)

Tickets carry `spec_revision`, `design_revision`, `approved_spec_revision` and
`approved_design_revision`, all set by the server. Approve and request changes
must send the stage's checkpoint and the revision the person was shown; a
mismatch (the text changed, or a double tap) answers 409 and changes nothing.
Gates are checked against these fields, never against the decision log.
Answers never satisfy a checkpoint. Approval decisions have source `approval`
and weight `binding`.

### Gates (LP-06)

Gates are read inside the transition's transaction. A ticket with no repo has
both gates on (fail closed). Changing a gate never moves tickets that are past
it. Repo sync never writes the gate columns; only `PATCH /repos/{id}` by a
person does.

### The design document (LP-07)

The design is fenced in the brief like every stored field, refused from agents
if it contains hidden characters, and the brief says an approved design cannot
override "What to do now", the public repo rules or a security fix. Spec and
design are each capped at 20,000 characters and **never cut** in a brief; the
brief limit is 120 KB and other sections shrink first. The public repo rules
name the spec and the design. The loop checks its run folder is outside every
git work tree, tells its subagents where to write, refuses to open a PR whose
diff contains a pipeline document, and rebuilds its local copy from the
board's approved revision.

### Migration (LP-10)

Unknown lanes abort the migration (no catch-all). Old Ready tickets go to
`spec_review`, since nobody approved them. Leases are cleared. Revisions start
at 1 where text exists. In Todo, old Ready tickets sort first, then New, then
old Pending Answers, each keeping its order. The down migration warns that it
drops designs, blocked reasons and revisions.

### Notifications (LP-11)

Pushes use fixed board text with no agent-written text, are deduplicated per
ticket and reason for 10 minutes, and never offer an approve action.

### Accepted for now

- **LP-09 (lease fencing):** leases are held by token name, so two loops
  sharing one token share leases. Run one agent token per loop.
- **LP-17:** every agent token can read every design, as with specs.

## API contract (final)

```ts
type LaneId = "todo" | "in_progress" | "review" | "merged";
type StageId = "refine" | "spec_review" | "ready" | "design" | "design_review" | "build" | "pr" | "done";
type Lane = { id: LaneId; label: string; description: string };
type Stage = { id: StageId; lane: LaneId; label: string; is_owner_review: boolean };

type Board = { lanes: Lane[]; stages: Stage[]; repos: Repo[]; tickets: TicketSummary[] };

type TicketSummary = {
  // ...existing fields, with lane now a LaneId above
  stage: StageId;
  owner: "agent" | "human" | "nobody";   // human whenever needs_owner
  needs_owner: boolean;                    // open questions, blocked, or an owner-review stage
  blocked_reason: string;                  // "" when not blocked
  spec_revision: number; design_revision: number;
};
type TicketDetail = TicketSummary & {
  // ...existing fields
  design: string;
  approved_spec_revision: number; approved_design_revision: number;
  gates: { spec: boolean; design: boolean };   // effective for this ticket (no repo = both true)
};
type Repo = { /* ...existing */ require_spec_approval: boolean; require_design_approval: boolean };
```

| Method | Path | Body | Who |
|---|---|---|---|
| POST | `/api/v1/tickets/{id}/approve` | `{checkpoint: "spec"\|"design", revision}` | person; 409 if stage or revision changed |
| POST | `/api/v1/tickets/{id}/request-changes` | `{checkpoint, revision, comment}` | person; comment required |
| POST | `/api/v1/tickets/{id}/unblock` | `{comment?}` | person |
| POST | `/api/v1/tickets/{id}/move` | `{lane, after_id}` | as before; the server picks the stage |
| PUT | `/api/v1/tickets/{id}/spec` | `{spec, submit?}` | agent with lease, stage refine |
| PUT | `/api/v1/tickets/{id}/design` | `{design, submit?}` | agent with lease, stage design |
| POST | `/api/v1/tickets/{id}/block` | `{reason}` | agent with lease |
| PATCH | `/api/v1/repos/{id}` | adds `require_spec_approval?`, `require_design_approval?` | person |
| POST | `/api/v1/tickets` | `lane` is no longer accepted; tickets start in todo/refine | anyone |

New timeline types: `stage` (data `{from, to}`), `approved` / `changes_requested`
(data `{checkpoint, revision}`, body = comment), `blocked` (body = reason),
`unblocked`, `design`.
