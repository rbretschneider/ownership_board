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
| `ask_questions` | Posts a round of questions. The ticket moves to Pending Answers and the human is notified. |
| `save_spec` | Saves the refined spec. |
| `update_ticket` | Sets `repo_id`, `pr_url`, `branch` or `title`. |
| `move_ticket` | Moves a ticket to a lane. |
| `add_comment` | Adds a comment. |
| `heartbeat` / `release_ticket` | Keeps or gives up the lease. |
| `create_ticket` | Files a follow-up story or bug. |

## 3. Pick a loop

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

1. The human creates a ticket. It lands in **New**.
2. The agent claims it with action `refine` and either asks questions (the
   ticket moves to **Pending Answers** and the human's phone buzzes) or saves
   a spec and moves it to **Ready**.
3. Answering the last question sends the ticket back to **New** with the
   answers in the brief, and the agent refines again.
4. The agent claims it from Ready with action `implement`. The ticket moves to
   **In Progress** and is leased to that agent.
5. The agent opens a pull request, links it, comments, and moves the ticket to
   **PR / Review**. The human is notified with a button to open the PR.
6. The human merges and deploys, then drags the ticket to **Done**. To ask for
   changes, they comment and drag it back to In Progress; the agent claims it
   with action `resume` and reads the newest comments first.

A lease lasts 30 minutes unless renewed with a heartbeat. If an agent dies,
the lease expires and the ticket is claimable again.

## Plain HTTP

Everything the MCP tools do is also in the REST API ([api.md](api.md)) with
the same token as a bearer token, for agents that do not speak MCP.
