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

Every pass also reads the board's CI sensors (GET /api/v1/manager/ci-flags):
CI jobs queued longer than MANAGER_CI_QUEUE_MINUTES, runs cancelled on a
current head outside the board, failed default-branch or release runs, and
offline runners. They cost provider API calls, not tokens. MANAGER_CI_SENSORS=0
turns them off.

Build-host repair over ssh is opt in: set MANAGER_BUILD_HOST_SSH_HOST,
MANAGER_BUILD_HOST_SSH_USER and MANAGER_BUILD_HOST_SSH_KEY (see
build_host_ssh.py). Without them a session can only read build-job results
through the board and file infrastructure tickets.

Usage:
  OWNERSHIP_URL=https://board.example.com OWNERSHIP_TOKEN=ob_pat_... \\
    python3 agent/manager/board_manager.py

The token must be created in Settings > Agent tokens with "Board manager" on.
Claude Code must have the board's MCP server configured as `ownership` with the
same token (see docs/agents.md). Standard library only.
"""
import json
import os
import re
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

import build_host_ssh

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
# Optional board review (seconds; 0 = off): a session that reads the whole board,
# not only the flags, and acts as the team lead ("Board review" in board-manager.md).
REVIEW_INTERVAL = int(os.environ.get('MANAGER_REVIEW_INTERVAL', '0'))
REVIEW_KEY = '_review'
# CI sensors (on unless "0"): the board reads the repos' CI with its own connection.
CI_SENSORS = os.environ.get('MANAGER_CI_SENSORS', '1') != '0'
CI_QUEUE_MINUTES = int(os.environ.get('MANAGER_CI_QUEUE_MINUTES', '30'))
BUILD_HOST_SCRIPT = Path(__file__).resolve().parent / 'build_host_ssh.py'
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


def session_notes(result_text):
    """What the session wrote before its MANAGER SUMMARY block: job ids, paths and
    housekeeping the runbook keeps out of the owner's summary. Only logged."""
    text = (result_text or '').strip()
    start = text.rfind('MANAGER SUMMARY')
    if start <= 0:
        return ''
    return ' '.join(text[:start].replace('```', ' ').split())[-1500:]


HEADLINE_LIMIT = 100


def session_headline(summary):
    """The summary's "HEADLINE: ..." line (see the runbook's Finish section), at most
    HEADLINE_LIMIT characters, or '' when the session wrote none."""
    for line in (summary or '').splitlines():
        line = line.strip().lstrip('#*->').strip()
        if line[:9].upper() == 'HEADLINE:':
            headline = ' '.join(line[9:].strip(' *_`').split())
            if headline:
                return headline if len(headline) <= HEADLINE_LIMIT else headline[:HEADLINE_LIMIT - 1].rstrip() + '…'
    return ''


def usage_body(cost, model, summary):
    """The POST /manager/usage body: the cost, the model, the summary and its headline."""
    return {'cost_usd': round(cost, 4), 'model': model, 'summary': summary, 'headline': session_headline(summary)}


def activity_targets(flags, host_names):
    """The tickets and the build host a session acts on, for the Team view's lines.
    A flag names a host with a 'host' field, or by mentioning a host's name."""
    ticket_ids = sorted({int(f['ticket_id']) for f in flags if f.get('ticket_id')})
    host = next((f['host'] for f in flags if f.get('host')), '')
    if not host:
        text = ' '.join(str(f.get('detail', '')) + ' ' + str(f.get('fingerprint', '')) for f in flags).lower()
        host = next((name for name in host_names if re.search(r'(?<![\w-])' + re.escape(name.lower()) + r'(?![\w-])', text)), '')
    return ticket_ids, host


def report_activity(state, kind='flag', flags=()):
    """Tells the board the manager started (working) or ended (idle) a session.
    Never fails the loop: the Team view is a convenience, not a gate."""
    try:
        body = {'state': state}
        if state == 'working':
            hosts = [h['name'] for h in api('GET', '/api/v1/team').get('build_hosts', [])] if kind != 'review' else []
            ticket_ids, host = activity_targets(flags, hosts) if kind != 'review' else ([], '')
            body.update(kind=kind, ticket_ids=ticket_ids[:20], host=host,
                        started_at=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()))
        api('POST', '/api/v1/manager/activity', body)
    except Exception as error:  # noqa: BLE001 - an older board answers 404
        log(f'activity report failed: {type(error).__name__}: {error}')


def session_model(flags):
    """The base model, or the stronger one when a flag came back after an earlier session."""
    if any(f['attempts'] > 0 and not repairing(f) for f in flags):
        return RETRY_MODEL
    return MODEL


def flag_line(flag):
    if flag.get('ticket_id'):
        where = f"#{flag['ticket_id']} [{flag['kind']}] {flag.get('title', '')} (stage {flag.get('stage', '?')})"
    elif flag.get('repo_id'):
        # A CI flag about a repository itself: its default branch, releases or runners.
        where = f"repo {flag['repo_id']} {flag.get('title', '')} [{flag['kind']}]"
    else:
        where = f"deployment [{flag['kind']}]"
    return (f"- {where}: {flag['detail']}. Fingerprint {flag['fingerprint']}; "
            f"earlier attempts on it: {flag.get('attempts', 0)}.")


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


def ci_flags():
    """The board's CI sensor flags, or [] when they are off or the call fails: CI
    trouble must never stop the board's own flags from being handled. What the
    board could not read (a refused token, a missing permission) is logged."""
    if not CI_SENSORS:
        return []
    try:
        result = api('GET', f'/api/v1/manager/ci-flags?queue_minutes={CI_QUEUE_MINUTES}')
    except Exception as error:  # noqa: BLE001 - an older board answers 404
        log(f'CI sensors failed: {type(error).__name__}: {error}')
        return []
    for note in result.get('notes') or []:
        log(f'CI sensors: {note}')
    return [f for f in result.get('flags') or [] if isinstance(f, dict) and f.get('kind') and f.get('fingerprint')]


def expire_links():
    """Drops this manager's depends_on links older than the board's limit (6 h), so a
    forgotten link never parks a ticket for good. Never fails the loop."""
    try:
        removed = api('POST', '/api/v1/manager/expire-links', {}).get('removed', 0)
    except Exception as error:  # noqa: BLE001 - an older board answers 404
        log(f'link expiry failed: {type(error).__name__}: {error}')
        return
    if removed:
        log(f'expired {removed} depends_on link(s) older than 6 h')


# What the owner was told, kept so a session never repeats it within OWNER_MEMORY.
OWNER_KEY = '_owner_asks'
OWNER_MEMORY = 24 * 3600
REPEAT_OVERLAP = 0.5
NOTHING_NEW = '- Nothing new.'


def owner_section(summary):
    """(start, end, bullets) of the summary's "### Owner" section, or None."""
    lines = (summary or '').splitlines()
    for start, line in enumerate(lines):
        if line.strip().lower() == '### owner':
            end = start + 1
            while end < len(lines) and not lines[end].startswith('### '):
                end += 1
            return start, end, [l.strip() for l in lines[start + 1:end] if l.strip().startswith('-')]
    return None


def ask_words(text):
    return {w for w in re.findall(r'[a-z0-9]+', text.lower()) if len(w) >= 4}


def is_nothing(bullet):
    return bullet.lstrip('- ').strip().rstrip('.').lower() in ('nothing', 'nothing new', 'none')


def is_repeat(bullet, told):
    """True when bullet says much the same as something already told (word overlap)."""
    words = ask_words(bullet)
    for earlier in told:
        other = ask_words(earlier)
        if words and other and len(words & other) / len(words | other) >= REPEAT_OVERLAP:
            return True
    return False


def recent_asks(state, now):
    return [a['text'] for a in state.get(OWNER_KEY, []) if now - a.get('at', 0) < OWNER_MEMORY]


def dedupe_owner(summary, state, now):
    """Drops owner bullets that repeat what the owner heard in the last 24 h, records the
    new ones in state, and returns the summary to keep."""
    section = owner_section(summary)
    if section is None:
        return summary
    start, end, bullets = section
    told = recent_asks(state, now)
    fresh = [b for b in bullets if not is_nothing(b) and not is_repeat(b, told)]
    kept = [a for a in state.get(OWNER_KEY, []) if now - a.get('at', 0) < OWNER_MEMORY]
    state[OWNER_KEY] = kept + [{'text': b, 'at': now} for b in fresh]
    lines = summary.splitlines()
    return '\n'.join(lines[:start + 1] + (fresh or [NOTHING_NEW]) + lines[end:])


def told_section(told):
    if not told:
        return ''
    return ('\n## Already told the owner (last 24 hours)\n\nDo not repeat these in `### Owner`; '
            'the loop drops repeats.\n\n' + '\n'.join(told) + '\n')


def build_host_section(access):
    """The prompt section that says whether this session may act on the build host."""
    if access is None:
        return ('\n## Build host\n\nBuild-host repair is off: you have no access to the build host. Read build-job '
                'results through the board (get_ticket, ci_status, ci_failed_log) and file an infrastructure ticket, '
                'or block with "Needs the owner:" naming the exact host fault.\n')
    return ('\n## Build host\n\nBuild-host repair is on, as the account `%s`. Run every build-host command through '
            '`python3 %s "<command>"` and nothing else: it logs each command, and it refuses deletes. Never delete; '
            'park or move aside (quarantine) instead, and follow "Repairs".\n') % (access['user'], BUILD_HOST_SCRIPT.as_posix())


def build_prompt(repo_id, flags, budget_left, local_runbook=None, review=False, build_host=None, told=()):
    lines = '\n'.join(flag_line(f) for f in flags) or '- none'
    runbook = COMMAND.read_text()
    if runbook.startswith('---'):
        # Drop the command front matter: a prompt starting with "---" reads as a CLI option.
        runbook = runbook.split('---', 2)[2].lstrip()
    if review:
        prompt = (runbook + f'\n\n## This session\n\nThe board review (every repo; see "Board review"). '
                  f'Budget left today: ${budget_left:.2f}. Flags open right now (their own sessions '
                  f'handle them; use them as context only):\n\n{lines}\n')
    else:
        scope = f'Repo id {repo_id}.' if repo_id else 'Deployment flags (no ticket; from the deployment sensors).'
        prompt = (runbook + f'\n\n## This session\n\n{scope} Budget left today: ${budget_left:.2f}. '
                  f'Flags to handle:\n\n{lines}\n')
    prompt += build_host_section(build_host)
    prompt += told_section(told)
    if local_runbook is None and LOCAL_RUNBOOK:
        local_runbook = Path(LOCAL_RUNBOOK).read_text()
    if local_runbook:
        prompt += '\n## This deployment\n\nRepairs are enabled.\n\n' + local_runbook.strip() + '\n'
    return prompt


def review_due(state, now):
    """True when the board review is on (MANAGER_REVIEW_INTERVAL > 0) and due."""
    return REVIEW_INTERVAL > 0 and now - state.get(REVIEW_KEY, {}).get('last', 0) >= REVIEW_INTERVAL


def run_session(repo_id, flags, budget_left, review=False, told=()):
    """One model session for one repo's flags, or the board review. Returns (cost in USD, model, summary)."""
    model = MODEL if review else session_model(flags)
    prompt = build_prompt(repo_id, flags, budget_left, review=review, build_host=build_host_ssh.build_host_access(), told=told)
    log(f'board review on {model}' if review else f'repo {repo_id}: {len(flags)} flag(s) on {model}')
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
    notes = session_notes(output.get('result'))
    if notes:
        log(f'repo {repo_id}: session notes: {notes}')
    summary = session_summary(output.get('result'))
    log(f'repo {repo_id}: done, ${cost:.2f}: {summary[-400:]}')
    return cost, model, summary


def main():
    if not URL or not TOKEN:
        sys.exit('set OWNERSHIP_URL and OWNERSHIP_TOKEN (a Board manager token)')
    try:
        access = build_host_ssh.build_host_access()
    except build_host_ssh.AccessError as error:
        sys.exit(f'build-host ssh settings: {error}')
    log(f'board manager online: every {INTERVAL}s, {MODEL} then {RETRY_MODEL}, max {MAX_ATTEMPTS} attempts per flag; '
        f'CI sensors {"on" if CI_SENSORS else "off"}; build-host repair {"on" if access else "off"}')
    if access and build_host_ssh.is_admin(access):
        log(f'warning: build-host repair logs in as {access["user"]}; prefer a limited build account')
    while True:
        try:
            status = api('GET', '/api/v1/manager/status')
            if not status['enabled']:
                time.sleep(INTERVAL)
                continue
            expire_links()
            flags = api('GET', '/api/v1/manager/flags')['flags'] + ci_flags() + sensor_flags()
            state, now = load_state(), time.time()
            # Forget flags that cleared, so a new problem on the same ticket starts fresh.
            # Keys starting with "_" are the loop's own bookkeeping (the review timer).
            current = {f['fingerprint'] for f in flags}
            state = {k: v for k, v in state.items() if k in current or k.startswith('_')}
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
                report_activity('working', 'repair' if any(repairing(f) for f in repo_flags) else 'flag', repo_flags)
                try:
                    cost, model, summary = run_session(repo_id, repo_flags, remaining, told=recent_asks(state, now))
                finally:
                    report_activity('idle')
                summary = dedupe_owner(summary, state, time.time())
                save_state(state)
                remaining = api('POST', '/api/v1/manager/usage', usage_body(cost, model, summary))['remaining_usd']
            if review_due(state, now) and remaining > 0:
                state[REVIEW_KEY] = {'last': now}
                save_state(state)
                report_activity('working', 'review')
                try:
                    cost, model, summary = run_session(0, flags, remaining, review=True, told=recent_asks(state, now))
                finally:
                    report_activity('idle')
                summary = dedupe_owner(summary, state, time.time())
                api('POST', '/api/v1/manager/usage', usage_body(cost, model, 'Board review\n' + summary))
            save_state(state)
        except Exception as error:  # the loop must outlive any one bad pass
            log(f'pass failed: {type(error).__name__}: {error}')
        time.sleep(INTERVAL)


if __name__ == '__main__':
    main()
