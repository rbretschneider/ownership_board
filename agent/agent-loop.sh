#!/usr/bin/env bash
# Runs an agent against ownership_board without spending model tokens while idle.
#
# The loop long-polls the board with curl. Only when a ticket is claimed does it
# start a headless Claude Code session, handing it the brief. That session
# reports back through the board's MCP tools (see docs/agents.md for setup).
#
# Usage, from inside the repository the agent should work in:
#   OWNERSHIP_URL=https://board.example.com OWNERSHIP_TOKEN=ob_pat_... \
#     ./agent-loop.sh acme/api
#
# Environment:
#   OWNERSHIP_URL    board origin (required)
#   OWNERSHIP_TOKEN  agent token from Settings > Agent tokens (required)
#   CLAUDE_ARGS      extra arguments for `claude -p`, for example
#                    "--permission-mode acceptEdits"
#   WAIT_SECONDS     long-poll length per request (default 300)
set -euo pipefail

repo="${1:-}"
url="${OWNERSHIP_URL:?set OWNERSHIP_URL}"
token="${OWNERSHIP_TOKEN:?set OWNERSHIP_TOKEN}"
wait_seconds="${WAIT_SECONDS:-300}"
backoff_seconds=30

claim() {
  local body
  body=$(printf '{"repo":"%s","wait_seconds":%d}' "$repo" "$wait_seconds")
  curl --silent --show-error --max-time $((wait_seconds + 30)) \
    --write-out '\n%{http_code}' \
    -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -X POST "$url/api/v1/agent/claim" -d "$body"
}

# brief extracts the "brief" string from the claim response without needing jq
# to be installed; python3 is used when present for exact JSON decoding.
brief() {
  if command -v jq >/dev/null 2>&1; then
    jq -r '.brief'
  else
    python3 -c 'import json,sys; print(json.load(sys.stdin)["brief"])'
  fi
}

while true; do
  response=$(claim) || { echo "claim failed; retrying in ${backoff_seconds}s" >&2; sleep "$backoff_seconds"; continue; }
  status=${response##*$'\n'}
  payload=${response%$'\n'*}
  case "$status" in
    200)
      prompt="You claimed this ticket from ownership_board. Follow the brief exactly, reporting back with the ownership MCP tools.

$(printf '%s' "$payload" | brief)"
      # shellcheck disable=SC2086
      claude -p "$prompt" ${CLAUDE_ARGS:-} || echo "claude session ended with an error; the lease will expire and the ticket will be reclaimed" >&2
      ;;
    204)
      ;; # Nothing to do; ask again straight away. The wait happened server side.
    *)
      echo "board answered HTTP $status: $payload" >&2
      sleep "$backoff_seconds"
      ;;
  esac
done
