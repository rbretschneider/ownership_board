#!/usr/bin/env python3
"""The board manager's one way onto a build host (opt in).

Repair sessions run every build-host command through this script instead of
plain ssh:

  python3 agent/manager/build_host_ssh.py '<command>'

It is off unless all three settings are set:

  MANAGER_BUILD_HOST_SSH_HOST  host name or address of the build host
  MANAGER_BUILD_HOST_SSH_USER  the account to log in as; prefer a limited
                               build account over an administrator
  MANAGER_BUILD_HOST_SSH_KEY   path to the private key file

Every command is logged (MANAGER_BUILD_HOST_SSH_LOG, default
~/.board_manager/build_host_ssh.log) before it runs and again with its exit
code, and refused commands are logged too. Commands that delete (rm, del,
Remove-Item, git clean, docker prune, ...) are refused: the manager parks or
quarantines (moves aside) instead. This is a guard against mistakes, not a
sandbox: the account's own rights are the real limit, so give it only what
repairs need. Standard library only.
"""
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

HOST_VAR = 'MANAGER_BUILD_HOST_SSH_HOST'
USER_VAR = 'MANAGER_BUILD_HOST_SSH_USER'
KEY_VAR = 'MANAGER_BUILD_HOST_SSH_KEY'
LOG_VAR = 'MANAGER_BUILD_HOST_SSH_LOG'
DEFAULT_LOG = Path.home() / '.board_manager' / 'build_host_ssh.log'
COMMAND_TIMEOUT = int(os.environ.get('MANAGER_BUILD_HOST_SSH_TIMEOUT', '1800'))

# A host or user must not start with "-" (it would become an ssh option) and
# holds no spaces or shell characters.
SAFE_NAME = re.compile(r'^[A-Za-z0-9_][A-Za-z0-9_.:%@\[\]-]{0,252}$')
ADMIN_USERS = {'root', 'admin', 'administrator'}

# Deleting verbs, as whole words at the start of a command or after a
# separator. The manager never deletes: it parks or moves aside.
_SEP = r'(?:^|[\s;&|(`{]|\$\()'
DELETE_PATTERNS = [re.compile(_SEP + pattern + r'(?=\s|$|[;&|)`}])', re.IGNORECASE) for pattern in (
    r'(?:sudo\s+)?rm', r'rmdir', r'del', r'erase', r'rd', r'ri', r'unlink', r'shred', r'truncate', r'mkfs\S*',
    r'format', r'remove-\w+', r'clear-content', r'format-\w+',
    r'git\s+clean', r'git\s+reset\s+--hard', r'git\s+branch\s+-[dD]', r'git\s+push\s+\S*\s*--delete',
    r'docker\s+(?:rm|rmi)', r'docker\s+(?:system|image|container|volume|builder|network)\s+(?:prune|rm)',
)]
# find's own deleting switches.
DELETE_PATTERNS.append(re.compile(r'\s-delete(?=\s|$)|\s-exec\s+rm\b', re.IGNORECASE))


class AccessError(ValueError):
    """The build-host settings are set but unusable."""


def build_host_access(env=None):
    """The build host as {'host', 'user', 'key'}, or None when repair over ssh is
    off (all three settings empty). Raises AccessError when only some are set,
    the key file is missing, or a name could smuggle an ssh option."""
    env = os.environ if env is None else env
    values = {name: (env.get(var) or '').strip() for name, var in (('host', HOST_VAR), ('user', USER_VAR), ('key', KEY_VAR))}
    if not any(values.values()):
        return None
    missing = [var for name, var in (('host', HOST_VAR), ('user', USER_VAR), ('key', KEY_VAR)) if not values[name]]
    if missing:
        raise AccessError('build-host ssh needs all of %s, %s and %s; missing %s' % (HOST_VAR, USER_VAR, KEY_VAR, ', '.join(missing)))
    for name in ('host', 'user'):
        if not SAFE_NAME.match(values[name]):
            raise AccessError('%s is not a plain host or user name' % (HOST_VAR if name == 'host' else USER_VAR))
    if not Path(values['key']).is_file():
        raise AccessError('%s does not name a readable key file' % KEY_VAR)
    return values


def is_admin(access):
    """True when the account looks like an administrator; a limited build
    account is preferred."""
    return access['user'].lower() in ADMIN_USERS


def refusal(command):
    """Why command is refused, or '' when it may run."""
    if not command.strip():
        return 'empty command'
    for pattern in DELETE_PATTERNS:
        match = pattern.search(command)
        if match:
            return 'deletes (%r); park or move it aside instead' % match.group(0).strip()
    return ''


def ssh_argv(access, command):
    """The ssh command line: key only, never a password prompt, and the host
    key pinned on first use."""
    return ['ssh', '-i', access['key'], '-o', 'BatchMode=yes', '-o', 'IdentitiesOnly=yes',
            '-o', 'StrictHostKeyChecking=accept-new', '-o', 'ConnectTimeout=15',
            '-l', access['user'], '--', access['host'], command]


def log_line(path, record):
    path.parent.mkdir(parents=True, exist_ok=True)
    record = dict(record, at=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()))
    with path.open('a', encoding='utf-8') as handle:
        handle.write(json.dumps(record) + '\n')


def run(command, env=None, runner=subprocess.run):
    """Runs command on the build host and returns its exit code. Every step is
    logged; a refused command or bad settings return 2 without connecting."""
    env = os.environ if env is None else env
    log = Path(env.get(LOG_VAR) or DEFAULT_LOG)
    try:
        access = build_host_access(env)
    except AccessError as error:
        print('build_host_ssh: %s' % error, file=sys.stderr)
        return 2
    if access is None:
        print('build_host_ssh: build-host repair is off (%s is not set); read build-job results through the board '
              'and file an infrastructure ticket instead' % HOST_VAR, file=sys.stderr)
        return 2
    target = {'host': access['host'], 'user': access['user'], 'command': command}
    reason = refusal(command)
    if reason:
        log_line(log, dict(target, event='refused', reason=reason))
        print('build_host_ssh: refused: %s' % reason, file=sys.stderr)
        return 2
    log_line(log, dict(target, event='start'))
    try:
        exit_code = runner(ssh_argv(access, command), timeout=COMMAND_TIMEOUT).returncode
    except subprocess.TimeoutExpired:
        exit_code = 124
    log_line(log, dict(target, event='end', exit=exit_code))
    return exit_code


def main(argv):
    if len(argv) != 2:
        print("usage: build_host_ssh.py '<command>'", file=sys.stderr)
        return 2
    return run(argv[1])


if __name__ == '__main__':
    sys.exit(main(sys.argv))
