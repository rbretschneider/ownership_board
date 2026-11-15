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
# Optional deployment runbook (Markdown). When set, every session gets it and may
# repair infrastructure causes itself (see "Repairs" in board-manager.md).
LOCAL_RUNBOOK = os.environ.get('MANAGER_LOCAL_RUNBOOK', '')
# Optional deployment sensors: a command printing {"flags": [...]} as JSON, run every
# pass at no model cost. Each flag needs kind, fingerprint and detail; ticket_id and
# repo_id are optional (a flag without a ticket is about the deployment itself, such
# as an offline CI runner) and get their own session.
SENSORS = os.environ.get('MANAGER_SENSORS', '')
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


def owner_held(flag):
    """A ticket blocked for the owner ("Needs the owner: ...") waits for a person; a
    session on it can only spend money, and might unblock what the owner parked."""
    return flag['kind'] == 'escalated' and flag['detail'].startswith('blocked: ' + OWNER_PREFIX)


OWNER_PREFIX = 'Needs the owner:'
REPAIR_PREFIX = 'Board manager repairing:'


def repairing(flag):
    """A ticket the manager parked while it repairs the cause. It is checked once per
    MANAGER_RETRY_AFTER on the base model until the repair lands, with no attempt cap:
    a long host job can outlast MAX_ATTEMPTS retries."""
    return flag['kind'] == 'escalated' and flag['detail'].startswith('blocked: ' + REPAIR_PREFIX)

# Count-based flags never clear by themselves: build passes, repeated failures and
# repair or review rounds only grow. One session per fingerprint; the fingerprint
# changes with every new pass or round, so a ticket that keeps burning attempts
# gets a fresh look each time instead of one look per cap-sized step.
COUNT_KEYS = (':build:', ':failure:', ':pr_repairs:', ':review_fixes:')


def max_attempts(flag):
    if any(key in flag['fingerprint'] for key in COUNT_KEYS):
        return 1
    return MAX_ATTEMPTS


def due_flags(flags, state, now):
    """Flags that need a session now, with the model each needs."""
    due = []
    for flag in flags:
        if owner_held(flag):
            continue
        entry = state.get(flag['fingerprint'], {})
        attempts = entry.get('attempts', 0)
        if attempts >= max_attempts(flag) and not repairing(flag):
            continue
        if attempts and now - entry.get('last', 0) < RETRY_AFTER:
            continue
        due.append(dict(flag, attempts=attempts))
    return due


def session_summary(result_text):
    """The session's MANAGER SUMMARY block for the owner, or its tail when it wrote none."""
    text = (result_text or '').strip()
    start = text.rfind('MANAGER SUMMARY')
    summary = text[start:] if start != -1 else text[-2000:]
    return summary.replace('```', '').strip()[:8000]


def session_model(flags):
    """The base model, or the stronger one when a flag came back after an earlier session."""
    if any(f['attempts'] > 0 and not repairing(f) for f in flags):
        return RETRY_MODEL
    return MODEL


def flag_line(flag):
    where = (f"#{flag['ticket_id']} [{flag['kind']}] {flag.get('title', '')} (stage {flag.get('stage', '?')})"
             if flag.get('ticket_id') else f"deployment [{flag['kind']}]")
    return (f"- {where}: {flag['detail']}. Fingerprint {flag['fingerprint']}; "
            f"earlier attempts on it: {flag['attempts']}.")


def sensor_flags():
    """The deployment's own flags (MANAGER_SENSORS), or [] when unset or broken: a
    broken sensor must never stop the board's own flags from being handled."""
    if not SENSORS:
        return []
    try:
        result = subprocess.run(SENSORS, shell=True, capture_output=True, text=True, timeout=300)
        flags = json.loads(result.stdout or '{}').get('flags') or []
    except (subprocess.TimeoutExpired, ValueError, AttributeError) as error:
        log(f'sensors failed: {type(error).__name__}: {error}')
        return []
    good = [f for f in flags if isinstance(f, dict) and f.get('kind') and f.get('fingerprint') and f.get('detail')]
    for flag in good:
        flag['repo_id'] = flag.get('repo_id') or 0
        flag['fingerprint'] = 'sensor:' + str(flag['fingerprint'])
    return good


def build_prompt(repo_id, flags, budget_left, local_runbook=None):
    lines = '\n'.join(flag_line(f) for f in flags)
    runbook = COMMAND.read_text()
    if runbook.startswith('---'):
        # Drop the command front matter: a prompt starting with "---" reads as a CLI option.
        runbook = runbook.split('---', 2)[2].lstrip()
    scope = f'Repo id {repo_id}.' if repo_id else 'Deployment flags (no ticket; from the deployment sensors).'
    prompt = (runbook + f'\n\n## This session\n\n{scope} Budget left today: ${budget_left:.2f}. '
              f'Flags to handle:\n\n{lines}\n')
    if local_runbook is None and LOCAL_RUNBOOK:
        local_runbook = Path(LOCAL_RUNBOOK).read_text()
    if local_runbook:
        prompt += '\n## This deployment\n\nRepairs are enabled.\n\n' + local_runbook.strip() + '\n'
    return prompt


def run_session(repo_id, flags, budget_left):
    """One model session for one repo's flags. Returns (cost in USD, model, summary)."""
    model = session_model(flags)
    prompt = build_prompt(repo_id, flags, budget_left)
    log(f'repo {repo_id}: {len(flags)} flag(s) on {model}')
    try:
        result = subprocess.run(['claude', '-p', prompt, '--model', model, '--output-format', 'json', *CLAUDE_ARGS],
                                capture_output=True, text=True, timeout=SESSION_TIMEOUT)
        output = json.loads(result.stdout or '{}')
    except subprocess.TimeoutExpired:
        log(f'repo {repo_id}: session timed out after {SESSION_TIMEOUT}s')
        return 0.0, model, f'Session timed out after {SESSION_TIMEOUT}s.'
    except ValueError:
        log(f'repo {repo_id}: session returned no JSON: {result.stderr[-300:]}')
        return 0.0, model, 'Session failed to start: ' + result.stderr[-300:]
    cost = float(output.get('total_cost_usd') or 0)
    summary = session_summary(output.get('result'))
    log(f'repo {repo_id}: done, ${cost:.2f}: {summary[-400:]}')
    return cost, model, summary


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
            flags = api('GET', '/api/v1/manager/flags')['flags'] + sensor_flags()
            state, now = load_state(), time.time()
            # Forget flags that cleared, so a new problem on the same ticket starts fresh.
            current = {f['fingerprint'] for f in flags}
            state = {k: v for k, v in state.items() if k in current}
            due = due_flags(flags, state, now)
            by_repo = {}
            for flag in due:
                by_repo.setdefault(flag.get('repo_id') or 0, []).append(flag)
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
                cost, model, summary = run_session(repo_id, repo_flags, remaining)
                remaining = api('POST', '/api/v1/manager/usage',
                                {'cost_usd': round(cost, 4), 'model': model, 'summary': summary})['remaining_usd']
            save_state(state)
        except Exception as error:  # the loop must outlive any one bad pass
            log(f'pass failed: {type(error).__name__}: {error}')
        time.sleep(INTERVAL)


if __name__ == '__main__':
    main()
