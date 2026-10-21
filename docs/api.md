# HTTP API

All endpoints live under `/api/v1`, take and return JSON, and are denied by
default. Errors have one shape:

```json
{ "error": { "code": "not_found", "message": "ticket 42 does not exist" } }
```

Codes: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`,
`rate_limited`, `internal`.

## Authentication

| Caller | How |
|--------|-----|
| Browser | `ob_access` cookie (15 min JWT) plus `ob_refresh` cookie (opaque, rotated on use, path `/api/v1/auth`). On a `401`, call `POST /auth/refresh` once and retry. |
| Agent   | `Authorization: Bearer ob_pat_...` personal access token, created in Settings. |

Unsafe browser requests (`POST`, `PUT`, `PATCH`, `DELETE`) must carry an `Origin`
that matches `PUBLIC_BASE_URL`.

### Public endpoints

| Method | Path | Body | Result |
|--------|------|------|--------|
| GET  | `/health` | | `{"status":"ok"}` (checks the database) |
| GET  | `/api/v1/auth/status` | | `AuthStatus` |
| POST | `/api/v1/auth/setup` | `{name, email, password}` | `User`, sets cookies. Only while no user exists. |
| POST | `/api/v1/auth/login` | `{email, password}` | `User`, sets cookies. Throttled per IP. |
| POST | `/api/v1/auth/refresh` | | `User`, rotates cookies |
| POST | `/api/v1/auth/logout` | | `204`, clears cookies |
| GET  | `/api/v1/auth/oidc/start` | | `302` to the IdP. `?link=1` (signed in) links SSO to the current account. |
| GET  | `/api/v1/auth/oidc/callback` | | `302` to `/#/`, `/#/settings?linked=1`, or `/#/login?error=sso_not_linked\|sso_denied\|sso_failed` |

SSO is link-first: an identity signs in only once an account has linked it.
Accounts are never matched by email.

```ts
type AuthStatus = {
  setup_required: boolean;
  password_login: boolean;
  oidc: { enabled: boolean; label: string };
  user: User | null;
};
type User = { id: number; name: string; email: string; is_admin: boolean; sso_linked: boolean };
```

## Board (human or agent)

| Method | Path | Body | Result |
|--------|------|------|--------|
| GET    | `/api/v1/me` | | `Actor` |
| GET    | `/api/v1/lanes` | | `Lane[]` |
| GET    | `/api/v1/board?repo=<id\|none>` | | `Board`; `none` = tickets without a repo |
| GET    | `/api/v1/repos` | | `Repo[]` |
| POST   | `/api/v1/tickets` | `CreateTicket` | `TicketDetail` |
| GET    | `/api/v1/tickets/{id}` | | `TicketDetail` |
| PATCH  | `/api/v1/tickets/{id}` | `EditTicket` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/move` | `{lane, after_id}` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/comments` | `{body}` | `TicketDetail` |
| PUT    | `/api/v1/tickets/{id}/spec` | `{spec}` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/questions` | `AskQuestions` | `TicketDetail` (card moves to `answers`) |
| GET    | `/api/v1/events/stream` | | Server-sent events |

`after_id` places the card directly below that card in the target lane; `null`
places it at the top.

## Human only

| Method | Path | Body | Result |
|--------|------|------|--------|
| POST   | `/api/v1/tickets/{id}/answers` | `{answers: Answer[]}` | `TicketDetail` (card returns to `new` once nothing is open) |
| DELETE | `/api/v1/tickets/{id}` | | `204` |
| POST   | `/api/v1/repos` | `{full_name, url?, description?, color?}` | `Repo` (added by hand) |
| PATCH  | `/api/v1/repos/{id}` | `{color?, description?}` | `Repo` |
| *      | `/api/v1/connections/...` | | Provider connections, admin only: see [connections.md](connections.md#api-admins-only-agents-get-403) |
| GET    | `/api/v1/tokens` | | `Token[]` |
| POST   | `/api/v1/tokens` | `{name}` | `Token & {secret}` (the secret is shown once) |
| DELETE | `/api/v1/tokens/{id}` | | `204` |

## Agent only

| Method | Path | Body | Result |
|--------|------|------|--------|
| POST | `/api/v1/agent/claim` | `{repo?, wait_seconds?, lease_minutes?}` | `Work`, or `204` when nothing is claimable |
| POST | `/api/v1/agent/tickets/{id}/heartbeat` | `{lease_minutes?}` | `TicketDetail` |
| POST | `/api/v1/agent/tickets/{id}/release` | | `TicketDetail` |
| GET  | `/api/v1/agent/tickets/{id}/brief` | | the brief, as `text/markdown` |
| POST | `/mcp` | JSON-RPC | MCP streamable HTTP (tools listed in [agents.md](agents.md)) |

`wait_seconds` (0 to 600) long-polls: the request stays open until a card
becomes claimable or the wait runs out. `repo` is a full name such as
`acme/api`; the agent then claims that repo's tickets plus tickets with no
repo that still need refining. An agent cannot move a ticket without a repo
to `ready` or `in_progress`.

## Types

```ts
type LaneId = "new" | "answers" | "ready" | "in_progress" | "review" | "done";
type Owner = "agent" | "human" | "nobody";
type Kind = "story" | "bug";

type Actor = { kind: "human" | "agent"; id: number; name: string };
type Lane = { id: LaneId; label: string; owner: Owner; description: string };

// repos is always the full list, even when the tickets are filtered.
// Tickets come sorted by rank within each lane.
type Board = { lanes: Lane[]; repos: Repo[]; tickets: TicketSummary[] };

type Provider = "github" | "bitbucket" | "forgejo" | "manual";
type Repo = {
  id: number; provider: Provider;
  full_name: string;       // "acme/api"
  name: string;            // "api"
  url: string; description: string; color: string; default_branch: string;
  is_private: boolean; is_archived: boolean; synced_at: string | null;
  missing_since: string | null;  // set while the provider no longer lists it
};

type Lease = { holder: string; expires_at: string };

type TicketSummary = {
  id: number; repo_id: number | null; kind: Kind; title: string;
  lane: LaneId; owner: Owner;
  pr_url: string; branch: string;
  open_questions: number;
  lease: Lease | null;
  created_at: string; updated_at: string; lane_changed_at: string;
};

type TicketDetail = TicketSummary & {
  body: string; spec: string; created_by: string;
  questions: Question[];
  timeline: TimelineEntry[];
};

type CreateTicket = {
  repo_id?: number | null; kind: Kind; title: string; body?: string; lane?: LaneId;
};
type EditTicket = Partial<{
  repo_id: number | null; kind: Kind; title: string; body: string;
  pr_url: string; branch: string;
}>;

type AskQuestions = {
  summary: string;
  questions: { prompt: string; options?: string[]; recommended?: number | null }[];
};
type Question = {
  id: number; round: number; prompt: string;
  options: string[]; recommended: number | null;
  asked_by: string; asked_at: string;
  answer: {
    choice: number | null; text: string; deferred: boolean;
    by: string; at: string;
  } | null;
};
type Answer = {
  question_id: number;
  choice?: number | null;  // index into options
  text?: string;           // free text, alone or alongside a choice
  deferred?: boolean;      // "your call": use the recommendation
};

type TimelineEntry = {
  id: number; type: TimelineType; at: string;
  actor: { kind: "human" | "agent" | "system"; name: string };
  body: string;            // comment text or round summary; "" otherwise
  data: Record<string, unknown>;
};
type TimelineType =
  | "created" | "moved" | "comment" | "spec" | "questions"
  | "answers" | "edited" | "claimed" | "released";
// moved: data {from, to}; questions/answers: data {round, count};
// claimed: data {action}; edited: data {fields: string[]}

type Token = {
  id: number; name: string; prefix: string;
  created_at: string; last_used_at: string | null;
};

type Work = {
  action: "refine" | "implement" | "resume";
  ticket: TicketDetail;
  repo: Repo | null;
  brief: string;           // markdown written for an LLM
  lease: Lease;
};
```

## Server-sent events

`GET /api/v1/events/stream` sends one event per change:

```
event: ticket
data: {"cursor": 118, "type": "moved", "ticket_id": 42}
```

`type` is a `TimelineType` or `deleted`. Clients refetch what they display; the
stream is a doorbell, not a replication log. A comment line is sent every 25
seconds to keep proxies from closing the connection.
