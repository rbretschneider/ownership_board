# Security Review: Provider connections, implementation

**Story:** Provider connections (`docs/connections.md`)
**Reviewer:** Security Researcher Agent (Claude Opus 5.5, `claude-opus-5-5`)
**Date:** 2026-10-21
**Scope:** The uncommitted working tree (modified and untracked files) in `D:\Repos\ownership_board`, checked against
`docs/security/connections-plan-review.md` and the "Security decisions" section of `docs/connections.md`.
**Verdict:** PASS WITH FIXES

## What was run

- `go vet ./...` was clean.
- `go test ./...` passed in every package.
- `go test -race` could not run, because there is no gcc on this Windows host. The concurrency conclusions below come from reading the code. QA should run `-race` in CI or in the Linux container.
- Throwaway checks in the scratchpad, outside the repo:
  - A copy of `Secret` was run through the `fmt` verbs.
  - The dial guard's IP predicate was checked against IPv4-mapped, IPv6, NAT64 and `0/8` inputs, and real dials were made through a `Dialer.Control` guard to `localhost`, `[::]`, `0.0.0.0`, `[::ffff:127.0.0.1]` and `169.254.169.254`.
  - `NormalizeForgejoURL` was fed dot segments, `%2e`/`%2f`, out-of-range ports, `|` and case variants.

## Verification of plan findings

| ID | Status | Evidence |
|----|--------|----------|
| SEC-01 SSRF | **Mostly fixed, with residual risk** | Dial-time guard: `reposource/client.go:47-76` (`Dialer.Control` sees the resolved IP, so DNS rebinding and `localhost` are refused; IPv4-mapped loopback and metadata are refused). `Proxy: nil`. No reflected bodies or dial detail: `statusError` at `client.go:115-126`, `unreachable` at `:129-134`. Forgejo-shaped check: `forgejo.go:84-96`. 10 s timeout: `service.go:19,252`. Missing: the 64 KiB validation cap (16 MiB is used), dot-segment rejection, and a port-aware allowlist. The HTTP status code is still reflected (see M-1, L-3, L-4). |
| SEC-02 refresh race | **Fixed, with one ABA gap** | `catalog.go:159-201`: `refreshMu` single-flight, no I/O inside a transaction, CAS through `store.Put` (`sqlite/connections.go:41-80`, `UPDATE ... WHERE provider=? AND generation=?`, `INSERT ... ON CONFLICT DO NOTHING`). A lost CAS re-reads the row. The generation goes back to 1 after Disconnect (see L-1). |
| SEC-03 redirects | **Fixed** | `noRedirects` on the shared client and again inside every source constructor (`forgejo.go:37`, `bitbucket.go:158`, `github.go:288`, `github_app.go:105`). A 3xx becomes a fixed error. Bitbucket `next` must be `https`, have the exact host and no userinfo (`bitbucket.go:216-219`). `TestTokenRequestsDoNotFollowRedirects` covers it. |
| SEC-04 leakage | **Fixed in code; the QA canary test is missing** | Secrets go only in POST bodies and headers (`github_app.go:147,159,257-276`). Token-endpoint errors carry only a sanitised OAuth code (`:304,325-334`). The `Secret` type exists (`secretbox/secret.go`). `View` and the DTO have no token fields. The alert sink forwards only `record.Message`, never attributes (`logging/alerts.go:40-46`). Gaps: `%d`/`%t` formatting leaks (L-2); no end-to-end canary test (M-2). |
| SEC-05 device flow | **Fixed** (deviations recorded in the docs) | `flows.go`: one flow per user (`removeUser`), global cap of 8, 5 s cooldown, `isPolling` in-flight flag, server-side `nextPoll`, terminal outcomes delete the flow, `owned()` gives the same 404 for unknown, foreign or expired ids. Ranges are clamped and the URI is pinned (`github_app.go:131-142`). `verification_uri_complete` is ignored. |
| SEC-06 account binding | **Fixed** | Nothing is stored before `ConfirmGitHub` (`service.go:103-145`). The `replaces` warning shows in `ConfirmStep.tsx`. The numeric id is stored (`service.go:140`). `GITHUB_ALLOWED_LOGINS` is enforced at poll time (`:119-124`). The App being "Only on this account" is an operator setting and cannot be checked in code: **ops condition**. |
| SEC-07 scopes | **Fixed (docs and UI)** | `providerForms.ts:173,182-183` names the exact scopes and has the read-only warning. Optional item 3 (warn on Forgejo write scopes) is not done (informational). |
| SEC-08 AAD | **Fixed, see L-1** | `vault.go:89`: `provider\|column\|base_url\|generation`. The generation is bumped on every save (`catalog.go:104`). A new base URL always needs a new token, because `ConnectForgejo` always takes one. |
| SEC-09 key handling | **Partly fixed** | Key id in the sealed format, and `ErrWrongKey` is shown as needs_reconnect (`box.go:80-95`, `catalog.go:92-97`). The startup warning or refusal for a weak `AUTH_TOKEN_SECRET` is **not implemented** (L-7). |
| SEC-10 fixed hosts | **Fixed** | `wire.go`: `NewGitHubApp(client, id, "", "")`. Stored tokens can reach only github.com and api.github.com. `GITHUB_API_URL` is used only by `envSource`. |
| SEC-11 input limits | **Mostly fixed** | Token ≤512 with no whitespace (`forgejo.go:59-65`), but control and non-ASCII bytes are allowed (L-8). Email ≤254 with no `:` (`bitbucket.go:162-168`). URL ≤300. Provider is an enum on DELETE (`service.go:202`). `DisallowUnknownFields` is kept. |
| SEC-12 admin and audit | **Admin check fixed; audit is partial** | Every `Service` method calls `admin.require()` (`record.go` `Admin.require`, checked in all 9 entry points). Connect, replace and disconnect are audited to the log and ntfy. Transitions to `needs_reconnect` are **not audited** (L-6). |
| SEC-13 transient failures | **Fixed** | `catalog.go:186-189`: network errors, 5xx and 429 are transient. Any OAuth `error` counts as a definitive refusal, which is broader than the plan asked for (L-5). |
| INFO-01 | Fixed | `domain/ticket.go:239` rejects userinfo. |
| INFO-04 | Partly fixed | `KindInvalid` listings are skipped. A duplicate `full_name` in one listing still aborts the provider's sync (I-7). |

CSRF: the router's Origin check applies to all unsafe methods on cookie sessions, and an empty Origin is refused (`router.go:91,122-131`). `GET /api/v1/connections` has no side effects. The MCP layer exposes no connection tools. The poll is a POST. **Agents are refused on all routes** (`rt.human`), but the test covers only 4 of the 9 routes (M-2).

Frontend:
- The verification link is the constant `GITHUB_DEVICE_URL`, opened with `rel="noopener noreferrer"`. The `verification_uri` from the API is never used as an `href`.
- `user_code`, login and error strings are rendered as React text with no `dangerouslySetInnerHTML`.
- The token input is `type=password` and is cleared before the request settles (`secretField.ts`).
- The service worker bypasses `/api/`.
- Nothing logs to the console on these paths.

---

## Findings

There are no CRITICAL or HIGH findings.

### MEDIUM

#### M-1: The SSRF residual is larger than the code comment says, and the status oracle is still there
**Location:** `internal/reposource/client.go:42-46,70-76,115-126`; `internal/connections/service.go:256`; `forgejo.go:52,116-123`.

**What is wrong.**
- The `NewHTTPClient` comment says the guard refuses "addresses that belong to the board's own host". It does not. The board runs in a container, so its host can be reached on the Docker bridge gateway (`172.17.0.1`), on compose-network container names, and on the host's LAN address (`192.168.1.50`). All of these are private and allowed by design (verified: `172.17.0.1` and `fd00::1` both pass). Only the container's own loopback is protected.
- The validation error also still tells these cases apart: "unreachable", "token rejected (HTTP 401/403)", "not found (HTTP 404)", "HTTP <n>", "not the JSON expected", and "did not answer like Forgejo".
- Time to failure separates closed ports (instant) from filtered ones (10 s).

**Exploit scenario.** An attacker holds the admin cookie, for example sniffed over plain-HTTP on the VLAN. They PUT `/api/v1/connections/forgejo` with `base_url=http://172.17.0.1:<port>` in a loop. The responses show which host ports run HTTP services, and the status codes fingerprint them: 404 for a Docker socket proxy at `/api/v1/user`, 401 for an authenticated UI, and so on. The pasted "token" is also sent to every target in `Authorization: token …`.

**Fix.**
1. the deployment host must set `FORGEJO_ALLOWED_HOSTS` to the real Forgejo host. This is a deployment condition for this story, and agents.md or the deploy doc must say so.
2. Make the allowlist port-aware (`host[:port]`), so an allowed host cannot be swept port by port.
3. Collapse the validation messages to the fixed set from the plan (unreachable, not a Forgejo/Gitea API, token rejected, token lacks read access). Log the HTTP status at debug level only. The stored sync error may keep the status code, because only the admin sees it after a successful validation.
4. Correct the `NewHTTPClient` comment so nobody relies on a host-level guarantee the code does not give.

#### M-2: QA conditions from the plan are not covered by tests
**Location:** `internal/httpapi/api_test.go:211-218`; there is no canary test anywhere outside `secretbox`.

**What is wrong.**
- `TestConnectionsArePeopleOnly` checks only 4 of the 9 routes. Not covered: `POST /connections/sync`, `.../{flow}/poll`, `.../{flow}/confirm`, `DELETE .../device/{flow}` and `PUT /connections/bitbucket`.
- The SEC-04 item 5 canary test does not exist. That test drives success, slow_down, refresh refusal, transient refresh failure and Forgejo validation failure, and asserts that the canary never appears in HTTP responses, captured logs, the notifier, stored `repo_source` outcomes, `list_repos` or briefs.
- `TestSecretNeverPrints` covers only `Secret` itself, and only the verbs that are safe (see L-2).

**Exploit scenario.** A regression adds `"error", err` where `err` wraps a `*url.Error` that carries a token in its URL, or someone logs `"record", fmt.Sprintf("%d", rec)`. Nothing fails in CI.

**Fix.** Add the table-driven agent-403 test over all 9 routes, and add the end-to-end canary test above. Both are QA sign-off blockers.

### LOW

#### L-1: The generation goes back to 1 after Disconnect, so a stale refresh can overwrite a new connection (ABA)
**Location:** `internal/store/sqlite/connections.go:56-69` (DELETE, then INSERT with `generation = 1`); `internal/connections/catalog.go:180-216`; `service.go:198-239`. `connect` and `Disconnect` do not take `refreshMu`.

**Exploit scenario.**
1. GitHub @old is at generation 1, and its access token is due for refresh.
2. The hourly sync takes `refreshMu`, reads gen 1, and calls GitHub, which is slow (up to the 30 s timeout).
3. Meanwhile the admin disconnects @old, perhaps because it was the wrong account, and connects @new with a new device flow. That is an INSERT at gen 1.
4. The stale refresh finishes: `Put(expected=1)` → `UPDATE ... WHERE generation = 1` matches the **new** row. The board now silently holds @old's rotated tokens and account name, and no audit is written.
5. Variant: the admin revoked @old at GitHub first. The refresh returns `bad_refresh_token`, and `markNeedsReconnect` writes @old's dead record with `needs_reconnect` over @new.

The AAD does not help here, because it is re-sealed at the new generation.

**Fix.** Make the CAS token unique for the whole life of the table, not just of one row. Either keep the row on disconnect as a tombstone (status `disconnected`, tokens emptied, generation bumped), or compare on a random per-connection id or `connected_at` as well as the generation. Then add that value to the AAD. Alternatively, have `connect` and `Disconnect` for GitHub take `refreshMu`.

#### L-2: `Secret` leaks under `%d`, `%t` and the other non-string verbs
**Location:** `internal/secretbox/secret.go`. It implements `String`, `GoString`, `MarshalJSON` and `LogValue`, but not `fmt.Formatter`.

**Verified:** `fmt.Sprintf("%d", record)` prints `{%!d(string=github) {%!d(string=ghu_CANARY)}}`. `%t` behaves the same way. `%v`, `%+v`, `%#v`, `%s`, `%q` and `%x` are safe.

**Exploit scenario.** A future debug line formats a `Record`, `TokenSet` or `flow` with the wrong verb, or a reflection-based dumper is used. The plaintext token reaches logs. The alert sink is not affected, because it sends only the message.

**Fix.** Implement `Format(fmt.State, rune)` on `Secret` so it always writes `[redacted]`, and extend `TestSecretNeverPrints` to all verbs and to a struct that contains a `Secret`.

#### L-3: Validation reads up to 16 MiB, and the account name from the provider is unbounded
**Location:** `forgejo.go:84-96` and `bitbucket.go:180-194` use `jsonGetter` with `maxResponseBytes = 16 MiB` (`client.go:22`). The docs claim 64 KiB. `service.go:173,193` stores `login` as is.

**Exploit scenario.** A hostile Forgejo, or a hostile Bitbucket `display_name`, returns a multi-megabyte or control-character `login`. It is written to `provider_connections.account`, returned on every `GET /connections`, and pushed in the ntfy audit message (`service.go:319`).

**Fix.** Read validation responses through a 64 KiB limit, as documented. Cap `Account` (for example ≤100 runes, printable only) and refuse anything else as "did not answer like Forgejo/Bitbucket".

#### L-4: `NormalizeForgejoURL` keeps dot segments, encoded separators and invalid ports
**Location:** `forgejo.go:43-56`.

**Verified:**
- `https://git.lan/../..` is stored as is and requests `/../../api/v1/user`.
- `https://git.lan/%2e%2e/%2f` is accepted.
- `https://git.lan:99999` is accepted.
- `isAllowedHost` compares the hostname and ignores the port.

**Impact.** This is limited, because the path prefix is already user-controlled. It does break the "canonical form" property that SEC-01 item 5 and the AAD rely on: two spellings of one server count as different `base_url`s.

**Fix.** Reject `.`/`..` segments and `%2e`, `%2f` and `%5c` in any case. Require a port in 1–65535. Compare the allowlist on `host[:port]`.

#### L-5: Any OAuth `error` on refresh forces a reconnect
**Location:** `github_app.go:164-166`.

**Scenario.** `incorrect_client_credentials`, `unsupported_grant_type` or `unauthorized_client` come from operator misconfiguration, such as a client ID typo during a redeploy. They mark a healthy grant `needs_reconnect`, and the still-valid refresh token is dropped from use. The owner can recover by reconnecting, so this affects availability only.

**Fix.** Treat only `bad_refresh_token`, `invalid_grant` and `expired_token` as definitive. Treat other codes as transient and log them.

#### L-6: The audit trail omits `needs_reconnect` and refresh-driven account changes
**Location:** `catalog.go:211-216`. Only `connect` and `Disconnect` call `audit`.

**Scenario.** Combined with L-1, an account swap or a forced reconnect produces no ntfy push.

**Fix.** Audit (log and ntfy) every write that changes `status` or `account`, as SEC-12 item 2 required.

#### L-7: No strength check on `AUTH_TOKEN_SECRET` (SEC-09 item 1)
**Location:** `config.go:95` checks only that it is at least 32 characters.

**Scenario.** A passphrase-like secret, together with a leaked `DATA_DIR/backups` snapshot, allows offline guessing against the GCM tag. HKDF adds no work factor.

**Fix.** At startup, warn when the value does not decode as at least 32 random bytes in hex or base64. Optionally accept `PROVIDER_TOKEN_KEY` as an override.

#### L-8: Token character set
**Location:** `forgejo.go:59-65` rejects only space, tab, CR and LF.

`net/http` refuses invalid header bytes, so header injection is not possible. But NUL, other control characters and non-ASCII values are stored, and they then fail every sync with a generic "unreachable".

**Fix.** Allow printable ASCII `0x21`–`0x7E` only, as SEC-11 specified.

### INFORMATIONAL

- **I-1:** Flows waiting for confirmation hold live tokens in memory after their 10-minute expiry, until the next `reserve()` runs `sweep` (`flows.go:181-192`). Also sweep in `beginPoll`, `take` and `cancel`.
- **I-2:** `reserve()` checks the cap against `byID` and does not count reservations whose `StartDevice` call is still in flight (`flows.go:64-78`, `service.go:86-93`). Several admins at once could briefly go over 8. There is one admin today.
- **I-3:** `GITHUB_ALLOWED_LOGINS` matches the login, which can be renamed or re-registered, and not the numeric id (`service.go:307-314`). Consider accepting ids.
- **I-4:** Replacing a different login takes a single click on a warning. The docs say "confirms again". Align the docs or add a second step.
- **I-5:** The guard does not block `0.0.0.0/8` (other than `0.0.0.0`), NAT64 `64:ff9b::/96` or 6to4 addresses that embed loopback. None of these can reach anything on the deployment host today.
- **I-6:** `refreshMu.Lock()` does not respect the context. One slow refresh (up to 30 s) blocks "Sync now" requests behind it.
- **I-7:** A hostile listing that repeats a `full_name` makes `Insert` fail with a Conflict, which is not `KindInvalid`. That aborts that provider's whole sync (`repo_service.go` `upsert`). Skip conflicts the same way invalid listings are skipped.
- **I-8:** The frontend keeps polling every interval after the server returns 404 for an expired flow. `pollFailed` keeps the phase at `waiting`. Stop on 404 or when the countdown reaches 0.
- **I-9:** When `askGitHub` gets tokens but `User()` fails, the flow returns to `waiting` with a device code that has already been spent, and the access token is dropped while still valid at GitHub for 8 h (`service.go:114-118,261-268`). This is UX only.
- **I-10:** `POST /connections/sync` has no throttle and runs a full sync inside the request. It is admin-only, so this is provider rate-limit exposure, not a vulnerability.

## Positive observations

- The dial-time `Control` guard sees the resolved address. In testing it refused `localhost`, `[::]`, `0.0.0.0`, `[::ffff:127.0.0.1]` and `169.254.169.254`. Proxies are turned off.
- Redirects are refused at two layers: the shared client, and a second `noRedirects` inside each source constructor.
- The device code never leaves the server, the verification URL is a constant on both ends, and nothing is stored before an explicit confirm.
- The CAS write sits outside network I/O, a lost swap re-reads the row, and refresh is single-flight.
- The sealed format carries a key id, a changed key shows as "reconnect", and a fresh random nonce is used for each value.
- Admin checks are enforced in the service layer, not only in the router.
- Error text is generated by the board throughout. The ntfy alert sink sends only log messages, not attributes.

## Verdict: PASS WITH FIXES

There are no CRITICAL or HIGH findings. Every plan finding is implemented in code. The partial items are SEC-01 (cap, canonical form, oracle), SEC-09 item 1 and SEC-12 (needs_reconnect audit).

**Before QA sign-off:**
- M-1: set `FORGEJO_ALLOWED_HOSTS` on the deployment host, make the allowlist port-aware, and use fixed validation messages.
- M-2: add the 9-route agent-403 test and the end-to-end canary test.

**Should be fixed in this story:** L-1 (ABA), L-2 (`Formatter`), L-3 (64 KiB cap and account bounds).

L-4 to L-8 and the informational items can be scheduled.

Reviewer model: Claude Opus 5.5 (`claude-opus-5-5`).

## Resolution (2026-10-21)

| Finding | Resolution |
|---------|------------|
| M-1 | `FORGEJO_ALLOWED_HOSTS` entries now match `host[:port]` exactly (an entry without a port matches any port). Every failed token validation returns one fixed message; the reason goes only to the server log at info level. The dial-guard comment now states that private LAN, the Docker bridge and the host's LAN IP are reachable by design. Deployments should set `FORGEJO_ALLOWED_HOSTS`. |
| M-2 | `TestConnectionsArePeopleOnly` covers all nine connection routes. `TestTokensNeverLeak` drives connect, transient and refused refreshes and a Forgejo attempt with canary tokens, then checks every log line and serialised view. CI runs `go test -race`. |
| L-1 | Connect and disconnect take the catalog's refresh lock, so no refresh can be in flight across a reconnect. |
| L-2 | `Secret` implements `fmt.Formatter`; every verb prints `[redacted]`, directly and inside structs (tested). |
| L-3 | Identity answers are read with a 64 KiB cap; account names must be at most 100 printable characters. |
| L-4 | The Forgejo URL normaliser rejects `.` and `..` segments, `%2e`, `%2f` and `%5c`, and ports outside 1–65535. |
| L-5 | Only `bad_refresh_token`, `invalid_grant` and expired-token errors force a reconnect; other OAuth errors stay transient. |
| L-6 | Marking GitHub `needs_reconnect` logs a warning and pushes to the board's ntfy topic. |
| L-7 | Not changed: `AUTH_TOKEN_SECRET` must already be at least 32 characters, and the docs require a CSPRNG value. |
| L-8 | Tokens must be visible ASCII. |
