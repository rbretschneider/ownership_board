#!/bin/sh
# Run from a downloaded file: sh build-host.sh --board https://board.example --code CODE
set -eu
umask 077
if [ "$(id -u)" -eq 0 ]; then
    echo 'Run this installer as an unprivileged build user, without sudo.' >&2
    exit 1
fi
board=''
code=''
parse_arguments() {
    while [ "$#" -gt 0 ]; do
        [ "$#" -ge 2 ] || { echo 'Every option requires a value.' >&2; exit 1; }
        case "$1" in
            --board) board=$2 ;;
            --code) code=$2 ;;
            --name|--build-root|--cache-root|--reserve-gib|--max-jobs|--max-parallel) ;;
            *) echo 'Unknown installer option.' >&2; exit 1 ;;
        esac
        shift 2
    done
}
parse_arguments "$@"
[ -n "$board" ] && [ -n "$code" ] || { echo '--board and --code are required.' >&2; exit 1; }
python=''
for candidate in python3 python; do
    if command -v "$candidate" >/dev/null 2>&1 && "$candidate" -c 'import sys; sys.exit(sys.version_info < (3,10))'; then
        python=$(command -v "$candidate")
        break
    fi
done
[ -n "$python" ] || { echo 'Install Python 3.10+ with venv support for this user, then rerun.' >&2; exit 1; }
command -v git >/dev/null 2>&1 || { echo 'Install Git for this user, then rerun.' >&2; exit 1; }
system=$(uname -s)
case "$system" in
    Linux) command -v systemctl >/dev/null 2>&1 && command -v loginctl >/dev/null 2>&1 || { echo 'A systemd user manager and loginctl are required.' >&2; exit 1; } ;;
    Darwin) command -v launchctl >/dev/null 2>&1 || { echo 'launchctl is required in a logged-in GUI session.' >&2; exit 1; } ;;
    *) echo 'This installer supports Linux and macOS.' >&2; exit 1 ;;
esac
application=$("$python" - <<'PY'
import os,pathlib,sys
if sys.platform=='darwin':
    path=pathlib.Path.home()/'Library/Application Support/OwnershipBuildHost'
else:
    path=pathlib.Path(os.environ.get('XDG_DATA_HOME',str(pathlib.Path.home()/'.local/share')))/'ownership-build-host'
path=path.absolute()
if any(ord(c)<32 for c in str(path)):
    raise SystemExit('Application path cannot contain control characters.')
if path.exists() or path.is_symlink():
    info=path.lstat()
    if path.is_symlink() or not path.is_dir() or info.st_uid!=os.geteuid() or info.st_mode & 0o077:
        raise SystemExit('Existing application directory must be private and owned by this user.')
else:
    path.mkdir(parents=True,mode=0o700)
print(path)
PY
)
if [ "$system" = Linux ]; then
    systemctl --user stop ownership-build-host.service >/dev/null 2>&1 || :
else
    launchctl bootout "gui/$(id -u)/org.ownership.build-host" >/dev/null 2>&1 || :
fi
"$python" - "$application" <<'PY'
import base64,os,pathlib,stat,sys
root=pathlib.Path(sys.argv[1])
for name,payload in [('build_host.py','@@AGENT_BASE64@@'),('requirements.txt','@@REQUIREMENTS_BASE64@@')]:
    path=root/name
    if path.exists() or path.is_symlink():
        info=path.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=os.geteuid() or info.st_mode & 0o077:
            raise SystemExit('Unsafe existing installation file: '+name)
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_TRUNC|getattr(os,'O_NOFOLLOW',0),0o600)
    with os.fdopen(fd,'wb') as f: f.write(base64.b64decode(payload,validate=True))
venv=root/'venv'
if venv.is_symlink(): raise SystemExit('Virtual environment cannot be a symlink.')
(root/'logs').mkdir(mode=0o700,exist_ok=True)
PY
"$python" -m venv "$application/venv" || { echo 'Python venv support is required; install it for this user.' >&2; exit 1; }
interpreter="$application/venv/bin/python"
"$interpreter" -m pip install --disable-pip-version-check --require-hashes -r "$application/requirements.txt"
"$interpreter" "$application/build_host.py" pair --config "$application/config.json" "$@"
"$interpreter" - "$application" <<'PY'
import os,pathlib,plistlib,sys
root=pathlib.Path(sys.argv[1])
args=[str(root/'venv/bin/python'),str(root/'build_host.py'),'run','--config',str(root/'config.json')]
if sys.platform=='darwin':
    path=pathlib.Path.home()/'Library/LaunchAgents/org.ownership.build-host.plist'
    path.parent.mkdir(parents=True,exist_ok=True)
    value={'Label':'org.ownership.build-host','ProgramArguments':args,'RunAtLoad':True,'KeepAlive':True,'ProcessType':'Background','StandardOutPath':str(root/'logs/service.log'),'StandardErrorPath':str(root/'logs/service-error.log')}
    if path.is_symlink(): raise SystemExit('Service file cannot be a symlink.')
    path.write_bytes(plistlib.dumps(value))
else:
    path=pathlib.Path(os.environ.get('XDG_CONFIG_HOME',str(pathlib.Path.home()/'.config')))/'systemd/user/ownership-build-host.service'
    path.parent.mkdir(parents=True,exist_ok=True)
    def quote(value):
        return '"'+value.replace('\\','\\\\').replace('"','\\"').replace('%','%%').replace('$','$$')+'"'
    command=' '.join(quote(a) for a in args)
    log=str(root/'logs/service.log').replace('%','%%')
    if path.is_symlink(): raise SystemExit('Service file cannot be a symlink.')
    path.write_text('[Unit]\nDescription=Ownership build worker\nAfter=network-online.target\n\n[Service]\nType=simple\nExecStart='+command+'\nRestart=on-failure\nRestartSec=10\nKillMode=control-group\nTimeoutStopSec=10\nNice=10\nUMask=0077\nStandardOutput=append:'+log+'\nStandardError=append:'+log+'\n\n[Install]\nWantedBy=default.target\n')
path.chmod(0o600)
PY
if [ "$system" = Linux ]; then
    systemctl --user daemon-reload
    if ! loginctl --no-ask-password enable-linger "$(id -un)" || [ "$(loginctl show-user "$(id -un)" -p Linger --value)" != yes ]; then
        echo 'Host policy did not enable lingering. Ask the machine administrator to enable lingering for this build user, then rerun.' >&2
        exit 1
    fi
    systemctl --user enable --now ownership-build-host.service
else
    launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/org.ownership.build-host.plist"
    launchctl enable "gui/$(id -u)/org.ownership.build-host"
fi
echo "Build worker registered. Logs: $application/logs"
