# Forgejo: turning it on

The board treats Forgejo (and Gitea) as a repo host next to GitHub. Repos
sync from both, agents work on both, and the merge sweep applies the same
rules to both. This page is the owner's checklist. The details live in
[connections.md](connections.md#agent-access-forgejo) and [api.md](api.md).

Examples use `https://git.example.lan` for the Forgejo address and
`ob-bot` for the account agents act as.

## 1. A Forgejo instance

Any Forgejo 9 or newer (or Gitea 1.22+) works. The board needs to reach it
from its container. Agents' machines need to reach it too, for git over
HTTP(S) and the API.

- Set `ROOT_URL` to the address people use. The board does not depend on it:
  it clones and calls the API at the address you connect with, even when
  Forgejo reports another `ROOT_URL`.
- **Make the instance private before agents can reach it.** Agent sandboxes
  that may reach the forge can read whatever it serves anonymously. Set
  `[service] REQUIRE_SIGNIN_VIEW = true`, or make every repo private.
- Optional: on the board, set `FORGEJO_ALLOWED_HOSTS=git.example.lan:443` so
  the board refuses any other forge address.

## 2. The token

Agents act as whichever account owns the token. A dedicated account (for
example `ob-bot`) keeps their pushes, pull requests and merges apart from
yours. Add it as a collaborator with **Write** on each repo agents work on, or
to an organization team with write access.

Sign in as that account, then **Settings → Applications → Generate new
token**:

| Field | Value |
|---|---|
| Token name | `ownership-board` |
| Repository and Organization Access | **All** (public, private and limited) |
| Permissions: repository | **Read and write** (push, pull requests, reviews, merges, commit statuses) |
| Permissions: user | **Read** (the board checks who the token belongs to and lists its repos) |
| Everything else | No access |

For repo sync only, without agents, use repository **Read** instead.

Forgejo tokens do **not expire**. Rotate the token yourself (generate a new
one, reconnect, delete the old one), and delete it in Forgejo to cut access
at once.

## 3. Connect it on the board

**Settings → Repositories → Add a provider → Forgejo / Gitea**:

| Field | Value |
|---|---|
| Base URL | `https://git.example.lan`, the root address with no path to a repo. Plain `http://` works on a LAN, but the form warns that the token then crosses the network unencrypted. |
| Token | the token from step 2 |
| Let board agents use this token | **on** if agents should work on these repos; off for sync only |

Press **Connect**. The board checks the token against `/api/v1/user` before
saving it, then syncs. To confirm it worked:

- The Forgejo card shows the account login, a repo count, "Board agents may
  use this token", and a recent sync with no error.
- **Repos** lists the Forgejo repos with the Forgejo mark. GitHub repos stay
  as they were. Both providers can be connected at once, and a repo with the
  same `owner/name` on both is two separate board repos.
- **Sync now** on the card re-runs the sync; it also runs on its own schedule.

To change the token or the switch later, use **Reconnect** on the card. The
board stores the token encrypted (see connections.md, "Storage").

## 4. Agents and merge conditions

### How agents get access

A worker asks the board for a Forgejo repo's credential with
`GET /api/v1/agent/repos/{id}/forge-credential`. The board answers only an
agent token, only for a Forgejo repo, and only while the switch is on. The
worker supervisor keeps the token out of the model's environment, passes it
to git through a credential helper (never on a command line), and clones and
pushes to the address you connected.

### What the merge sweep checks on Forgejo

The rules are the same as on GitHub, read from Forgejo's API:

| Condition | Where it comes from on Forgejo |
|---|---|
| Open, draft, merged, closed | the pull request |
| Conflict | `mergeable: false` that stays false for 10 minutes. Forgejo reports false both while checking and on a conflict. |
| Review | each reviewer's latest approve or request-changes; any request for changes blocks. Required approvals come from branch protection. |
| Checks | the combined commit status of the PR head. Each Forgejo Actions job reports one status. A required status context that never reported counts as *expected*, so the PR waits. |
| Unresolved and manual-check markers | the PR body, as on GitHub |
| Merge | a merge commit of exactly the head that was checked (`head_commit_id`). Forgejo enforces branch protection on the merge itself. |

What Forgejo has no API for, and therefore never happens there: reading CI
job logs (repair passes see the failing check names without a log tail),
re-running or cancelling CI from the board, and listing runners.

### Set up each repo

1. **Actions.** Turn on Actions for the repo and register a
   [Forgejo runner](https://forgejo.org/docs/latest/admin/actions/). Its
   labels must cover every `runs-on:` in the workflows: a job runs only on a
   runner that has *all* of the job's labels. Workflows that `uses:`
   GitHub-hosted actions need the instance to fetch them, either through
   `[actions] DEFAULT_ACTIONS_URL = https://github.com` or with full URLs in
   `uses:`.
2. **Branch protection** (repo **Settings → Branches → Add rule** for the
   default branch):
   - **Enable status check** with the contexts that must pass. Forgejo names
     them `<workflow name> / <job name> (<event>)`, for example
     `ci / test (pull_request)`. Patterns such as `ci / *` are allowed.
   - Required approvals: 0 if the board's own review and checks are enough,
     more if a person must approve.
   - Block merge on rejected reviews: on.
   - Leave force-merge off. The sweep never force-merges, so protection
     binds it even when the token's account is an admin.
3. **On the board**, set the repo's merge and deploy settings exactly as for a
   GitHub repo. **Agents merge green PRs** is off until you turn it on.
