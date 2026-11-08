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
| POST   | `/api/v1/tickets/{id}/move` | `{lane, after_id}` | `TicketDetail`. The server picks the stage. Agents may only submit a finished build to stage `pr` with `lane: "in_progress"`. |
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

Agents submit a ticket from `build` to `pr` with `lane: "in_progress"`, with
`pr_url` set, no open questions and no block. The ticket stays in In Progress
and the lease ends. Deprecated agent input `lane: "review"` is an alias for
this handoff; it is not a valid lane for lists or human moves. Other valid
lane targets from an agent answer `403`. People dropping within In Progress
only reorder; a cross-lane drop selects build if the design is cleared under
the repo approval rules, otherwise design. No human drop target selects PR.

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
| POST | `/api/v1/agent/tickets/{id}/pr-state` | `{state: "merged"\|"closed"\|"open"\|"repair", note?}` | `TicketDetail`. Needs no lease. |
| POST | `/mcp` | JSON-RPC | MCP streamable HTTP (tools listed in [agents.md](agents.md)) |

`wait_seconds` (0 to 600) long-polls: the request stays open until a card
becomes claimable or the wait runs out. `repo` is a full name such as
`acme/api`; the agent then claims that repo's tickets plus tickets with no
repo that still need refining. An agent cannot submit a spec for a ticket
without a repo.

A claim takes tickets in this order: stage `build`, `design`, `refine`, then
`ready`. It never returns a ticket with open questions, a blocked ticket, or
a ticket in `spec_review`, `design_review`, `pr`, `done` or `deployed`, and never a
ticket whose `waiting_on` is not empty. While as many autonomous tickets as
`autonomous_in_flight_cap` allows are in `design` or `build`, it takes no
autonomous ticket from `ready` (other tickets still come through). Claiming
from `ready` moves the ticket to `design`.

Filing an autonomous ticket answers `409` once the same token has filed 50 in
the last hour.

When a PR report or a person's move takes active work into `done` or
`deployed`, every ticket that waited only on it gets a `dependencies_met`
timeline entry by the same actor and is claimable from the next claim on.

`pr-state` acts only on a ticket in stage `pr` that has a `pr_url`, otherwise
`409`. `merged` moves it to `deployed` / `deployed` when its repository
uses `deploy_mode: "off"` (the default) or it has no repository. With
`deploy_mode: "agent"`, it moves to `merged` / `done` awaiting deployment.
`closed` moves it to `in_progress` / `build` and blocks it with the reason "The pull request was
closed without merging", so a person decides what happens next. `open`
changes nothing. People get `403`: they move tickets by dragging. `repair` returns a PR-stage
ticket to `in_progress` / `build` without a block and requires evidence.

## Types

```ts
type LaneId = "todo" | "in_progress" | "merged" | "deployed";
type StageId =
  | "refine" | "spec_review" | "ready"      // todo
  | "design" | "design_review" | "build"    // in_progress
  | "pr"                                     // in_progress
  | "done"                                   // merged
  | "deployed";                              // deployed
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
  deploy_mode: "off" | "agent"; // human-controlled, default off
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
  is_health_check: boolean;  // read-only; identifies a scheduled architecture review
  spec_revision: number; design_revision: number;
  pr_url: string; branch: string;
  open_questions: number;
  waiting_on: number[];      // depends_on targets outside done and deployed; [] when none
  lease: Lease | null;
  created_at: string; updated_at: string; lane_changed_at: string;
  attempts: StageAttempts;
};

// Read-only counts derived from stage moves and work-model reports
// ("· build-server:…", "· awaiting repair", "· awaiting review",
// "· adversarial review"). 0 means the step never started.
type StageAttempts = {
  build: number;          // entries into build, plus returns after a failed Windows run or review
  test: number;           // finished Windows verification runs, plus one in flight
  test_failures: number;  // Windows runs that failed ("· awaiting repair")
  review: number;         // fresh-context adversarial review rounds
  review_fixes: number;   // review rounds after which the author resumed work
  pr: number;             // entries into the PR stage
  pr_repairs: number;     // returns from an open PR to build
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
// completed ticket_id, the last ticket this one depended on

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


### GET /api/v1/account-usage

Human authentication required. Returns `{ "accounts": [...] }` from the
host's optional `DATA_DIR/account_usage.json` cache. No provider request or
model turn occurs. Each account has `provider` (`codex` or `claude`), nullable
`observed_at`, and `windows` containing `label`, `used_percent` (0–100 used,
not remaining), and nullable `resets_at`. Missing/invalid caches return an
empty account list, never an invented zero. Responses exclude credentials
and identity details. UI marks old, unknown-age and reset-expired readings
stale; the sampler refreshes independently every five minutes.

## Build hosts

Registry management requires a signed-in person. Agent credentials receive
`403`; missing or invalid credentials receive `401`. Browser mutations retain
the Origin requirement. Responses use `Cache-Control: no-store`.

| Method | Path | Body | Result |
|--------|------|------|--------|
| GET | `/api/v1/build-hosts` | | Host array, ordered by ID (possibly empty) |
| GET | `/api/v1/build-hosts/{id}` | | Host |
| POST | `/api/v1/build-hosts/pairing-codes` | Empty or `{}` | `201 {code, expires_at}` |
| POST | `/api/v1/build-hosts/{id}/approve` | Empty or `{name?, capabilities?, max_parallel?}` | Approved host |
| PATCH | `/api/v1/build-hosts/{id}` | `{name?, capabilities?, max_parallel?, enabled?}` | Updated host |
| POST | `/api/v1/build-hosts/{id}/revoke` | Empty or `{}` | Revoked host |

A host response contains `id`, `name`, `os`, `arch`, `status`, `capabilities`,
`max_parallel`, `enabled`, `public_key`, `facts`, `agent_version`,
`last_seen_at`, `created_at`, `approved_at`, `approved_by`, and `revoked_at`.
Capabilities are an array; facts are an object; unavailable timestamps and
approving user IDs are `null`. Timestamps use UTC with millisecond precision.

Approval applies only to pending hosts, records the approving user and time,
and preserves the independent enabled setting. Repeated approval returns
`409`. Revocation disables a host permanently and preserves earlier approval
information. Repeated revocation returns the original revocation time.
Revoked hosts can have metadata edited but cannot be enabled or approved.
An unchanged metadata update succeeds without recording another mutation.

Names are required and limited to 200 characters. `max_parallel` accepts
integers 1 through 8. Capabilities accept up to 32 strings matching
`^[a-z][a-z0-9_-]{0,31}$`; duplicates are removed and values sorted. An empty
array clears capabilities, and `enabled: false` disables a host. Omitted
fields retain their values. Nulls, unknown fields, duplicate keys, non-object
bodies, and additional JSON values are rejected with `400`. Missing hosts
return `404`.

Pairing codes contain 26 cryptographically random characters using
`23456789ABCDEFGHJKLMNPQRSTUVWXYZ`. The response is the only plaintext
readback; persistence retains a SHA-256 digest and a 15-minute expiry.
Issuance and actual host-management changes produce structured audit logs
without codes, digests, keys, or self-reported facts.

Host responses additionally include `running_job_ids` (an array),
`free_disk_bytes` (null until reported), and `online`. Connectivity is computed:
an approved identity with a heartbeat less than 180 seconds old is online,
including when disabled. Other identities and hosts never seen are offline.

### Machine enrollment and reports

Machine endpoints use a separate authentication path. Cookies and bearer tokens
have no authority here, and these endpoints do not require browser Origin.
Use a trusted transport: signatures do not encrypt enrollment codes or reports.

| Method | Path | Authentication | Result |
|--------|------|----------------|--------|
| POST | `/api/v1/build-host/pair` | Pairing code in body | `201 {id, status: "pending"}` |
| GET | `/api/v1/build-host/status` | Signature, pending or approved | `{id, status, enabled, capabilities, max_parallel, waiting_for_approval}` |
| POST | `/api/v1/build-host/heartbeat` | Signature, approved | `{id, last_seen_at}` |

Enrollment requires `{code, public_key, name, facts}`. The key is a canonical,
padded standard-base64 Ed25519 public key (32 decoded bytes). Names use the
registry's 200-character limit. A successful enrollment consumes the code and
creates its identity in one transaction. Unknown, expired, used, and incorrectly
formatted codes all return the same `401`. Duplicate public keys return `409`
and leave the code available. Validation errors return `400` without consuming
it. All lifecycle states reserve their public key permanently.

Each client IP may make five enrollment attempts per minute, including failed
attempts and successful enrollments. Excess attempts return `429` with
`Retry-After`. Forwarded addresses follow the server's trusted-hop configuration.
The limiter retains at most 4,096 active addresses and refuses new addresses
while full. This limit applies independently of sign-in and agent write limits.

A status request has no body. Pending identities receive
`waiting_for_approval: true`. Polling does not update connectivity. Approval is
read from storage on each request; revocation invalidates machine credentials.

A heartbeat requires `{facts, running_job_ids, free_disk_bytes}`. Job IDs must be
distinct positive int64 values, up to 128 entries; `[]` reports idle. Free disk
is a nonnegative int64, measured on the host's chosen build volume. It is stored
separately from the disk snapshot in facts. Reports replace telemetry and update
the OS, architecture, and agent-version projections, with a server UTC timestamp
at millisecond precision. They preserve owner settings, including capabilities.
Disabled approved hosts can still report health. Job IDs are observations and
do not authorize operations on those jobs.

### Request signatures

Send exactly one of each header:

- `X-Build-Host`: positive decimal host ID, without signs or leading zeroes.
- `X-Timestamp`: positive Unix seconds, in the same decimal encoding.
- `X-Signature`: canonical padded standard base64 of the 64-byte signature.

Sign these bytes with the enrolled private Ed25519 key, without a final newline:

```text
METHOD + "\n" + PATH + "\n" + TIMESTAMP + "\n" + lowercase_hex(SHA256(BODY))
```

Use the actual uppercase method and the literal endpoint path. Query strings,
escaped path variants, and HEAD status requests are refused. BODY is the exact
request body, including whitespace; an empty status body uses SHA-256 of zero
bytes. The timestamp may differ from server Unix seconds by at most 300 seconds,
in either direction. Missing or ambiguous headers, bad signatures, unknown hosts,
and disallowed lifecycle states return the same `401`. Storage failures return
`500`. Existing human and agent APIs do not accept machine signatures.

There is no nonce or replay cache. An identical signed request can be accepted
again while its timestamp remains fresh. Synchronize the host clock.

### Machine facts

New enrollments and heartbeats require every field below. Historical partial
facts remain readable. Facts must occupy at most 65,536 UTF-8 bytes before
compaction; the full machine body is limited to 96 KiB. JSON must contain one
object, with exact field names, no duplicate keys (at any depth), no nulls, no
unknown fixed-schema fields, and no trailing values. Integers must fit int64;
metadata strings cannot contain control characters. Violations return `400`.

| Fields | Type and limits |
|--------|-----------------|
| `os` | `windows`, `linux`, or `macos` |
| `arch` | Lowercase token matching `^[a-z0-9][a-z0-9_-]{0,63}$` |
| `os_version`, `cpu_model` | Strings, at most 256 and 512 characters respectively; may be empty |
| `hostname`, `agent_version` | Nonempty strings, at most 255 and 64 characters |
| `lan_ip` | IP literal without a zone, or empty when unavailable; never resolved or trusted |
| `cores`, `threads` | Positive integers; threads must be at least cores |
| `ram_bytes` | Nonnegative integer |
| `disks` | Up to 128 `{mount, total, free, is_build_volume}` objects; unique nonempty mount up to 1,024 characters; nonnegative byte counts with free <= total; boolean build-volume flag |
| `gpus` | Up to 32 `{name, driver, opengl_version}` objects; nonempty name; strings at most 256 characters; driver/version may be empty |
| `toolchains` | Up to 64 tool-name/version entries; names match `^[a-z][a-z0-9_-]{0,31}$`; versions are nonempty strings up to 256 characters |
| `interactive_session`, `build_user_is_admin` | Booleans |

Disk and GPU arrays and the toolchain object may be empty. At enrollment,
nonempty versions for `msvc`, `dotnet`, `cmake`, `ninja`, `gcc`, `clang`, `qt`,
`xcode`, `python`, and `docker` suggest the matching capabilities. `gl` is
suggested only with an interactive session and a GPU reporting desktop OpenGL
3.3 or newer. Versions start with numeric major.minor and may have whitespace
and vendor text afterward; OpenGL ES does not qualify. Suggestions are sorted
and unique. Other tools and administrator privileges suggest nothing. The owner
confirms or edits capabilities at approval; heartbeats never replace them.

### Report history

`GET /api/v1/build-hosts/{id}/events?after_id=0&limit=100` requires human
authentication. `after_id` must be nonnegative; `limit` is 1–200, default 100.
The response is an array ordered by ascending event ID, empty when no history
exists; a missing host returns `404`. Each entry contains `id`, `host_id`,
`event_type`, `data`, and `created_at`. A `toolchains_changed` event's data holds
`before` and `after` version maps. JSON key order does not count as a change;
missing toolchains in old facts count as an empty map. Reports and history
commit together. History retention is not automatic.

Each completed enrollment invokes the configured notifier once with fixed text
requesting approval. Failed or repeated enrollment requests do not notify.
Delivery failure is logged without machine metadata and does not undo enrollment.
This is best-effort delivery, with no durable retry or crash-proof exactly-once
guarantee.

## Delivery statistics (human only)

`GET /api/v1/stats?days=7&tz_offset=0` returns delivery metrics for the selected
period. `days` accepts 1–365 (default 30); `tz_offset` is the local UTC offset
in minutes (default 0). Agents receive `403`.

Each entry in `repos` includes `health`, describing the repository's current
architecture review status independently of the delivery period:

```json
{
  "every": 8,
  "merges_since_check": 3,
  "next_in": 5,
  "last_check_ticket": 42,
  "last_checked_at": "2026-03-10T15:00:00Z",
  "open_follow_ups": 2
}
```

`every` is the configured merge interval, with zero disabling scheduling.
`merges_since_check` counts currently completed ordinary tickets beyond the
persisted merge checkpoint; review tickets do not advance this count.
`next_in` is the remaining merge count, floored at zero, or null when disabled.
A zero may indicate a backlog waiting for an open review to finish.

`last_check_ticket` identifies the most recently filed review still associated
with the repository, or null. `last_checked_at` is the filing/checkpoint time,
not the completion time; it can survive deletion of the review ticket, and
can be null even when a ticket exists. `open_follow_ups` counts distinct existing
unfinished tickets with a direct outgoing `follow_up_of` relation to that
latest review, including blocked or waiting tickets.

Configured repositories appear even without delivery activity. Synthetic rows
for unassigned work and unresolved repository references have `health: null`.
Delivery counters keep their period-based semantics. Statistics are read-only
and may reflect concurrent updates across reads.

## Verification jobs

Agent tokens can upload a commit with `POST /api/v1/build-jobs` and inspect its
state with `GET /api/v1/build-jobs/{id}`. Uploads use multipart form data: a
`metadata` JSON part followed by one `bundle` part. The filename is ignored.
Metadata contains `ticket_id`, the ticket's assigned `repo` full name, `sha`
(a full 40- or 64-digit Git object ID), `preset` (`dotnet`, `cmake`, or `script`),
`target`, `bundle_bytes`, and `bundle_sha256` (lowercase SHA-256 of the upload).
For example, `target` can be
`{"os":"linux","required_capabilities":["cmake","ninja"]}`. Targets also accept
`windows` and `macos`; capability tokens follow the host registry syntax.

The response is a job object: `id`, `ticket_id`, `repo`, `sha`, `preset`, `target`,
`status`, `host_id`, `attempt`, `created_at`, `started_at`, `finished_at`, `phase`,
`detail`, `result`, `bundle_bytes`, and `bundle_sha256`. Unset assignments,
timestamps, and results are null. Creation returns `201`; an identity already
stored for that ticket returns `200` with its current state. Identity depends
on repository, commit, preset, and canonical target. Reordered or repeated
capabilities do not create a different job. A collision with another ticket,
an active upload, or an incompatible retained payload returns `409`.

First publication of a distinct commit supersedes queued and running work for
the same ticket and complete target. A retry cannot reset a job or replace its
bundle. Additional presets for the current commit can run independently;
additional checks for an older superseded commit stay superseded. Commit order
is publication order, without fetching Git history.

Approved hosts use the existing Ed25519 signature protocol on these routes.
Sign the concrete path containing the job hash. Query strings, path aliases,
and ambiguous headers are rejected.

| Request | Body | Response |
|---------|------|----------|
| `POST /api/v1/build-host/claim` | `{}` | `200 {"job": ...}` or `204` when no eligible slot/work exists |
| `GET /api/v1/build-host/jobs/{id}/bundle` | Empty | Opaque bytes, with Content-Length; requires a current running assignment |
| `POST /api/v1/build-host/jobs/{id}/progress` | `{"attempt":1,"phase":"build","detail":"compiling"}` | `{stop,status,attempt}` |
| `POST /api/v1/build-host/jobs/{id}/result` | `{attempt,status,result}` | Final job object |

Claims match host OS and owner-approved capabilities and respect enabled state
and `max_parallel`. Heartbeat telemetry does not grant capacity or capabilities.
Use the assignment's attempt on every report. Progress phase/detail allow at
most 64/4096 UTF-8 bytes. On `stop:true`, stop execution and acknowledge with
phase `stopped` (or send a terminal report). Stopping assignments continue to
occupy a slot until acknowledgment or connectivity timeout.

A result has `commands`, an array of `{label,exit_code,log}`, and optional
`failure_kind`. Each label is 1–200 bytes, each log is at most 16 KiB, and there
are at most 128 commands. A `passed` report requires at least one command and
all zero exit codes. A `failed` report requires a nonzero exit code or nonempty
failure kind (at most 64 bytes). Conflicting repeated terminal reports return
`409`; identical reports are idempotent. Superseded/cancelled jobs keep their
terminal status when acknowledging completion.

Only a signed-in administrator can call
`POST /api/v1/build-jobs/{id}/cancel`, with the normal same-origin requirement.
Repeated cancellation succeeds; other terminal states return `409`. Agent tokens,
browser cookies, and host signatures each authorize only their own route class.

Heartbeats accept legacy positive integer job IDs and lowercase 64-digit hash
strings together, for example `"running_job_ids":[7,"<64-digit hash>"]`.
Scalar types round-trip unchanged. Decimal strings, fractions, malformed hashes,
and duplicates are rejected. These values are observations, not assignments.

The heartbeat timeout is three minutes, inclusive. Recovery runs at startup,
every 30 seconds, and during worker operations. A disconnected or revoked host's
work is retried up to three execution attempts, then fails with
`failure_kind:"infrastructure"`. Disabling a host stops new claims while its
current work can finish. Progress does not replace heartbeats.

Payload storage is under the data directory's `build_jobs/<id>/commit.bundle`.
Bundles are immutable and limited to 100 MiB each; the aggregate allowance is
2 GiB, including retained partial and orphan payloads. Reservations prevent
concurrent uploads from overspending that allowance. Metadata is limited to
16 KiB and multipart overhead to 64 KiB. Result objects are limited to 256 KiB;
the signed result envelope permits an additional 4 KiB. Size and capacity
violations return `413`. Other existing machine routes keep their 96 KiB limit.

Interrupted transfers remain on disk and consume capacity. Uploaders hold a
process lock for each reservation; startup only abandons reservations whose lock
is free, preserving transfers through other server instances. Empty `.lock` files
are persistent coordination metadata and must not be removed while servers run.
Shared storage must support operating-system advisory locks. Startup validates
referenced payloads and refuses missing or corrupt files. Nothing prunes failed
uploads or published bundles; operators manage retention externally. SQLite
backups alone do not contain bundle payloads. Back up the data volume to preserve
both. Uploads retain the server's existing read timeout.

## Repository deployment policy

Repository objects expose `deploy_mode` through existing reads, including
board and work responses and MCP `list_repos`. To change it, a person sends
`PATCH /api/v1/repos/{id}` with `{"deploy_mode":"off"}` or
`{"deploy_mode":"agent"}` using the same authentication and Origin checks as
other repository settings. Agent tokens receive `403` before any fields are
changed, including mixed-field requests. Unknown values, an empty string, or
non-string values receive `400` and leave the repository unchanged. Omitting
the field or supplying `null` leaves its value alone. Repository creation
uses `off`; provider synchronization preserves the owner's policy.

The policy affects future merged-PR reports only. A human cross-lane move to
Deployed sets stage `deployed` and records `Marked deployed by <name>` in the
stage entry, alongside its actor fields. Moving to Merged selects `done`,
even on an `off` repository. Reordering within a lane keeps the stage and
adds no deployment entry. Deployed is not claimable or an agent work stage.
The stage catalog's `done` label remains Merged; clients derive Awaiting deploy
when the associated repository's mode is `agent`.

Dependencies are satisfied by either `done` or `deployed`. Completion metrics
retain their existing `merged` field names and count either terminal state.
A move between those states does not reset lead/cycle time or throughput
windows and does not repeat dependency release or health-check scheduling.
The schema upgrade converts existing Merged/Done tickets to Deployed without
rewriting history; downgrade collapses deployment state back to Merged/Done.

### Build-host presentation

Human sessions can manage registered hosts in Settings. The legacy
`GET /api/v1/runners` feed remains independent of registered hosts.

`POST /api/v1/build-hosts/pairing-codes` returns `code`, `expires_at`, and
`public_base_url` from server configuration. The response is marked
`Cache-Control: no-store`. Pairing plaintext is available only at issuance.
An empty public URL requires configuration before installation commands can
be copied; the browser does not substitute its own origin.

Installation commands download `/install/build-host.ps1` or
`/install/build-host.sh` from the configured public base URL. The installer
integration contract is PowerShell `-BoardUrl <url> -PairingCode <code>`, or
POSIX shell `--board-url <url> --pairing-code <code>`. Pairing codes are
arguments, never download URL parameters. The UI can display these commands
before installer routes are deployed; this does not implement installers.

`GET /api/v1/build-hosts/runners` requires a human session and returns:

- `observed_at`: the server observation timestamp.
- `hosts`: approved hosts in ID order, each with `id`, `name`, `os`,
  `enabled`, `max_parallel`, `online`, `last_seen_at`, and `jobs`.
- Each job has `id`, `ticket_id`, `repo`, `sha`, `preset`, `status`, `phase`,
  `detail`, `started_at`, and `attempt`.
- `queued_by_os`: integer counts for `windows`, `linux`, and `macos`,
  including zero counts.

Jobs represent authoritative, unreleased assignments. Cancelled and
superseded assignments remain visible as stopping until released. Chips
identify assignments by job ID rather than persistent physical slot numbers.
Queue depth includes all queued jobs for that OS, regardless of individual
host capabilities or enabled state. Connectivity is independent of enabled
state and uses the existing three-minute heartbeat threshold. This read-only
projection is eventually consistent across concurrent assignment changes.

`GET /api/v1/build-hosts/{id}/events?latest=true&limit=20` returns recent
events in descending ID order. The optional limit is 1–200 (default 100).
`latest` accepts `true` or `false`; it cannot accompany `after_id`. Duplicate
or invalid pagination arguments are rejected. Without `latest=true`, the
existing ascending `after_id` cursor behavior remains available.
