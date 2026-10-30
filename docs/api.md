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
| POST   | `/api/v1/tickets` | `CreateTicket` | `TicketDetail`. The ticket starts in `todo` / `refine`. |
| GET    | `/api/v1/tickets/{id}` | | `TicketDetail` |
| PATCH  | `/api/v1/tickets/{id}` | `EditTicket` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/move` | `{lane, after_id}` | `TicketDetail`. The server picks the stage. Agents may only move a finished build to `review`. |
| POST   | `/api/v1/tickets/{id}/comments` | `{body}` | `TicketDetail` |
| PUT    | `/api/v1/tickets/{id}/spec` | `{spec, submit?}` | `TicketDetail` |
| PUT    | `/api/v1/tickets/{id}/design` | `{design, submit?}` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/block` | `{reason}` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/questions` | `AskQuestions` | `TicketDetail` (the ticket keeps its lane and stage) |
| GET    | `/api/v1/events/stream` | | Server-sent events |

`after_id` places the card directly below that card in the target lane; `null`
places it at the top.

The spec and design endpoints behave differently by caller:

- **Agent.** Needs the lease. `PUT /spec` works in stage `refine` and
  `PUT /design` in stage `design`; otherwise `409`. Each save is a new
  revision when the text changed. With `submit: true` the ticket is handed on
  and the lease ends: `spec_review` (or `ready` when the repo has Approve spec
  off) for a spec, `design_review` (or `build` when Approve design is off) for
  a design. A submit needs non-empty text, no open questions, and no block. A
  spec submit also needs a repo.
- **Person.** May rewrite the text in any stage, as a new revision, so an
  earlier approval no longer covers it. `submit` is refused: people approve.

`POST /block` needs the lease and a ticket in `refine`, `design` or `build`.
It records the reason (up to 1,000 characters), ends the lease, and makes the
ticket unclaimable until a person unblocks it.

Agents move a ticket to `review` only from `build`, with `pr_url` set, no open
questions and no block. Every other `lane` from an agent answers `403`.

## Human only

| Method | Path | Body | Result |
|--------|------|------|--------|
| POST   | `/api/v1/tickets/{id}/answers` | `{answers: Answer[]}` | `TicketDetail` (once nothing is open the ticket is claimable again in the same stage) |
| POST   | `/api/v1/tickets/{id}/approve` | `{checkpoint, revision}` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/request-changes` | `{checkpoint, revision, comment}` | `TicketDetail` |
| POST   | `/api/v1/tickets/{id}/unblock` | `{comment?}` | `TicketDetail` |
| DELETE | `/api/v1/tickets/{id}` | | `204` |
| POST   | `/api/v1/repos` | `{full_name, url?, description?, color?}` | `Repo` (added by hand) |
| PATCH  | `/api/v1/repos/{id}` | `{color?, description?, treat_as_public?, require_spec_approval?, require_design_approval?}` | `Repo` |
| *      | `/api/v1/connections/...` | | Provider connections, admin only: see [connections.md](connections.md#api-admins-only-agents-get-403) |
| GET    | `/api/v1/tokens` | | `Token[]` |
| POST   | `/api/v1/tokens` | `{name, is_delegate?}` | `Token & {secret}` (the secret is shown once). `is_delegate` lets the token file autonomous tickets. |
| DELETE | `/api/v1/tokens/{id}` | | `204` |
| GET    | `/api/v1/settings` | | `BoardSettings` |
| PATCH  | `/api/v1/settings` | `Partial<BoardSettings>` | `BoardSettings`. People only; agents get `403`, a value out of range `400`. |

`checkpoint` is `"spec"` or `"design"`. `revision` is the `spec_revision` or
`design_revision` the person was shown. The endpoint answers `409` and changes
nothing when the ticket is not waiting at that checkpoint or the revision
changed. Approve moves `spec_review` to `ready` and `design_review` to
`build`, and records a binding "approved" decision. Request changes needs a
`comment` and moves the ticket back to `refine` or `design`. Both endpoints
answer `403` to agents. Unblock answers `409` when the ticket is not blocked.

`depends_on` on `POST /api/v1/tickets` needs `repo_id`, and every id must be a
ticket in the same repo; otherwise `400` naming the id, and no ticket is
created. A link that would close a cycle answers `409` with the path, such as
`#3 -> #5 -> #3`. Changing or clearing `repo_id` with `PATCH` answers `409`
while it would leave a dependency across repos. Relations
(`POST` and `DELETE /api/v1/tickets/{id}/relations...`) are described in
[traceability.md](traceability.md#api), dependencies in
[agents.md](agents.md#dependencies).

## Agent only

| Method | Path | Body | Result |
|--------|------|------|--------|
| POST | `/api/v1/agent/claim` | `{repo?, wait_seconds?, lease_minutes?}` | `Work`, or `204` when nothing is claimable |
| POST | `/api/v1/agent/tickets/{id}/heartbeat` | `{lease_minutes?}` | `TicketDetail` |
| POST | `/api/v1/agent/tickets/{id}/release` | | `TicketDetail` |
| GET  | `/api/v1/agent/tickets/{id}/brief` | | the brief, as `text/markdown` |
| POST | `/api/v1/agent/tickets/{id}/pr-state` | `{state: "merged"\|"closed"\|"open", note?}` | `TicketDetail`. Needs no lease. |
| POST | `/mcp` | JSON-RPC | MCP streamable HTTP (tools listed in [agents.md](agents.md)) |

`wait_seconds` (0 to 600) long-polls: the request stays open until a card
becomes claimable or the wait runs out. `repo` is a full name such as
`acme/api`; the agent then claims that repo's tickets plus tickets with no
repo that still need refining. An agent cannot submit a spec for a ticket
without a repo.

A claim takes tickets in this order: stage `build`, `design`, `refine`, then
`ready`. It never returns a ticket with open questions, a blocked ticket, or
a ticket in `spec_review`, `design_review`, `pr` or `done`, and never a
ticket whose `waiting_on` is not empty. While as many autonomous tickets as
`autonomous_in_flight_cap` allows are in `design` or `build`, it takes no
autonomous ticket from `ready` (other tickets still come through). Claiming
from `ready` moves the ticket to `design`.

Filing an autonomous ticket answers `409` once the same token has filed 50 in
the last hour.

When `pr-state` `merged` (or a person's move) brings a ticket to `done`, every
ticket that waited only on it gets a `dependencies_met` timeline entry by the
same actor and is claimable from the next claim on.

`pr-state` acts only on a ticket in stage `pr` that has a `pr_url`, otherwise
`409`. `merged` moves it to `merged` / `done`. `closed` moves it to
`in_progress` / `build` and blocks it with the reason "The pull request was
closed without merging", so a person decides what happens next. `open`
changes nothing. People get `403`: they move review tickets by dragging.

## Types

```ts
type LaneId = "todo" | "in_progress" | "review" | "merged";
type StageId =
  | "refine" | "spec_review" | "ready"      // todo
  | "design" | "design_review" | "build"    // in_progress
  | "pr"                                     // review
  | "done";                                  // merged
type Owner = "agent" | "human" | "nobody";
type Kind = "story" | "bug";

type Actor = { kind: "human" | "agent"; id: number; name: string };
type Lane = { id: LaneId; label: string; description: string };
// is_owner_review: the board waits for a person in this stage.
type Stage = { id: StageId; lane: LaneId; label: string; is_owner_review: boolean };

// repos is always the full list, even when the tickets are filtered.
// Tickets come sorted by rank within each lane.
type Board = { lanes: Lane[]; stages: Stage[]; repos: Repo[]; tickets: TicketSummary[] };

type Provider = "github" | "bitbucket" | "forgejo" | "manual";
type Repo = {
  id: number; provider: Provider;
  full_name: string;       // "acme/api"
  name: string;            // "api"
  url: string; description: string; color: string; default_branch: string;
  is_private: boolean; is_archived: boolean; synced_at: string | null;
  missing_since: string | null;  // set while the provider no longer lists it
  treat_as_public: boolean;
  visibility: "public" | "private";
  // Checkpoints. Only people change them, with PATCH /repos/{id}; repo sync never does.
  require_spec_approval: boolean;    // default true
  require_design_approval: boolean;  // default false
};

type Lease = { holder: string; expires_at: string };

type TicketSummary = {
  id: number; repo_id: number | null; kind: Kind; title: string;
  lane: LaneId;
  stage: StageId;
  owner: Owner;              // "human" whenever needs_owner
  needs_owner: boolean;      // open questions, blocked, or an owner-review stage
  blocked_reason: string;    // "" when not blocked
  is_autonomous: boolean;    // no checkpoints or questions; runs to a pull request
  spec_revision: number; design_revision: number;
  pr_url: string; branch: string;
  open_questions: number;
  waiting_on: number[];      // depends_on targets not merged yet; [] when none
  lease: Lease | null;
  created_at: string; updated_at: string; lane_changed_at: string;
};

type TicketDetail = TicketSummary & {
  body: string; spec: string; design: string; created_by: string;
  approved_spec_revision: number; approved_design_revision: number;
  // Effective checkpoints for this ticket. A ticket with no repo has both on;
  // an autonomous ticket has both off.
  gates: { spec: boolean; design: boolean };
  questions: Question[];
  timeline: TimelineEntry[];
};

// A lane is no longer accepted: the request is refused as an unknown field.
// New tickets start in todo / refine.
type CreateTicket = {
  repo_id?: number | null; kind: Kind; title: string; body?: string;
  depends_on?: number[];    // same-repo ticket ids, at most 20
};

type BoardSettings = {
  autonomous_in_flight_cap: number;  // 1 to 10, default 1
};
type EditTicket = Partial<{
  repo_id: number | null; kind: Kind; title: string; body: string;
  pr_url: string; branch: string;
  is_autonomous: boolean;   // people only; agents are refused
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
  | "created" | "moved" | "comment" | "spec" | "design" | "questions"
  | "answers" | "edited" | "claimed" | "released"
  | "stage" | "approved" | "changes_requested" | "blocked" | "unblocked"
  | "dependencies_met";
// moved (lane change): data {from, to}; stage: data {from, to} as stage ids;
// spec/design: data {revision, submitted};
// approved/changes_requested: data {checkpoint, revision}, body = the comment;
// blocked: body = the reason; unblocked: body = the optional comment;
// questions/answers: data {round, count};
// claimed: data {action}; edited: data {fields: string[]};
// dependencies_met: data {ticket_id, merged_by_kind}, by the actor that
// merged ticket_id, the last ticket this one depended on

type Token = {
  id: number; name: string; prefix: string;
  created_at: string; last_used_at: string | null;
};

type Work = {
  action: "refine" | "design" | "implement";
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
