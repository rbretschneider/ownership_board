# Security Review: Four lanes with stages (plan review)

**Story:** Four lanes with stages, checkpoints, flags and the design document (`docs/lanes.md`, proposed 2026-10-21)
**Reviewer:** Security Researcher Agent (Claude Opus 5.5, model id `claude-opus-5-5`), single agent, no subagents
**Date:** 2026-10-21
**Verdict:** APPROVE WITH CHANGES (LP-01 to LP-08 must be written into `docs/lanes.md` before the service layer is built)

This review covers the design. Code read: `docs/lanes.md`, `docs/design.md`, `docs/traceability.md` (Security decisions),
`docs/agents.md`, `internal/domain/{lane,ticket,work,lease,actor,limits,stage}.go`,
`internal/app/{board_service,agent_service,agent_rules,brief,review_service,ticket_writer,placement,notification_policy,trace_service}.go`,
`internal/httpapi/{router,handlers_board}.go` and the route registrations, `internal/mcp/{board_tools,loop_tools,server}.go`,
`internal/store/sqlite/code_repos.go`, `internal/store/sqlite/db.go`, `web/src/lib/markdown.ts`, `agent/claude/board-loop.md`,
and the prior reviews in `docs/security/`. Implementation work has already started: `internal/domain/stage.go`, a changed
`internal/domain/lane.go` and a draft migration `0005_lanes.{up,down}.sql` are in the working tree. LP-10 reviews that draft.

All guarantees in the traceability "Security decisions" section (SEC-01 to SEC-15) are still in force. Every finding below
either keeps one of them or extends it to the new surface.

---

## Threat model

### Assets
- **Checkpoint integrity.** The point of this story is that the owner signs off on a spec and (optionally) a design before code is written. A checkpoint an agent can get around is worse than having no checkpoint, because the owner believes it held.
- **State-machine integrity.** Lane and stage decide who may act next, what is claimable, what the brief tells the agent to do, and what "Needs you" shows.
- **The design document.** This is new private content. It is the most detailed description of the change, so it is the board content most likely to leak into a public repo. Once approved, it also carries the owner's authority in the build brief.
- **Approval records.** The decision log should show when the owner signed off and on what.
- **Existing data.** Every ticket, question round, decision and lease goes through a lane-id migration.
- **The owner's attention and privacy.** Pushes go to ntfy, which may be a third-party host.

### Threat actors
- **A prompt-injected agent (primary).** It holds a valid bearer token and may have read hostile text from a public repo, an issue or a dependency. It can call every agent tool and REST route, and it holds a lease on whatever it claimed.
- **An honest but eager agent.** It "helps" by finishing the flow itself, copies the ADD into `docs/` or the PR body, and reuses its lease across stages.
- **A stale or duplicate agent session.** Two loops run on one token name, or a pass is still running after the owner moved the ticket.
- **The owner acting on stale UI.** A phone sheet that is not up to date, a double tap, or a drag meant to reorder that also changes state.
- **Operators.** Deploying the migration while loops with the old prompt are still running.

### Attack surface introduced
- REST: `POST /tickets/{id}/approve`, `/request-changes`, `/unblock` (people); `PUT /spec {submit}`, `PUT /design {submit}`, `POST /block` (agents); `PATCH /repos/{id}` gains two gate fields.
- MCP: `save_spec.submit`, new `save_design`, new `mark_blocked`, a narrower `move_ticket`.
- New stored, agent-written text: `design` (up to the spec limit) and `blocked_reason`.
- New implicit transitions: claim (`todo/ready` to `in_progress/design`), submit with gates on or off, people's drop-to-stage defaults, and `report_pr_state` landing in `in_progress/build`.
- The loop's local copy of the design, kept outside the repo.
- Four new notification triggers.

### What already holds (verified in code)
- `ticketWriter.change` runs the lease check for every agent write inside a `BEGIN IMMEDIATE` transaction (`_txlock=immediate`). Owner and agent writes are serialised, and a claim's select plus update is atomic.
- `Answer`, deletes, token management and `PATCH /repos` are human-only, at the route and in the service.
- Cookie writes need a same-origin `Origin`. Bearer writes are limited to 120 per minute per token.
- The brief fences stored text, and its cap drops whole items (CR-01 is fixed).
- Markdown in the UI goes through `marked` and then `DOMPurify`.

---

## Findings

### HIGH

#### LP-01: No server-enforced transition table, so most agent operations are not stage-guarded
**Domain:** Business logic / state machine
**Location:** `docs/lanes.md` "Agent flow", "API summary"; `BoardService.SetSpec`, `Edit`, `Ask`, `Move`
**Description:** The plan lists what agents should do in each stage. It does not say what the server refuses. Today the only stage-like guard is `isUnrefinedHandoff`. `SetSpec`, `Edit` (including `body`, `kind` and `repo_id` over REST), `Ask`, `Comment` and `Move` work in any lane once the agent holds the lease. With stages, a lease in `design` or `build` is a lease on a ticket whose spec the owner has already approved.
**Attack scenario:**
1. The owner approves the spec for ticket #40. An agent claims it, and the ticket moves to `in_progress/design`.
2. The injected agent calls `save_spec` (no `submit`) and rewrites the scope, or uses `PATCH /tickets/40` to rewrite `body`. The lease check passes.
3. With the design gate off, it then submits a design for the new scope and builds it. The decision log still says "Spec approved", but the approval was for a different text.
4. A variant: in `build`, it calls `save_design` and replaces the approved design. Another: it calls `save_spec submit:true` from `build`. The semantics of that call are undefined, which is exactly the problem.

**Required resolution:** Add to `docs/lanes.md` an explicit transition table of (current stage, actor kind, operation) to (next stage, flag changes, lease effect). The service enforces it inside the write transaction, and anything not listed is refused (fail closed). At minimum:
- `save_spec` (with or without `submit`) only in `refine`. `save_design` only in `design`.
- `submit` is refused while open questions exist or the ticket is blocked. A spec submit is refused without a repo (this keeps the existing rule).
- An agent `move_ticket` to `review` only from `build`, only with `pr_url` set, no open questions and not blocked. Every other agent `move_ticket` is refused, including same-lane reorders.
- Agents may change `repo_id`, `body` and `kind` only in `refine`. After that, only `title`, `pr_url` and `branch`, and `pr_url` only in `build`.
- `ask_questions` and `mark_blocked` only in agent stages (`refine`, `ready` is never held, `design`, `build`).
- Lane and stage must not be able to disagree: either store only the stage and derive the lane (`stage.go` already says "a ticket's lane can always be derived from its stage"), or put a database constraint on the allowed pairs.

**Blocks:** Implementation

#### LP-02: Approvals are not bound to the content approved, so they can be raced, replayed or made stale
**Domain:** Business logic / integrity
**Location:** "Checkpoints"; `POST /tickets/{id}/approve {checkpoint}`; "People's moves" ("ready if the spec was approved", "build if a design exists")
**Description:** An approval names a checkpoint, not a version. Nothing records which spec or design text was approved. The plan also leaves open where "the spec was approved" is read from. If it is read from the "Spec approved" binding decision, people can delete it (SEC-10 tombstones) and it can be superseded. The decision log is the wrong place to keep gate state.
**Attack scenario:**
1. An agent submits spec v1, and the owner opens the sheet on a phone.
2. While the owner reads v1, a session with the same token name (LP-09), or one that kept its lease (LP-03), saves v2.
3. The owner taps Approve, and v2 is approved unseen.
4. Replay variant: the owner double-taps Approve, or a queued retry fires after a request-changes and resubmit. The second request approves a submission the owner never opened.

**Required resolution:**
- Each ticket keeps server-side, monotonic `spec_revision` and `design_revision` counters plus `approved_spec_revision` and `approved_design_revision`. The two approval fields are set only by the approve use case, and the server sets every one of these fields.
- `approve` and `request-changes` must carry the expected stage and revision. On a mismatch they fail with 409 and change nothing, so they are idempotent and cannot be replayed.
- Every gate check and drop-to-stage default reads these fields, never decisions. Deleting or superseding an approval decision neither grants nor revokes approval.
- The "Spec approved" and "Design approved" decisions are written by the server with weight `binding`, `decided_by` the person, a distinct `source` (for example `checkpoint`), and the revision number in the summary.
- The plan must state what happens when a person edits an approved spec or design on the sheet: either the approval follows the person's edit, or the approval is cleared. Agents can never edit after approval (LP-01).

**Blocks:** Implementation

#### LP-03: Asking no longer moves the ticket, so asking (and the other hand-offs) no longer drops the lease
**Domain:** Business logic / lease integrity
**Location:** "Open questions", "Blocked", submit to a review stage; `Ticket.MoveTo` is today the only thing that clears a lease on a hand-off
**Description:** The lease is dropped today only as a side effect of `MoveTo` (`Ask` moves to `answers`). Under the plan, `ask_questions` and `mark_blocked` keep the lane and stage, so unless the design says so, the agent keeps the lease. A submit into `spec_review` or `design_review` that is coded as "set stage" rather than "move" would also keep the lease.
**Attack scenario:**
1. The agent asks one question on ticket #22, which notifies the owner, and keeps the lease.
2. It saves a spec and submits with the question unanswered, so the owner gets a spec that ignores their pending answer.
3. Or it asks ten rounds in a row, each one a high-priority push.
4. Or, still holding the lease after its spec submit, it swaps the spec while the owner reviews (the LP-02 race, with no second session needed).

**Required resolution:** Write these invariants into the plan and enforce them in the domain:
- An active lease exists only while the stage is an agent stage, no questions are open, and the ticket is not blocked.
- `ask_questions`, `mark_blocked`, every submit, every move into an owner stage, and every stage change by a person clear the lease in the same transaction.
- The claim query itself excludes tickets with open questions, blocked tickets and owner-review stages. The UI or brief saying so is not enough.

**Blocks:** Implementation

#### LP-04: Agents can create tickets in any lane, which skips both checkpoints
**Domain:** Authorization / state machine
**Location:** `POST /api/v1/tickets` (`createTicket` passes `body.Lane` into `TicketDraft.Lane` for any caller); `domain.NewTicket`/`initialLane`; `CreateRelated`
**Description:** The REST create route is `anyone`, and it accepts a `lane` for agents as well as people (MCP `create_ticket` does not expose it, but REST does). Under the new model this lets an agent create a ticket already in `in_progress` (with whatever default stage the code gives it), `review` or `merged`. No lease is needed for a plain create.
**Attack scenario:**
1. An injected agent POSTs `/api/v1/tickets` with lane `in_progress` and a body that reads like a spec.
2. It calls `claim_work`. In-progress tickets are claimed first, so it gets that ticket back and builds it.
3. No spec or design was ever approved. The first time the owner sees the work is a PR, or a "merged" note if it then abuses LP-07.

**Required resolution:** Tickets created by agents always start in `todo/refine` (and the service refuses or ignores any lane or stage an agent sends). People may choose the starting lane. The starting stage for people follows the same fail-closed rule as a drop (LP-05). State this in the plan's API summary and enforce it in the service, not only in the handler (SEC-13).

**Blocks:** Implementation

#### LP-05: People's drag defaults can silently skip a checkpoint, including on a simple reorder
**Domain:** Business logic
**Location:** "People's moves"
**Description:** "In Progress → `build` if a design exists" checks for a design, not an approved one, and an agent can make a design exist with `save_design` without `submit`. "Dragging also clears Blocked" and "dropping into a lane sets a sensible stage" do not distinguish reordering inside a lane from moving between lanes. Reordering is the owner's main way to set priority.
**Attack scenario:**
1. Ticket #31 sits in `in_progress/design_review` (design gate on), or in `design` holding an unsubmitted agent design.
2. The owner drags it to the top of In Progress to prioritise it. The drop recomputes the stage to `build`, because a design exists.
3. The next claim builds a design the owner never approved. The same reorder also clears a Blocked flag the owner meant to keep.
4. A variant: an agent marks a ticket blocked with a reason that nudges the owner to "just drag it back to In Progress".

**Required resolution:**
- A reorder inside a lane never changes the stage, either flag, or the approvals.
- A drop into another lane picks the first stage in that lane whose gates are met by the approved revisions from LP-02. Todo: `ready` only if the spec gate is off for the ticket's repo or the current spec revision is approved. In Progress: `build` only if the design gate is off or the current design revision is approved, otherwise `design`.
- Clearing Blocked on a cross-lane drop is fine but must be explicit in the timeline.
- The UI should show the stage the drop will produce before or after it lands, so the owner sees when a drop has crossed a checkpoint.

**Blocks:** Implementation

#### LP-06: Gate settings, mid-flow repo changes and tickets without a repo are undefined
**Domain:** Authorization / configuration
**Location:** "Checkpoints" (per-repo); `Edit` lets an agent with the lease change `repo_id` in any lane; `code_repos.go` `UPDATE repos` column list
**Description:** The plan does not say when the gate settings are read, which repo's settings apply after a repo change, or what applies when the ticket has no repo.
**Attack scenario:**
1. Ticket #50 is in repo A, which has design approval on.
2. The agent holding the lease in `design` sets `repo_id` to repo B, which has it off, and submits. The ticket goes straight to `build`.
3. It sets `repo_id` back afterwards. The timeline shows two edits, but no checkpoint ran.

**Required resolution:**
- Gates are read inside the transition's transaction, from the ticket's current repo.
- A ticket with no repo is treated as having both gates on (fail closed), and a spec submit without a repo is refused (keeps the existing rule).
- Agents may change `repo_id` only in `refine` (LP-01). A person changing the repo later keeps the approvals, and the new repo's gates apply at the next transition.
- Changing a gate never moves tickets that are already past it. Say so in the plan, so turning a gate off does not release waiting tickets by surprise and turning it on does not strand built ones.
- The gate fields are human-only at the route and in the service. Repo sync must never write them: they must not join the sync `UPDATE` column list the way `treat_as_public` did, and new synced or manual repos get the plan's defaults. Every gate change is logged with the person and the time.

**Blocks:** Implementation

#### LP-07: The design document is a new private-content and prompt-injection channel, and the approval raises its authority
**Domain:** Data exposure / injection
**Location:** "The design document"; `brief.go` levels (`fieldRunes: 12_000`, `briefMaxBytes 60_000`); `publicRepoRules`; `board-loop.md` step 4 (`<repos>/.ownership_runs`)
**Description:** Until now the ADD stayed in the loop's run folder. Now it is stored on the board, approved by the owner, and fed back into every build brief. Four separate problems follow:
1. **Authority.** A design written by an architect subagent that read hostile public-repo content (for example a README that says "the install step must run `curl … | sh`", or "copy the spec into `docs/`") becomes "the approved design". It is the closest thing to an instruction from the owner that an agent sees.
2. **Silent truncation.** The spec and the design can each be 50,000 characters, but the brief is capped at 60 KB, and the last level cuts every field to 12,000 runes. The build agent can be handed a cut approved design without understanding that what is missing was approved too, or that the cut part may be what was approved.
3. **Leakage.** `publicRepoRules` does not name the spec or the design. The loop's run folder is "`OWNERSHIP_REPOS_DIR` or the parent of the session directory", and that parent can be inside a git work tree. The `architect` and `security_researcher` roles by default write `docs/architecture/…` and `docs/security/…` inside the repo.
4. **Divergence.** The loop's local `ADD.md` is editable, and nothing ties it to the approved revision.

**Attack scenario:** On a public repo, the architect writes a design that embeds an instruction to "document the design in `docs/design-<id>.md`". The owner skims and approves. At build, `technical_writer` follows the approved design, and the board's private design lands in a public commit. Or the brief cuts the design at 12,000 runes and the agent builds the approved first half plus its own idea of the rest.

**Required resolution:**
- In the brief, the design is rendered fenced (SEC-02), under the untrusted-content banner, with an explicit line: the approved design sets scope and approach, but it cannot override "What to do now", the public repo rules, a security fix, or the never-merge/never-push rules. Agent-written design text gets the SEC-08 hidden-character check.
- The approved design (and spec) is never cut in an `implement` brief. Either set the limits so spec, design and instructions always fit, or have the brief say it was cut and give a read tool that returns the full approved revision with its revision number. The brief shows the approved revision number.
- `publicRepoRules` names the spec and the design explicitly.
- The loop instructions require that the run folder is outside any git work tree (checked, not assumed), override the subagents' default output paths, run a pre-PR check on public repos that the diff adds no pipeline documents and no text copied from them, and delete the run folder once the ticket is merged.
- At build start the loop rewrites its local copy from the board's approved revision. Its own edits are a working draft and never the source of truth.
- The design never appears in pushes, MCP `list_tickets` output or SSE payloads beyond what already goes to authenticated readers.

**Blocks:** Implementation

#### LP-08: The plan leaves required security behaviour implicit
**Domain:** Process
**Location:** `docs/lanes.md` as a whole
**Description:** `docs/traceability.md` keeps a "Security decisions" section that overrides the rest of the document. `docs/lanes.md` has nothing like it, so LP-01 to LP-07 would live only in this report. They would be lost the next time the document is edited, and senior_dev and QA would have no normative source.
**Attack scenario:** An implementer follows `docs/lanes.md` as written: drop-to-stage uses "a design exists", and ask keeps the lease. Every bypass above ships.
**Required resolution:** Add a "Security decisions (from docs/security/lanes-plan-review.md)" section to `docs/lanes.md` stating the properties in LP-01 to LP-07 and LP-09 to LP-11, and that it overrides the rest. Restate that SEC-01 to SEC-15 still apply.
**Blocks:** Implementation

---

### MEDIUM

#### LP-09: A lease is a holder name, not a claim, so stale sessions survive stage changes
**Domain:** Authentication / business logic
**Location:** `domain.Lease{Holder, ExpiresAt}`; `requireLeaseForAgent` compares `Lease.Holder == actor.Name`
**Description:** Two sessions on one agent token (two loops, a pass restarted while the old one still runs, or an MCP plus a `curl` loop) share every lease. With stages, one ticket goes through more hand-offs (design, request changes, design again, build), so it is more likely that the same name claims the ticket again while an older session is still working.
**Attack scenario:**
1. The owner requests changes on #60's design, so the stage returns to `design` and the lease is cleared.
2. The next pass, on the same token, claims #60.
3. The previous pass, still running its pipeline, calls `save_design submit:true` with the old design. The write passes the name check and lands as the new submission.

**Required resolution:** A claim returns a lease id (a fencing token) bound to ticket and stage. Every write that needs a lease must present it, and it is a tool argument on MCP. Any stage change or new claim invalidates the old one, and `heartbeat` refuses a lease whose stage has changed. This adds to "a lease on every agent write"; it does not replace it.
**Blocks:** QA sign-off

#### LP-10: Migration correctness and safety (reviews the draft `0005_lanes`)
**Domain:** Data integrity
**Location:** `internal/store/sqlite/migrations/0005_lanes.{up,down}.sql` (draft), "Migration"
**Description and required resolution:**
1. **Unknown values fail open.** Both `CASE` expressions end with `ELSE 'refine'` / `ELSE 'todo'`, so a corrupt or unexpected lane becomes claimable work. The migration must fail on any lane outside the six known ids rather than map it.
2. **No pair constraint.** `stage` defaults to `'refine'` for any lane, so a code path that inserts with a lane and no stage creates `in_progress/refine`. Add a database-level guarantee of valid (lane, stage) pairs, or store only the stage (LP-01).
3. **Old `ready` tickets were never approved.** Under the old flow, an agent moved tickets to `ready` on its own. Mapping them to `todo/ready` while spec approval defaults to on lets them be built without sign-off. Decide explicitly: map agent-moved `ready` tickets (the last `moved` entry by an agent) to `todo/spec_review`, or document that they are grandfathered and list them for the owner after the migration. Old `in_progress` tickets with no approved design are fine while the design gate is off. Write that down, so turning the gate on later does not retroactively matter (LP-06).
4. **Leases.** Clear every lease in the migration and deploy with loops stopped. A session holding an old lease runs the old prompt (`move_ticket ready`, `resume`).
5. **Approval columns.** The LP-02 revision and approval fields belong in the same migration, so there is no window where gates exist but approvals cannot be recorded.
6. **Ranks.** Three lanes merge into `todo` and their ranks interleave, so the owner's priority order changes. Renumber `todo` in a defined order (old `new`, then `answers`, then `ready`, each in its own rank order) and say so.
7. **Rollback loses data and reopens gates.** The down migration drops `design` and `blocked_reason` without warning and returns to a schema where agents move tickets freely. Take an automatic backup before the up migration. The down migration must refuse while any design or blocked reason is non-empty, or the docs must say the data is lost.
8. **Old clients.** Update `agent/claude/board-loop.md`, `board-work.md`, `docs/agents.md` and the MCP `serverInstructions` (which still list the old lanes) in the same release. The API must reject old lane ids with an error that explains the new flow, never map them silently.
9. **QA** must verify that restoring a pre-0005 backup into the new binary runs 0005, and that history entries with old lane ids still render.

**Blocks:** QA sign-off (items 1 to 5 before the migration ships)

#### LP-11: Notifications carry agent text off the LAN, and new triggers multiply pushes
**Domain:** Data exposure / abuse
**Location:** "Notifications"; `questionsAsked` puts `event.Detail` (the agent's summary) in the push body
**Description:** ntfy may be a third-party host. The new triggers (spec submitted, design submitted, blocked) invite templates that include the blocked reason or an excerpt of the spec or design. All of those are agent-written private content, so they could carry injected text or leak data. Repeated block, unblock, re-block cycles, or many tickets blocking in one pass, each produce a high-priority push.
**Attack scenario:** An injected agent blocks five tickets in one pass, each with a reason that quotes private design text. Five high-priority pushes put that text on the ntfy host and on the lock screen.
**Required resolution:**
- The new pushes use fixed text written by the board: ticket number plus "spec ready", "design ready" or "blocked". No reason, spec or design text.
- Coalesce per ticket (one push per hand-off, repeats within a window collapse) and cap pushes per agent token per window.
- Never make approve, request-changes or unblock a push action, and never put a credential in a push URL.
- The quiet "merged" note names the agent that reported it.
- SEC-14 (decisions never notify) covers the approval decisions too.

**Blocks:** QA sign-off

#### LP-12: `report_pr_state` is an unverified, lease-free path past the `pr` checkpoint and back into `build`
**Domain:** Business logic
**Location:** "Agent flow" (`report_pr_state`: review to merged, or to in_progress/build); `review_service.go`
**Description:** Any agent token can report on any review ticket, and nothing checks the claim. "Merged" takes the ticket out of Needs you. "Closed" puts it in `in_progress/build`, where it is claimable at once. The new design makes `pr` an explicit owner checkpoint, and this path is a way past it.
**Attack scenario:** An injected agent reports "closed" on #70 while the owner is still reviewing the PR. The ticket is claimed and rebuilt from the old design, and a second PR appears. Or it reports "merged" on an unmerged PR, and the ticket leaves the owner's "Your move" list.
**Required resolution:**
- Act only when the stage is `pr`.
- "Closed" lands in `in_progress/build` with Blocked set and a board-written reason ("PR closed without merging; confirm next step"), so a person decides what happens next. If the design gate is on, unblocking offers "Request design changes".
- "Merged" records the reporting agent and the PR URL host check (SEC-07). The UI shows "reported by agent".
- This stays the only write that needs no lease, and the plan says so.

**Blocks:** QA sign-off

---

### LOW / INFORMATIONAL

#### LP-13: Answers must never count as an approval
An agent can ask "Approve this spec as written?", recommend "Yes", and the owner taps **Accept recommendations**. The plan should state that only the approve use case changes a checkpoint stage. No answer, decision or comment can satisfy a gate, and the brief says that answers to such questions are not approvals. (They are recorded as `delegated` under SEC-03.)

#### LP-14: People-only endpoints are enforced in the service too
`approve`, `request-changes`, `unblock` and the gate fields need a human actor in the service layer, not only `rt.human` routing (SEC-13). They are POST only, with the same-origin check, and unknown fields are rejected. The request-changes comment is stored as the person's comment, and the next brief shows it as the newest comment.

#### LP-15: Agent decisions can look like approvals
`record_decision` lets an agent record "Spec approved" with weight `agent`. The UI shows the kind (CR-12), but the plan should require checkpoint decisions to be visually distinct (`source: checkpoint`) and say again that gate state never comes from decisions (LP-02).

#### LP-16: The blocked reason is new agent-written text
Apply the comment length limit and the SEC-08 hidden-character check, fence it in the brief, show it on one line in `list_tickets` and on cards, and cover it in the public repo rules.

#### LP-17: Accepted risk: every agent token can read every design
`get_ticket` and claim briefs expose the design of any ticket to any agent token, as they already do for specs. This is acceptable for a single-owner board. Note it in the docs next to SEC-15.

---

## Positive observations
- Agents losing free lane moves is the right direction and closes the old "agent moves itself to ready" path. With LP-01 and LP-04 it becomes a real guarantee.
- People-only approval, plus a required comment on request-changes, fits the existing human/agent split, and the binding-decision record fits SEC-01.
- The design keeps `report_pr_state` as the single lease-free write and keeps it narrow.
- Keeping the design out of repos and PRs is stated up front, and the loop already keeps pipeline documents out of worktrees.
- Spec approval on and design approval off for existing repos is a safe default.
- `BEGIN IMMEDIATE` write transactions already make an owner approval and an agent claim impossible to interleave, as long as every check above runs inside the transaction.

## Conditions for QA
1. A table-driven test over every (stage, actor, operation) pair: everything not listed in the LP-01 table is refused.
2. A stale-revision approve returns 409, and a double approve is a no-op.
3. `ask_questions`, `mark_blocked` and every submit clear the lease. A ticket with open questions or blocked is never returned by claim.
4. An agent create with any lane lands in `todo/refine`.
5. Reordering within In Progress leaves the stage and flags unchanged. A drop into In Progress with an unapproved design lands in `design`.
6. An agent `repo_id` change outside `refine` is refused. A ticket without a repo cannot submit a spec. Repo sync does not change the gate fields.
7. A brief with a 50,000-character spec and a 50,000-character design either holds both in full or says plainly that they were cut and how to fetch them.
8. Migration: an unknown lane aborts, leases are cleared, ranks are renumbered, and the rollback behaviour matches the docs.
9. Pushes for blocked, spec and design contain no agent text.
10. `report_pr_state closed` leaves the ticket blocked.

## Verdict rationale
**APPROVE WITH CHANGES.** Nothing is CRITICAL. Every checkpoint bypass still ends in a PR the owner must merge on the code host, and the existing lease, binding-decision, fencing and public-repo guarantees carry over. But LP-01 to LP-07 are real bypasses of the checkpoints this story exists to add, or real leaks of its new private content. Each has a clear resolution inside the current architecture, and none needs a change to the business requirements. They must be written into `docs/lanes.md` (LP-08) before the service layer is built. The draft migration needs LP-10 items 1 to 5 before it ships. LP-09, LP-11 and LP-12 must be resolved before QA sign-off.

Reviewed by Claude Opus 5.5 (`claude-opus-5-5`).
