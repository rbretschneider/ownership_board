# Design

ownership_board is a private task board where an AI agent is the primary worker
and a human is the product owner. The board answers one question at a glance:
**whose move is it?**

## Lanes

The lanes are fixed. They are a domain enum, not configuration, so every agent
prompt, notification and API client can rely on them never changing.

| Id            | Label           | Ball is with | Meaning |
|---------------|-----------------|--------------|---------|
| `new`         | New             | agent        | Created, waiting for refinement. The agent acts as a business analyst. |
| `answers`     | Pending Answers | human        | The agent asked questions. Nothing moves until they are answered. |
| `ready`       | Ready           | agent        | Refined and unambiguous. The agent's work queue. |
| `in_progress` | In Progress     | agent        | Claimed by an agent and being built. |
| `review`      | PR / Review     | human        | Code complete, a PR is open. The human reviews and merges. |
| `done`        | Done            | nobody       | Merged and deployed. |

The human can drag any card anywhere. The agent moves cards through the API.
Two moves are automatic:

- Posting questions moves a card to `answers` and notifies the human.
- Answering the last open question moves the card back to `new`, so the agent
  refines again with the answers in hand.

Claiming a `ready` card moves it to `in_progress`.

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
likely repos as options. An agent cannot queue a ticket for building until it
has a repo.

## Agent leases

When an agent claims a card it holds a lease (30 minutes by default, renewable
with a heartbeat). Other agents skip leased cards. A lease that expires makes the
card claimable again, so a crashed session never strands work. A human moving a
card clears its lease.

Claim order is: unleased `in_progress` cards first (finish what was started),
then `new` (refining is cheap and unblocks the human sooner), then `ready`.
Within a lane, cards are taken top to bottom, so the human prioritises by
dragging.

## The brief

Every claim returns a `brief`: one markdown document written for an LLM. It holds
the request, the refined spec, every question round with its answers, recent
comments, and explicit instructions for the action the agent should take. The
agent never has to stitch context together from several calls.

## Waking up

An agent loop does not need to poll on a timer. `POST /api/v1/agent/claim` with
`wait_seconds` blocks until work exists (or the wait expires), so an idle loop
costs one open HTTP request rather than repeated model turns. The same
operations are exposed as MCP tools at `/mcp`.

## Notifications

The board pushes to ntfy when the ball lands with the human: questions asked and
a PR ready for review, plus a quieter note when a card reaches Done. Each push
deep-links to the card.
