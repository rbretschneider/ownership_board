# Connect GitHub (one-time setup)

The board lists your repos through a GitHub App that can only read repository
metadata (names, descriptions, default branches). It cannot read code or write
anything. You register the App once; after that Settings > Repositories has a
**Connect GitHub** button that works like signing in to GitHub Desktop.

1. On GitHub: **Settings → Developer settings → GitHub Apps → New GitHub App**.
2. **GitHub App name:** anything unique, such as `ownership-board-<you>`.
   **Homepage URL:** your board's address (any URL works).
3. **Identifying and authorizing users:** leave *Callback URL* empty, keep
   **Expire user authorization tokens** ticked, leave *Request user
   authorization during installation* unticked, and tick **Enable Device Flow**.
4. **Webhook:** untick *Active*.
5. **Permissions:** under *Repository permissions*, leave **Metadata: Read-only**
   (the default) and add nothing else. No account or organization permissions.
6. **Where can this GitHub App be installed?** *Only on this account*. Choose
   *Any account* only if you need to install it on an organization; that lets
   other accounts install your app, which grants them nothing on your board.
7. **Create GitHub App**, then copy the **Client ID** (it starts with `Iv`).
   You do not need a client secret or a private key.
8. **Install App** (left menu) → your account → **All repositories**, so
   repos you create later are included automatically. Repeat for each
   organization whose repos you want.
9. Put `GITHUB_APP_CLIENT_ID=<the Client ID>` in the board's `.env` and
   restart it. Open Settings > Repositories > **Connect GitHub**, enter the
   code at github.com/login/device, and confirm the account the board shows.

The board refreshes its token by itself (tokens last 8 hours, renewal lasts 6
months) and re-syncs every hour. Disconnect in Settings forgets the board's
copy; to revoke the grant on GitHub too, remove the app under **GitHub →
Settings → Applications → Authorized GitHub Apps**.

Forgejo/Gitea and Bitbucket have a token form under **Add another provider**.
Use a Forgejo token with `read:user` and `read:repository` only, or a Bitbucket
scoped API token with `read:repository:bitbucket` and `read:user:bitbucket`.

# Running an agent

An agent is any program holding an agent token. It claims a ticket, reads the
brief, does the work, and reports back. This page sets that up for Claude
Code, but nothing in the board is specific to one agent.

## 1. Create a token

Settings > Agent tokens > New token. Name it after the agent (the name is
what the timeline shows and what holds leases), and copy the secret: it is
shown once.

## 2. Connect Claude Code to the board

```bash
claude mcp add --transport http ownership https://board.example.com/mcp \
  --header "Authorization: Bearer ob_pat_..."
```

The MCP server offers these tools:

| Tool | Does |
|------|------|
| `claim_work` | Claims the next ticket and returns its brief. `repo` limits it to one repo; `wait_seconds` (up to 300) blocks until work appears. |
| `get_ticket` | Reads a brief without claiming. |
| `list_repos` | Lists repos with their ids, to work out where a ticket belongs. |
| `ask_questions` | Posts a round of questions. The ticket keeps its lane and stage, the human is notified, and your lease ends. The ticket is claimable again once every question is answered. |
| `save_spec` | Saves the refined spec while refining. With `submit: true` the ticket goes to the owner for approval (or straight to Ready when the repo has Approve spec off) and your lease ends. A submit needs a repo on the ticket. |
| `save_design` | Saves the design while designing. With `submit: true` the ticket goes to the owner for approval (or straight to build when the repo has Approve design off) and your lease ends. |
| `mark_blocked` | Stops work when you cannot go on and no question would help. Takes a short reason. The owner is notified, your lease ends, and nobody claims the ticket until a person unblocks it. |
| `update_ticket` | Sets `repo_id`, `pr_url`, `branch` or `title`. `title` and `repo_id` only while refining; `pr_url` and `branch` only while building. |
| `move_ticket` | Moves a finished build to `review`. Needs `pr_url` set, no open questions and no block. This is the only lane move an agent makes, and it ends your lease. |
| `add_comment` | Adds a comment. |
| `heartbeat` / `release_ticket` | Keeps or gives up the lease. |
| `create_ticket` | Files a follow-up story or bug, optionally already linked (`relation` + `related_ticket_id`). |
| `add_work_link` | Records a PR or commit for a ticket you hold. |
| `record_decision` | Records a significant choice; agents cannot replace the owner's binding decisions. |
| `link_tickets` | Relates two tickets (`caused_by`, `follow_up_of`, `duplicate_of`, `relates_to`). |
| `find_tickets_for_work` | Commit sha or PR URL → the tickets it was recorded on. |
| `list_tickets` | Read-only overview, optionally by lane or repo. |
| `report_pr_state` | For a ticket in review: `merged` → Merged, `closed` → back to build, blocked until the owner decides, `open` → no change. The only agent write that needs no lease, and it only ever moves review tickets along those two edges. |

**Traceability rules agents follow** (they are also in every brief): end each
commit with the trailer `Ownership-Ticket: <id>`, record commits and PRs with
`add_work_link`, record significant choices with `record_decision`, and on a
public repo keep all board text (titles, questions, answers, decisions,
comments) out of commits, branches and PRs. These are instructions, not
enforcement: review each PR before merging.

## 3. Pick a loop

**The full pipeline loop.** `agent/claude/board-loop.md` does one pass over
the whole board each time it runs:

1. Checks every PR in review with `gh` and reports merged (→ Merged) or closed
   (→ build, blocked).
2. Claims one ticket. Tickets to refine go to a `business_developer`
   subagent on a model sized to the ticket (haiku, sonnet or opus). It asks
   questions, or the loop saves the spec with `submit: true`.
3. Tickets to design (claimed from Ready, or resumed) go to `architect`, then
   `security_researcher`. The loop saves the design with `submit: true`. It
   stops there; the build is a later pass.
4. Tickets to build go through `senior_dev` → `qa_engineer` →
   `security_researcher` → `technical_writer` in a separate git worktree,
   then `senior_dev` opens the PR with the `pr` skill and waits for green CI.
   Each stage runs on a model picked by the spec's build tier (small, normal
   or large), and two QA failures move the build up a tier.
5. Reports work links, the PR and a summary to the board and moves the ticket
   to review.

The pass itself only coordinates: it calls board tools and `gh pr view`, and
hands all repo work to subagents. So it can run on a small model, for example
`claude -p "/board-loop" --model haiku` from a headless runner.

Pipeline documents (FRD, ADD, reviews, QA reports) live in a run folder
outside every git work tree and never go into a repo, PR or commit. The loop
rebuilds them from the board's spec and design on every claim.

It expects those subagents and the `pr` skill to exist in your Claude Code
setup, and `OWNERSHIP_REPOS_DIR` (or the session's parent folder) to hold your
clones. Copy it to `~/.claude/commands/` and run:

```
/loop /board-loop
```

With no interval, Claude paces itself. It never merges, deploys or pushes to a
default branch. Deploys happen through the repo's CI/CD.

**Inside an interactive session.** Copy `agent/claude/board-work.md` to
`.claude/commands/` in the repo (or `~/.claude/commands/` for every repo),
then run it on a loop:

```
/loop /board-work acme/api
```

Each run long-polls for up to five minutes, so an idle loop wakes rarely.

**Headless, with no model tokens spent while idle.** `agent/agent-loop.sh`
long-polls the board with `curl` and starts `claude -p` only once it holds a
ticket:

```bash
cd ~/src/api
OWNERSHIP_URL=https://board.example.com OWNERSHIP_TOKEN=ob_pat_... \
  CLAUDE_ARGS="--permission-mode acceptEdits" \
  /path/to/ownership_board/agent/agent-loop.sh acme/api
```

Run one loop per repository. A loop given a repo also refines tickets that
have no repo yet, because working out the repo is part of refinement.

## How a ticket flows

1. The human creates a ticket. It lands in **Todo**, stage `refine`.
2. The agent claims it with action `refine` and either asks questions or saves
   a spec with `save_spec` and `submit: true`.
3. Asking does not move the ticket. The human's phone buzzes. Answering the
   last question makes the ticket claimable again in the same stage, with the
   answers in the brief.
4. A submitted spec goes to `spec_review` (Spec ready, **Needs you**) when the
   repo has Approve spec on. The human taps **Approve**, or **Request
   changes** with a comment, which sends the ticket back to `refine` and puts
   the comment in the next brief. With Approve spec off, the ticket goes
   straight to `ready`.
5. The agent claims it from `ready` with action `design`. The ticket moves to
   **In Progress**, stage `design`, and is leased to that agent. The agent
   saves the design with `save_design` and `submit: true`.
6. A submitted design goes to `design_review` when the repo has Approve design
   on, with the same Approve and Request changes choices (a rejected design
   returns to `design`). With Approve design off, it goes straight to `build`.
7. The agent claims it from `build` with action `implement`. It opens a pull
   request, links it, comments, and calls `move_ticket` to `review`. The human
   is notified with a button to open the PR.
8. The human merges. The agent's next pass sees the merge and calls
   `report_pr_state` with `merged`, which moves the ticket to **Merged**. The
   repo's own CI/CD deploys it. A person can also drag the ticket to Merged.
9. If the PR is closed without merging, the ticket returns to `build`,
   blocked. The human comments and unblocks it to have the agent rebuild, or
   drags it elsewhere.

At any step an agent that cannot continue calls `mark_blocked` with a reason.
The ticket shows **Needs you** until a person unblocks it. Agents cannot
choose a lane or stage: the board moves the ticket when the agent hands it on.

A lease lasts 30 minutes unless renewed with a heartbeat. Every hand-off ends
the lease. If an agent dies, the lease expires and the ticket is claimable
again. Leases belong to the token name, so run one agent token per loop.

## Plain HTTP

Everything the MCP tools do is also in the REST API ([api.md](api.md)) with
the same token as a bearer token, for agents that do not speak MCP.
