# Shared worker access and scaling

Workers can share one board bearer token. Send a stable, unique `X-Worker-ID`
on **every** request, including claims, heartbeats, updates, releases and MCP.
Values are 1–48 ASCII letters, digits, underscores or hyphens. The board scopes
the lease identity to the credential ID and worker ID. Reusing a worker ID
means the same worker, not a second independent session. The token's write
rate limit remains shared. Revoking it stops every worker using it.

Worker sessions have execution-only authority, even if the underlying token
can file autonomous tickets. Spec-authoring chats omit the worker header and
retain the token's existing filing grant and per-token filing budget. Requests
without the header retain legacy behavior. `@worker/` names are reserved.

## Runner names

Each worker gets a friendly display name the first time it claims work. The
names come from a per-family pool in the `runner_names` table, seeded by
migration 0026: the family is the worker ID without its numeric suffix
(`claude_1` is family `claude`, slot 1). Worker `<family>_<n>` takes the
name at position `n`, so `claude_1` is always the family's first name across
restarts and new tokens. A worker without a free slot of its own takes the
last free name. Families without a pool keep their raw identity.

Assignment is idempotent and runs in one write transaction; a unique index
on the worker stops a second assignment. People rename a runner in
Settings → Runner names (`PATCH /api/v1/runner-names/{worker}`), and the
rename sticks. Leases, timelines and the API keep the raw identity
(`@worker/<token>/<worker>`); the UI shows "Name (worker)", such as
"Michael (claude_1)", with the full identity in its tooltip, and briefs and
`list_tickets` say the same. When two
families have a runner of the same name, the UI badges each with its
family.

`GET /api/v1/settings` now includes `codex_workers` (0–6, default 3) and the
read-only `codex_worker_limit` (6). Only a signed-in person may PATCH the count.
Zero pauses new assignments. The settings control also sets the autonomous
in-flight cap to the requested count (minimum 1 while paused).

Hawaii provisions six resource-limited containers, sharing `codex_shared.env`
but using different `RUNNER_NAME` and `CODEX_WORKER_SLOT` values. Before every
claim, each supervisor reads the configured count. Slots above that count
wait 15 seconds without calling a model or claiming work. Reductions drain
the current assignment before waiting. Containers stay ready, so changing
the count needs no Docker access from the board or from model processes.

Each container is limited to 2 CPUs, 8 GiB memory and two build/test jobs.
The maximum six active slots can use 12 CPU equivalents and 48 GiB memory.
No model inference is used for settings polling or account usage monitoring.
