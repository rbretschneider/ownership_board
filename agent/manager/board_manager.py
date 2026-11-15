#!/usr/bin/env python3
"""Board manager loop for ownership_board.

Keeps autonomous tickets moving while spending as little as possible. Every
MANAGER_INTERVAL seconds it asks the board, at no model cost, which autonomous
tickets need attention (GET /api/v1/manager/flags): escalated, cycling, over
the build cap, or stalled. It starts a model session only for flags it has not
handled yet, one repo at a time, and only while today's budget
(Settings > Board manager) has money left. Each session follows
agent/claude/board-manager.md and acts through the board's MCP tools, so every
decision lands on the ticket's timeline.

A flag the manager has already handled is left alone for MANAGER_RETRY_AFTER
seconds. If it comes back, the next attempt runs on MANAGER_RETRY_MODEL
(stronger). After MANAGER_MAX_ATTEMPTS attempts the loop stops spending on it;
the session's last action parks the ticket for the owner.

Usage:
  OWNERSHIP_URL=https://board.example.com OWNERSHIP_TOKEN=ob_pat_... \\
    python3 agent/manager/board_manager.py

The token must be created in Settings > Agent tokens with "Board manager" on.
Claude Code must have the board's MCP server configured as `ownership` with the
same token (see docs/agents.md). Standard library only.
"""
import json
import os
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

URL = os.environ.get('OWNERSHIP_URL', '').rstrip('/')
TOKEN = os.environ.get('OWNERSHIP_TOKEN', '')
INTERVAL = int(os.environ.get('MANAGER_INTERVAL', '600'))
RETRY_AFTER = int(os.environ.get('MANAGER_RETRY_AFTER', '3600'))
MAX_ATTEMPTS = int(os.environ.get('MANAGER_MAX_ATTEMPTS', '3'))
MODEL = os.environ.get('MANAGER_MODEL', 'sonnet')
RETRY_MODEL = os.environ.get('MANAGER_RETRY_MODEL', 'opus')
SESSION_TIMEOUT = int(os.environ.get('MANAGER_SESSION_TIMEOUT', '1800'))
STATE = Path(os.environ.get('MANAGER_STATE', str(Path.home() / '.board_manager' / 'state.json')))
COMMAND = Path(__file__).resolve().parent.parent / 'claude' / 'board-manager.md'
CLAUDE_ARGS = os.environ.get('CLAUDE_ARGS', '').split()


def log(message):
    print(time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), message, flush=True)


def api(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    request = urllib.request.Request(URL + path, data=data, method=method, headers={
        'Authorization': 'Bearer ' + TOKEN, 'Content-Type': 'application/json'})
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.load(response)


def load_state():
    try:
        return json.loads(STATE.read_text())
    except (OSError, ValueError):
        return {}


def save_state(state):
    STATE.parent.mkdir(parents=True, exist_ok=True)
    tmp = STATE.with_suffix('.tmp')
    tmp.write_text(json.dumps(state, indent=1))
    tmp.replace(STATE)


def due_flags(flags, state, now):
    """Flags that need a session now, with the model each needs."""
    due = []
    for flag in flags:
        entry = state.get(flag['fingerprint'], {})
        attempts = entry.get('attempts', 0)
        if attempts >= MAX_ATTEMPTS:
            continue
        if attempts and now - entry.get('last', 0) < RETRY_AFTER:
            continue
        due.append(dict(flag, attempts=attempts))
    return due


def run_session(repo_id, flags, budget_left):
    """One model session for one repo's flags. Returns its cost in USD."""
    model = RETRY_MODEL if any(f['attempts'] > 0 for f in flags) else MODEL
    lines = '\n'.join(f"- #{f['ticket_id']} [{f['kind']}] {f['title']} (stage {f['stage']}): {f['detail']}. "
                      f"Fingerprint {f['fingerprint']}; earlier attempts on it: {f['attempts']}." for f in flags)
    prompt = (COMMAND.read_text() + f'\n\n## This session\n\nRepo id {repo_id}. Budget left today: ${budget_left:.2f}. '
              f'Flags to handle:\n\n{lines}\n')
    log(f'repo {repo_id}: {len(flags)} flag(s) on {model}')
    try:
        result = subprocess.run(['claude', '-p', prompt, '--model', model, '--output-format', 'json', *CLAUDE_ARGS],
                                capture_output=True, text=True, timeout=SESSION_TIMEOUT)
        output = json.loads(result.stdout or '{}')
    except subprocess.TimeoutExpired:
        log(f'repo {repo_id}: session timed out after {SESSION_TIMEOUT}s')
        return 0.0
    except ValueError:
        log(f'repo {repo_id}: session returned no JSON: {result.stderr[-300:]}')
        return 0.0
    cost = float(output.get('total_cost_usd') or 0)
    log(f'repo {repo_id}: done, ${cost:.2f}: {(output.get("result") or "")[-400:]}')
    return cost


def main():
    if not URL or not TOKEN:
        sys.exit('set OWNERSHIP_URL and OWNERSHIP_TOKEN (a Board manager token)')
    log(f'board manager online: every {INTERVAL}s, {MODEL} then {RETRY_MODEL}, max {MAX_ATTEMPTS} attempts per flag')
    while True:
        try:
            status = api('GET', '/api/v1/manager/status')
            if not status['enabled']:
                time.sleep(INTERVAL)
                continue
            flags = api('GET', '/api/v1/manager/flags')['flags']
            state, now = load_state(), time.time()
            # Forget flags that cleared, so a new problem on the same ticket starts fresh.
            current = {f['fingerprint'] for f in flags}
            state = {k: v for k, v in state.items() if k in current}
            due = due_flags(flags, state, now)
            by_repo = {}
            for flag in due:
                by_repo.setdefault(flag['repo_id'], []).append(flag)
            remaining = status['remaining_usd']
            for repo_id, repo_flags in sorted(by_repo.items()):
                if remaining <= 0:
                    log(f'daily budget spent; {sum(len(v) for v in by_repo.values())} flag(s) wait for tomorrow')
                    break
                for flag in repo_flags:
                    entry = state.setdefault(flag['fingerprint'], {'attempts': 0})
                    entry['attempts'] += 1
                    entry['last'] = now
                save_state(state)
                cost = run_session(repo_id, repo_flags, remaining)
                remaining = api('POST', '/api/v1/manager/usage', {'cost_usd': round(cost, 4)})['remaining_usd']
            save_state(state)
        except Exception as error:  # the loop must outlive any one bad pass
            log(f'pass failed: {type(error).__name__}: {error}')
        time.sleep(INTERVAL)


if __name__ == '__main__':
    main()
