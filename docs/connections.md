# Provider connections (design)

Status: proposed, 2026-10-21. Replaces "put a token in `.env`" as the main way
to sync repos. Env tokens keep working as a fallback.

## Goal

A person clicks **Connect GitHub**, enters a short code at
github.com/login/device, and the board keeps their full repo list in sync from
then on. Forgejo/Gitea and Bitbucket get a "paste a token" form in Settings
instead of server env vars. Nothing about this needs the board to be reachable
from the internet: every call goes from the board out to the provider.

## GitHub: a GitHub App with the device flow

The operator registers a GitHub App once (docs/agents.md gets the steps):

- Permissions: **Repository → Metadata: read-only**, nothing else. The token
  can list repos and read their names, URLs, descriptions, default branches
  and visibility. It cannot read code, issues or anything else, and cannot write.
- "Enable Device Flow" on. Webhooks off. No callback URL.
- Installed on the operator's account (and any orgs) with **All repositories**,
  so repos created later are included automatically.
- Only the **client ID** goes to the board, as `GITHUB_APP_CLIENT_ID`. It is a
  public identifier. No client secret, private key or webhook secret exists on
  the board: device-flow tokens refresh without a secret.

### Flow

1. `POST /api/v1/connections/github/device` (person only, origin-checked). The
   board calls `POST https://github.com/login/device/code` with the client ID
   and `scope=offline_access` (asks for expiring tokens). It keeps the
   `device_code` **server side** in memory, keyed by a random 32-byte flow id
   bound to the person who started it, and returns only
   `{flow_id, user_code, verification_uri, expires_at, interval}`.
2. The UI shows the code with a copy button and a link that opens
   `verification_uri` (always `https://github.com/login/device`; the board
   rejects any other value from GitHub) in a new tab.
3. The UI calls `POST /api/v1/connections/github/device/{flow_id}/poll` every
   `interval` seconds. Each call makes at most one token request to GitHub and
   never faster than the current interval (`slow_down` adds 5 s). Results:
   `pending`, `connected`, `expired`, `denied`. Only the person who started a
   flow can poll it.
4. On success the board reads `GET https://api.github.com/user` for the login,
   stores the connection, ends the flow, and starts a sync in the background.

Pending flows expire with GitHub's `expires_in` (15 minutes) and are capped at
a few per person, so the in-memory map cannot grow without bound.

### Listing repos

`GET /user/installations`, then `GET /user/installations/{id}/repositories`
for each, 100 per page with the existing page cap. The result is the
intersection of what the person can see and what the app is installed on.
Suspended installations are skipped.

### Token lifetime

Access tokens last 8 hours and refresh tokens 6 months. Before each sync, the
board refreshes the access token if it expires within 10 minutes
(`grant_type=refresh_token`, client ID only). Refreshing rotates both tokens;
the new pair is saved in the same transaction that reads the old one, so two
syncs racing cannot both use one refresh token. A failed refresh marks the
connection `needs_reconnect`, which Settings shows. If GitHub returns a
non-expiring token, it is stored without refresh data.

## Forgejo/Gitea and Bitbucket: paste a token

- Forgejo: `{base_url, token}`. The board validates with
  `GET {base_url}/api/v1/user` before saving and stores the login.
  `base_url` must be http(s) with a host, no credentials, query or fragment.
  Redirects are not followed when the request carries a token.
- Bitbucket: `{email?, token}`, validated with `GET /2.0/user`.

Both are entered only by a signed-in person. The guidance says to use
read-only tokens (Forgejo `read:repository`, Bitbucket repository read).

## Storage

New table `provider_connections` (one row per provider):
`provider, base_url, account, email, access_token_enc, refresh_token_enc,
access_expires_at, refresh_expires_at, status, connected_by, connected_at,
updated_at`.

Tokens are encrypted with AES-256-GCM: random 96-bit nonce per value, and the
provider name plus column name as additional data, so a ciphertext cannot be
moved to another row or column. The key is derived with HKDF-SHA256 from
`AUTH_TOKEN_SECRET` (info `ownership_board/provider-tokens/v1`). Rotating
`AUTH_TOKEN_SECRET` therefore also invalidates stored tokens, and Settings asks
to reconnect. Tokens never appear in API responses, logs, errors or briefs.

If both a stored connection and env tokens exist for a provider, the stored
connection wins.

## API (admins only; agents get 403)

| Method | Path | Body | Result |
|--------|------|------|--------|
| GET | `/api/v1/connections` | | `Connection[]` (github, forgejo, bitbucket, always all three) |
| POST | `/api/v1/connections/sync` | | `Connection[]` after syncing every connected provider |
| POST | `/api/v1/connections/github/device` | | `DeviceFlow` |
| POST | `/api/v1/connections/github/device/{flow_id}/poll` | | `DevicePoll` |
| POST | `/api/v1/connections/github/device/{flow_id}/confirm` | | `Connection`; stores it and starts a background sync |
| DELETE | `/api/v1/connections/github/device/{flow_id}` | | `204`, cancels the flow |
| PUT | `/api/v1/connections/forgejo` | `{base_url, token}` | `Connection` (validated, then a background sync starts) |
| PUT | `/api/v1/connections/bitbucket` | `{email, token}` (`email` may be empty for an access token) | `Connection` |
| DELETE | `/api/v1/connections/{provider}` | | `204`, forgets the stored tokens |

```ts
type ConnectionStatus = "disconnected" | "connected" | "needs_reconnect";
type Connection = {
  provider: "github" | "forgejo" | "bitbucket";
  label: string;                 // "GitHub", "Forgejo / Gitea", "Bitbucket"
  is_available: boolean;         // github: GITHUB_APP_CLIENT_ID set; others: always true
  status: ConnectionStatus;
  managed_by: "settings" | "server_env" | null; // env tokens show as server_env, read-only in the UI
  account: string;               // "@octocat" style login, "" when disconnected
  base_url: string;              // forgejo only
  email: string;                 // bitbucket only
  connected_at: string | null;
  repo_count: number;
  last_sync: { at: string; error: string } | null;
};
type DeviceFlow = {
  flow_id: string; user_code: string;
  verification_uri: "https://github.com/login/device";
  expires_at: string; interval_seconds: number;
};
type DevicePoll = {
  status: "pending" | "confirm" | "expired" | "denied";
  interval_seconds: number;
  login: string;                 // set with "confirm"
  replaces: string;              // login of the currently connected account it would replace, or ""
  message: string;               // human-readable reason for "denied"
};
```

Disconnect only deletes the board's copy. The UI says the person can also
remove the app's authorization in their GitHub settings.

## Repos that disappear

A repo a provider stops listing is kept (tickets may point at it) and gets
`missing_since`; it is cleared if the repo comes back. The UI labels it "No
longer on GitHub" and hides it from the repo picker.

## Settings screen

Repositories becomes: one card per provider. Unconfigured providers are hidden
behind "Add a provider". GitHub shows **Connect GitHub** when the client ID is
set, and "Connected as @login · N repos · synced 5m ago · Sync now ·
Disconnect" once linked. Manual repos move behind a small "Add a repo by
hand" link.

## Security decisions (from the plan review, docs/security/connections-plan-review.md)

These override anything above that disagrees.

- **Admins only.** Every connection endpoint is a person route and also
  requires `is_admin`. Agent tokens get 403. No MCP tools touch connections.
- **Confirm the account (SEC-06).** A finished device flow does not connect
  anything by itself. The poll answers `confirm` with the GitHub login, and the
  board stores nothing until the admin confirms that login (and confirms again
  if it replaces a different, already-connected login). Optional
  `GITHUB_ALLOWED_LOGINS` refuses any other account outright. The numeric
  GitHub user id is stored. Register the App as "Only on this account".
  Adding any permission beyond Metadata needs a new security review.
- **Device flows (SEC-05).** One active flow per person; starting a new one
  cancels the old. At most 8 flows in total, and a person may start one every
  5 seconds. Each flow allows one GitHub call at a time, and the server enforces
  the interval: a poll before then is answered `pending` without calling GitHub.
  Terminal flows are deleted. Unknown and foreign flow ids get the same 404.
  `interval` (5–60 s), `expires_in` (60–1800 s) and `user_code` (≤16 chars,
  `[A-Z0-9-]`) are range-checked. `verification_uri_complete` is ignored.
- **Refresh without races (SEC-02, SEC-13).** No network call ever happens
  inside a database transaction. One in-process lock serialises refreshes per
  provider. Saves use a compare-and-swap on a `generation` column; on a lost
  swap the board re-reads and uses the stored tokens. Only a definitive refusal
  from GitHub (an `error` in the token response) marks `needs_reconnect`.
  Network errors, 5xx and 429 just fail that sync and are retried next time.
- **No redirects, anywhere (SEC-03).** Every provider client refuses all
  redirects, including the env-token clients. Bitbucket `next` links must be
  https on the same host.
- **Fixed errors (SEC-01, SEC-04).** Provider errors carry only a status code
  and a fixed hint: never response bytes, never dial details. Secrets travel
  only in POST bodies or headers, never in URLs. Decrypted tokens use a
  `Secret` type that prints and marshals as `[redacted]`.
- **Forgejo SSRF (SEC-01).** The URL is normalised (scheme + host + optional
  port + path, no credentials, query or fragment) and every connection is
  checked at dial time, which also defeats DNS rebinding: loopback,
  link-local (including 169.254.169.254), unspecified and multicast addresses
  are refused. Private LAN addresses stay allowed because self-hosted forges
  live there. Optional `FORGEJO_ALLOWED_HOSTS` limits it to named hosts.
  Validation requires a response shaped like a Forgejo user, read with a 64 KiB
  cap and a 10 s timeout.
- **Scopes (SEC-07).** Forgejo: `read:user` and `read:repository`. Bitbucket: a
  scoped API token or access token with `read:repository:bitbucket` and
  `read:user:bitbucket`. The form names these and warns that the board cannot
  verify that a token is read-only.
- **Encryption details (SEC-08, SEC-09).** The associated data is provider,
  column, canonical base URL and generation. Sealed values carry a key id, so
  a changed `AUTH_TOKEN_SECRET` reads as "reconnect" rather than garbage. The
  key stays derived from `AUTH_TOKEN_SECRET` (kept in `.env`, outside
  `DATA_DIR`, so backups never hold the key next to the ciphertext).
- **Fixed hosts (SEC-10).** Stored GitHub App tokens only ever go to
  github.com and api.github.com. `GITHUB_API_URL` applies to the env PAT only.
- **Audit (SEC-12).** Connect, replace and disconnect are logged and pushed to
  the board's ntfy topic.
- **Untrusted metadata.** Repo URLs with embedded credentials are rejected. One
  bad listing is skipped and logged instead of aborting the whole sync.

## Out of scope

Webhooks (the board is LAN-only, so GitHub cannot reach it), reading PR status,
and any write access.
