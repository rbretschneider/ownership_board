# ownership_board

A private, self-hosted task board where an AI agent does the work and you
own the decisions. It answers one question at a glance: **whose move is it?**

You drop an idea or a bug in from your phone. An agent picks it up, acts as a
business analyst, and either writes a spec or asks you a few multiple-choice
questions. Your phone buzzes; you tap the answers; the agent carries on,
builds it, opens a pull request, and the card lands in review.

It exists because public issue trackers are the wrong place for half-formed
ideas about open-source projects.

## Lanes

Fixed on purpose, so agents and integrations can rely on them.

| Lane | Whose move |
|------|------------|
| New | agent: refine it |
| Pending Answers | you: answer the questions |
| Ready | agent: build it |
| In Progress | agent, holding a lease |
| PR / Review | you: review and merge |
| Done | nobody |

## What makes it agent-friendly

- **Structured question rounds.** Agents ask multiple-choice questions with a
  recommended option. You answer with a tap, add a note, or say "your call".
  Answering the last one sends the ticket straight back to the agent.
- **One brief per ticket.** A claim returns a single markdown document with
  the request, the spec, every answer, recent comments and explicit
  instructions for the next step.
- **Long-poll claims.** `claim` blocks until there is work, so an idle agent
  costs one open HTTP request, not a model turn per poll.
- **Leases.** A claimed ticket is held for 30 minutes and renewable; if an
  agent dies, the ticket frees itself.
- **MCP built in.** `/mcp` exposes the board as tools for Claude Code and any
  other MCP client.
- **Repo agnostic.** Repos sync from GitHub, Forgejo/Gitea and Bitbucket. A
  ticket can start without one and the agent works out which repo it means,
  or asks.

## Run it

```bash
cp .env.example .env   # set AUTH_TOKEN_SECRET and PUBLIC_BASE_URL at least
docker compose up -d
```

Open the board, create the first account in the setup screen, add a repo
(or set a provider token and press Sync now), and create an agent token in
Settings. Then follow [docs/agents.md](docs/agents.md) to point Claude Code at
it.

The container listens on plain HTTP on port 8080. Put a TLS-terminating
reverse proxy in front and set `TRUST_PROXY_HOPS=1`.

Data lives in one SQLite file on the `/data` volume, with daily snapshots in
`/data/backups`. To restore, stop the container and copy a snapshot over
`/data/ownership.db`.

## Features

- Mobile-first, installable web app; dark by default.
- Drag and drop on desktop and touch (long-press, or drop onto a lane tab).
- Live updates over server-sent events.
- ntfy push when the ball is in your court, with a deep link to the ticket.
- Password login plus OIDC SSO (link-first: SSO identities are linked to an
  account deliberately, never matched by email).
- One Go binary with the web app embedded; distroless image for amd64 and
  arm64.

## Develop

```bash
# API on :8080 (no web app embedded)
AUTH_TOKEN_SECRET=$(openssl rand -hex 32) go run ./cmd/ownership

# web app on :5173, proxying /api to :8080
cd web && npm ci && npm run dev
```

Tests: `go test ./...` and `cd web && npm test`.

Docs: [design](docs/design.md), [HTTP API](docs/api.md), [agents](docs/agents.md).

## License

MIT
