# Security Review: Provider connections plan

**Story:** Provider connections (`docs/connections.md`, proposed 2026-10-21)
**Reviewer:** Security Researcher Agent (Claude Opus 5.5)
**Date:** 2026-10-21
**Verdict:** APPROVE WITH CHANGES

This is a review of the design only. Nothing is implemented yet. Code read: `docs/connections.md`, `docs/design.md`, `docs/api.md`,
`internal/httpapi/{router,respond,server,network}.go`, `internal/auth/{service,signer,secret,throttle}.go`,
`internal/reposource/{client,github,forgejo,bitbucket}.go`, `internal/secretbox/box.go`,
`internal/app/repo_service.go`, `internal/domain/repo.go`, `internal/config/config.go`,
`cmd/ownership/wire.go`, `internal/store/sqlite/db.go`, `internal/logging/alerts.go`, `docker-compose.yml`,
`web/src/lib/markdown.ts`.

---

## Threat model

### Assets
- **Provider credentials.** GitHub App user tokens (access token lasts 8 h, refresh token 6 months, and the refresh works without a client secret). Forgejo/Gitea PATs. Bitbucket API or access tokens. A Bitbucket token may be an unscoped Atlassian API token, which can write to the whole account.
- **`AUTH_TOKEN_SECRET`.** It signs the JWTs and, under this plan, is also the key-encryption root.
- **The repo list.** It includes private repo names and descriptions, and it is the list of targets that agents work on.
- **The board's network position.** The board runs in a container on the Docker host, which shares a network with many unauthenticated LAN services.
- **Board availability**, in particular SQLite write availability for agent leases and heartbeats.

### Threat actors
- **Agents (bearer-token holders, LLM loops that can be prompt-injected).** Most likely to misbehave. They can read the board and write tickets, specs and comments.
- **A LAN/VLAN peer.** The board serves plain HTTP, so a peer can sniff session cookies and pasted tokens. The VLAN is trusted, but that trust is the only control here.
- **A remote phisher.** The GitHub client ID is public and has no secret, so anyone can start a device flow for this app.
- **Hostile or compromised provider responses.** Mainly a user-supplied Forgejo host. Org members also control repo descriptions.
- **A stolen DB or backup** (`VACUUM INTO` snapshots in `DATA_DIR/backups`).
- **The owner by mistake**, for example by pasting an over-privileged token or an internal URL.

### Attack surface introduced
- Five new human-only endpoints under `/api/v1/connections`.
- An in-memory map of device flows.
- Outbound calls to github.com/api.github.com and to a **user-chosen** Forgejo URL.
- A new table of encrypted secrets.
- A background refresh with rotation.
- New error strings that reach the UI, the logs and the ntfy alert sink.

---

## Findings (in priority order)

### HIGH

#### SEC-01: Forgejo `base_url` turns the board into an SSRF pivot with a read-back oracle
**Domain:** Input validation / SSRF. **Location:** connections.md "Forgejo/Gitea and Bitbucket", `reposource/client.go` `statusError`, `wire.go` shared `http.Client`.

**Risk.** `base_url` may be any http(s) host, including `127.0.0.1`, the Docker bridge gateway (`172.17.0.1`, a common place for `:2375` or a socket proxy), container names on the compose network, `169.254.0.0/16`, and every LAN service on 192.168.1.0/24. Three things make it worse:
- `statusError` puts up to 300 bytes of the target's response body into the error.
- Go's dial errors tell "connection refused", "no such host" and "timeout" apart.
- Both kinds of error come back in the PUT response and in the stored sync outcome that Settings shows.

Together this is a port scanner plus a banner grabber for the Docker host and the VLAN. The design allows a path in `base_url`, so the attacker also partly controls which path is requested. A save-time hostname check alone is defeated by DNS rebinding, because the hostname is resolved again on every hourly sync.

**Attack scenario.** Someone obtains the owner's session: by sniffing the plain-HTTP cookie on the VLAN, through a future XSS in agent-written markdown, or by walking up to an unlocked browser. They PUT `{base_url:"http://172.17.0.1:2375", token:"x"}`, read the error, and repeat across ports and hosts. The board maps the internal network for them and returns response snippets. Hourly sync keeps probing a rebinding hostname.

**Required changes to the design.**
1. Check the destination when the connection is dialled, not only when the form is saved. Enforce this on every connection, for the save-time validation and for syncs. Always refuse loopback, link-local (169.254/16, fe80::/10), unspecified, multicast and the IPv4-mapped forms of these.
2. Add an operator allowlist setting (for example `FORGEJO_ALLOWED_HOSTS`, a list of host[:port]). When it is set, no other destination can be dialled. Docs should tell the deployment host to set it to the real Forgejo host. Do not try to block all private ranges, because the legitimate Forgejo is on the LAN.
3. Responses must not reflect upstream content. The PUT returns one of a fixed set of messages: `unreachable`, `not a Forgejo/Gitea API`, `token rejected`, `token lacks read access`. It never includes response body bytes or dial-error detail. The stored sync error follows the same rule. Full detail goes to the server log at debug level only, and that log must not feed the ntfy alert sink.
4. Validation passes only if the response is JSON shaped like a Forgejo user (`id` is a number, `login` is a non-empty string). Cap the validation response at a small size (64 KiB, not 16 MiB). Use a short timeout of no more than 10 s.
5. Normalise `base_url` before storing it: lowercase scheme and host, no userinfo, query or fragment, no `.`/`..` or percent-encoded dot or slash segments, a length cap, and no trailing slash. Store and display it in that canonical form.
6. Never follow redirects (see SEC-03).

#### SEC-02: The refresh-rotation design is unsafe as written ("saved in the same transaction that reads the old one")
**Domain:** Business logic / race / availability. **Location:** connections.md "Token lifetime", `store/sqlite/db.go` (`_txlock=immediate`, `busy_timeout=5000`).

**Risk.** The design can be built in only two ways, and both fail:
- **(a) The HTTP refresh call runs inside the DB transaction.** Every write transaction is `BEGIN IMMEDIATE`, so the SQLite write lock is held for up to the 30 s outbound timeout. Every other write on the board (agent claims, heartbeats, comments) then fails after the 5 s busy timeout. A slow GitHub becomes a board-wide outage that can expire agent leases.
- **(b) Read, then call, then write in separate transactions.** Three triggers can run at once: the hourly ticker, "Sync now", and the sync that starts after connecting. Two of them refresh with the same refresh token. GitHub rotates the token on the first call, so the second gets `bad_refresh_token` and marks the connection `needs_reconnect`. That status overwrites the good token pair the first one saved, or a stale writer overwrites newer tokens.

**Required changes to the design.**
1. Refresh runs under a single-flight lock, one per provider connection, inside the process. Concurrent callers wait for the one refresh and reuse its result. The deployment is a single process. State that as an assumption.
2. No network I/O while a DB transaction is open.
3. The write is a compare-and-swap on a `generation` (or `updated_at`) column read before the call. If the swap fails, the board re-reads the row and uses the stored tokens. It does not mark `needs_reconnect`.
4. Only a definitive answer from GitHub marks `needs_reconnect`: `bad_refresh_token`, `invalid_grant`, or a revoked or expired refresh token. Network errors and 5xx/429 responses keep the current status and retry on the next sync.
5. Accept and document one residual risk: if the process crashes after GitHub rotates the tokens but before they are committed, the refresh token is lost and the connection needs a reconnect.

---

### MEDIUM

#### SEC-03: Redirects are followed today, and Go keeps `Authorization` on same-hostname redirects
**Domain:** Data exposure (systemic, in existing code). **Location:** `wire.go` `&http.Client{Timeout: outboundTimeout}` has no `CheckRedirect`. Also `bitbucket.go` pagination check.

**Risk.** The design says "Redirects are not followed when the request carries a token", but every provider request carries a token, and the existing shared client does follow redirects. Go strips `Authorization` only when the redirect goes to a different hostname. Redirects to the same hostname on another port, or from `https` down to `http`, keep the header. So a Forgejo at `https://git.lan` that redirects to `http://git.lan:8080` sends the PAT in clear text. A redirect is also a way around SEC-01 checks made only at save time. Separately, Bitbucket's `next`-link guard compares only the host (`hostOf` drops the scheme), so an `http://api.bitbucket.org/...` link would pass.

**Required changes.** All provider HTTP clients refuse every redirect and treat a 3xx as an error. This covers GitHub OAuth endpoints, api.github.com, Forgejo, Bitbucket, and the existing env-token path. Bitbucket pagination must match the scheme, host and port exactly against `https://api.bitbucket.org`. Fix this in existing code as part of this story.

#### SEC-04: Token leakage paths the design does not close
**Domain:** Secrets / logging. **Location:** connections.md "Storage" (the "never appear in ... logs, errors" claim), `respond.go` `writeError`, `logging/alerts.go`, `repo_service.go` `outcome.Error`.

**Risk.** Tokens can escape through paths the design never mentions:
- **Device-flow fields in the URL.** GitHub's OAuth endpoints also accept `device_code` and `refresh_token` as query parameters. If an implementation puts them there, Go's `*url.Error` includes the full URL. That error then goes to `logger.Error`, to the ntfy alert sink (possibly a public ntfy server), and into `outcome.Error`, which is stored and shown.
- **Response bodies in errors.** The token endpoint returns secrets in a 200 body. Any "decode failed: <body>" style error, or reuse of `statusError` there, leaks them.
- **Struct dumps.** Logging a whole connection struct leaks the decrypted tokens.

**Required changes.**
1. `device_code`, `refresh_token` and every token travel only in a POST form body or an `Authorization` header, never in a URL.
2. Token-endpoint errors never include response bodies. Only the OAuth `error` code (for example `authorization_pending`) appears.
3. Decrypted tokens are held in a type whose string, JSON and slog forms all print as redacted. The design must state this as a required property.
4. The connection domain object exposed to the HTTP DTO layer has no token fields at all.
5. QA condition: a test seeds canary token values. It drives success, failure, slow_down, refresh failure and Forgejo validation failure, then asserts the canaries never appear in responses, logs, alerts, `repo_source` outcomes, briefs or MCP output.

#### SEC-05: Device-flow state machine and rate limits are under-specified
**Domain:** Abuse prevention / business logic. **Location:** connections.md "Flow".

**Risk.**
- **Concurrent polls.** Two polls on the same flow (two tabs, a retry) can both call GitHub. The loser gets `expired_token` or `incorrect_device_code` and could report `expired` after a success, or overwrite a just-stored connection.
- **Vague cap.** "A few per person" with no global cap, plus a new GitHub request on every `POST /device`, means a looping client can drain the client ID's rate limit at GitHub.
- **Leaky lookups.** A missing flow and someone else's flow are not specified to look the same.
- **Unvalidated GitHub fields.** Values from GitHub (`interval`, `expires_in`, `user_code`) are not range-checked.

**Required changes.**
1. Each flow has a mutex or in-flight flag, so at most one GitHub token request is outstanding per flow. A second concurrent poll returns `pending` without calling GitHub.
2. The server keeps `next_allowed_poll_at`. A poll before that time returns `pending` (with the interval) and makes no GitHub call. `slow_down` adds 5 s and persists.
3. Terminal states (`connected`, `denied`, `expired`) remove the flow immediately. A later poll returns `404`.
4. A person has at most one active flow. Starting a new one cancels the old one. Add a global cap (for example 10 flows) and refuse new flows when it is reached. Sweep expired flows on every access.
5. An unknown, expired or someone else's `flow_id` returns the same `404 not_found`.
6. Validate GitHub's response before using it: `verification_uri` exactly equal to `https://github.com/login/device` (already in the design); `interval` between 5 and 60; `expires_in` no more than 900–1800; `user_code` matching `^[A-Z0-9]{4}-[A-Z0-9]{4}$`. Ignore `verification_uri_complete`.
7. Apply an existing-style throttle to `POST /device`, for example 5 per 15 minutes per user.

#### SEC-06: Account binding, and why the client ID must stay metadata-only and private
**Domain:** Authentication / business logic. **Location:** connections.md "GitHub", step 4.

**Risk.** The flow binds the board to whichever GitHub account enters the `user_code` first.
- **Wrong account.** Someone who sees the code within its 15-minute life can authorize with their own account: a shoulder-surfer, a screen-share viewer, or an agent reading a screenshot. The board then silently syncs their repos. Their repo names and descriptions become agent work targets and prompt-injection carriers.
- **Reverse phishing.** The client ID is public and has no secret, so anyone can start a flow for this app and phish the owner into entering an attacker's code. The attacker then gets a token for the owner's account with the app's permissions. Today that is metadata only: private repo names, descriptions and visibility.
- **Third-party installs.** If the app is registered as public, third parties can install it on their own orgs, and those installations show up in `/user/installations` for any member.

**Required changes.**
1. The App must be registered as **"Only on this account"** (private). agents.md must say so.
2. After a flow succeeds, the connection is stored as `pending_confirmation`, showing `@login` (and the numeric user id). It is used only after the person confirms in the UI. Alternatively, add an optional `GITHUB_ALLOWED_LOGINS` env setting. Either way, replacing an existing connection that has a **different** login needs an explicit confirmation.
3. Store the GitHub numeric `id` as well as the login, because logins can be renamed.
4. The design must state that the app's permissions are a security boundary. Adding Contents or any write permission later needs a new security review, because the public, secretless device flow would then let phishers get code access.
5. agents.md tells the owner to enter only codes the board itself displays.

#### SEC-07: Least privilege for pasted tokens is only advice
**Domain:** Least privilege. **Location:** connections.md "Both are entered only by a signed-in person".

**Risk.**
- **Forgejo scopes.** `GET /api/v1/user` needs `read:user`, but the guidance names only `read:repository`. Validation would fail, and people then create an all-scopes token to make it work.
- **Bitbucket tokens.** "Email + token" basic auth accepts unscoped Atlassian API tokens, which have full account write access. One such token stored on the board turns any DB-plus-secret leak into a write-capable account compromise.

**Required changes.**
1. The docs and the form name the exact minimum scopes: Forgejo `read:repository` and `read:user`; Bitbucket scoped API token with `read:repository:bitbucket` and `read:user:bitbucket`, or a workspace or repository access token with repository read.
2. The form recommends scoped tokens only, and shows a one-line warning that the board cannot verify a token is read-only.
3. If Forgejo returns scope information, the board warns when it sees a write or admin scope.

---

### LOW

#### SEC-08: AAD should bind to the destination, not just to provider and column
**Location:** connections.md "Storage". Today the associated data is `provider + column`, so a ciphertext stays valid if `base_url` changes. A later "edit URL" feature, a migration or a bug that updates `base_url` alone would then silently send the stored Forgejo token to the new host.

**Change:**
1. The associated data is `ownership_board/provider-tokens/v1|<provider>|<column>|<canonical base_url or fixed GitHub host>|<connection generation or id>`.
2. Any change of `base_url` requires the token to be entered again. The design must state this.

#### SEC-09: Key derivation: keep HKDF from `AUTH_TOKEN_SECRET`, but harden the input and plan for rotation
A separate key is not required. HKDF with a distinct `info` gives an independent key. Rotating both together is acceptable and is the desired behaviour after a compromise. Putting a key file in `DATA_DIR` would be worse, because backups copy that directory. Remaining gaps:
- `AUTH_TOKEN_SECRET` only needs to be 32 characters, which could be a weak passphrase. HKDF is not a password KDF, so a leaked backup allows offline guessing against the GCM tag.
- The `v1:` prefix does not identify a key.

**Change:**
1. At startup, warn (or refuse) when `AUTH_TOKEN_SECRET` does not look like at least 32 random bytes (hex or base64). Optionally allow `PROVIDER_TOKEN_KEY` to override the derived key.
2. Sealed values carry a key id, so rotation can be detected and reported as "reconnect needed" instead of a generic open error.
3. Document that `.env` must never live in `DATA_DIR` or inside the backups.

#### SEC-10: Stored GitHub-App tokens must be sent only to github.com
`GITHUB_API_URL` exists for GitHub Enterprise Server and env tokens. If the stored-connection path reuses `NewGitHub(client, cfg.GitHubAPIURL, token)`, a github.com user token would be sent to whatever host that setting names.

**Change:** stored GitHub connections always use the fixed `https://api.github.com` and `https://github.com` endpoints. Device flow is turned off when `GITHUB_API_URL` is not the default, until GHES is designed for.

#### SEC-11: Input limits on the new request bodies
**Change:**
- `token`: printable ASCII, no whitespace or control characters, at most 512 bytes.
- `email`: at most 254 bytes, a valid address, no `:`.
- `base_url`: at most `MaxURLLength`, plus SEC-01's normalisation.
- `{provider}` path value: one of an enum (anything else returns 404).
- Unknown JSON fields are rejected (already the case).

#### SEC-12: Admin gate and audit trail for connection changes
Today only one admin user exists, so `human()` is enough in practice.

**Change:**
1. Connection endpoints also require `IsAdmin`, so the rule keeps holding if more people are added later.
2. Connect, replace, disconnect and `needs_reconnect` events are written to the log (provider, account, user id; never tokens) and sent to the owner via ntfy. A silent replacement (SEC-06) then cannot go unnoticed.

#### SEC-13: Transient failures must not force a reconnect
This is part of SEC-02 item 4, stated separately for QA. A network blip or a GitHub 5xx must not flip the connection to `needs_reconnect`.

---

### INFORMATIONAL

- **INFO-01: Provider metadata is untrusted and goes into agent briefs.** Repo names, descriptions and `html_url` are controlled by anyone who can edit a repo in an installed org, or by a hostile Forgejo. They reach LLM agents through `GET /api/v1/repos`, `list_repos` and briefs. "All repositories" across orgs widens this, and it also shows private org repo names to every agent-token holder. The briefs should mark the description as untrusted data. Synced repo URLs with userinfo should be rejected (today `parseOptionalURL` accepts `https://user:pass@...`). The owner should know that every agent sees every private repo name.
- **INFO-02: Disconnect does not revoke at GitHub.** The board has no client secret, so it cannot call the revoke API. A leaked DB plus secret gives up to 6 months of refreshable access, limited to metadata. The design already tells the owner to revoke in GitHub settings. Keep that text.
- **INFO-03: Plain HTTP on the VLAN.** Pasted Forgejo and Bitbucket tokens and the session cookies cross the VLAN in clear. This is accepted per the deployment context. Note that a sniffed cookie is exactly what turns SEC-01 into a live pivot. The form should also warn when `base_url` is `http://`, because the PAT is then sent in clear on every sync.
- **INFO-04: One bad listing aborts the provider's whole sync** (`upsert` returns on the first invalid repo). Hostile Forgejo data can freeze that provider's list. The bad entry should be skipped and logged instead.

---

## Agent tokens and CSRF (checked; conditions for QA)

- **Agents get 403.** `router.authorize` with `accessHuman` rejects agent actors (`requireKind`), and a bearer request never falls back to cookies. Registering every connection route with `rt.human(...)` is enough. **QA conditions:**
  1. A test sends an agent bearer token to each of the five routes and expects 403.
  2. No MCP tool exposes connection status, sync errors, or "sync now".
  3. `GET /api/v1/repos` and `/repo-sources` responses never carry connection fields.
- **CSRF.** Every state change is POST, PUT or DELETE, so the Origin check (exact `PUBLIC_BASE_URL`, or Host match when it is unset) plus SameSite=Lax covers them. Poll is correctly a POST. DNS rebinding beats the Host fallback but brings no cookies, so it reaches nothing on these routes. **Condition:** no GET handler under `/connections` may have side effects (no "GET to start a flow"). Recommend setting `PUBLIC_BASE_URL=http://192.168.1.50:8660` on the deployment host so the strict comparison is used.

## Positive observations

- `device_code` stays on the server and the flow is tied to the user who started it, so the browser never holds a credential that can be redeemed.
- `verification_uri` is pinned to a constant.
- The board holds no client secret and no private key, and the app permission is metadata-only. That is real least privilege.
- AES-256-GCM through the existing `secretbox`, with a random nonce per value, an HKDF purpose label, a version prefix, and associated data per column.
- Routing is default-deny (`router` is the only way to register a route), and agents and people are separated by kind.
- Tokens are never returned by the API. Replacing a connection needs a new token, so a stolen session cannot read out or redirect an existing token.
- The Bitbucket pagination host guard and the response-size caps already exist.
- Markdown is sanitised with DOMPurify, and a CSP and frame-deny are already set, which lowers the chance of reaching these endpoints through XSS.

## Verdict: APPROVE WITH CHANGES

There are no CRITICAL findings. The two HIGH findings have clear resolutions that stay inside the current architecture:
- SEC-01: dial-time destination policy, optional allowlist, no reflected responses, no redirects.
- SEC-02: single-flight refresh outside the transaction, compare-and-swap write, definitive-error-only reconnect.

`docs/connections.md` must be updated with SEC-01 through SEC-07 before implementation starts. QA must verify the canary-token test (SEC-04), the agent-403 and MCP conditions, and SEC-08 through SEC-13 before sign-off.

Reviewer model: Claude Opus 5.5 (`claude-opus-5-5`).
