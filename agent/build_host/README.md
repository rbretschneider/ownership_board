# Build worker

The worker enrolls a dedicated user account with the server and runs verification
jobs assigned to that machine. Use it only for repositories you trust: a build
has the user's filesystem access, including access to the worker's identity.
This is process management, not isolation for hostile code.

Python 3.10 or newer (with pip and venv), Git and the workload's tools must
already be available to the build user. Installers check prerequisites and never
install system packages or request elevation. Ed25519 signing uses the
hash-locked PyNaCl dependency in a private virtual environment.

Download and inspect `/install/build-host.sh` or `/install/build-host.ps1` from
your server. These anonymous downloads contain the runtime shipped with that
server; they contain no deployment credentials. Then run as the build user:

```sh
sh build-host.sh --board https://board.example --code CODE --name Builder
```

```powershell
.\build-host.ps1 -Board https://board.example -Code CODE -Name Builder
```

Do not use sudo or an Administrator terminal. The process refuses root,
elevated Windows tokens and Windows service identities. Ordinary filtered
Windows desktop accounts are supported. Pairing remains open until an owner
approves the pending host. The code is passed only to the pairing process;
it is not stored in configuration or service definitions. HTTPS is required,
except for HTTP on a loopback address during local testing. Redirects are
rejected and certificate verification stays enabled.

The shell installer accepts `--build-root`, `--cache-root`, `--reserve-gib`,
`--max-jobs` and `--max-parallel`; PowerShell accepts the corresponding
`-BuildRoot`, `-CacheRoot`, `-ReserveGiB`, `-MaxJobs` and `-MaxParallel` parameters.
Settings are persisted for the service. Linux and macOS service registration
preserves the installer's tool search path. `max-jobs` limits each build tool;
`max-parallel` limits concurrent assignments, further restricted by approved
host capacity and the CPU budget. The defaults are one thread and one job.
Linux uses at most three CPU slots; Windows uses at most two. Smaller inherited
MAKEFLAGS, CMake, CTest and OpenMP limits are respected. Unbounded or ambiguous
parallel settings cause an error. Repository scripts must cooperate with these
limits.

Application files are private to the build user:

| Platform | Application directory | Startup |
| --- | --- | --- |
| Windows | `%LOCALAPPDATA%\OwnershipBuildHost` | Interactive scheduled task `OwnershipBuildHost` at user logon |
| Linux | `$XDG_DATA_HOME/ownership-build-host` or `~/.local/share/ownership-build-host` | `ownership-build-host.service` in the systemd user manager |
| macOS | `~/Library/Application Support/OwnershipBuildHost` | `org.ownership.build-host` launch agent at GUI login |

Linux installation enables and checks lingering so the user service can run
without an active login. If machine policy denies this operation, an
administrator must configure lingering before installation can finish. The
installer does not elevate. Windows desktop/GL work requires the build user
to stay logged in; macOS launch agents also start after login. Do not run a
second foreground worker alongside its service: one OS lock protects each
identity.

`config.json` stores the origin, host ID and resource settings. `identity.seed`
is a separate private signing seed. Neither belongs in a repository or support
log. Reinstalling preserves an existing identity and resumes approval waiting.
To upgrade an enrolled worker, re-run the installer with only the board
(`--board`/`-Board`); no new code is needed. Re-runs rewrite only the owner and
access rules of existing files, so an unprivileged user can upgrade.
If an enrollment response is lost before the host ID is saved, automatic
re-enrollment is intentionally refused. Stop the service, archive the application
directory explicitly and obtain a fresh code from the owner. Do not copy a key
from another machine or guess a host ID.

By default, builds and caches use a private user directory on the largest
usable local volume; explicit roots override discovery. New claims pause when
either volume falls below the free-space reserve (50 GiB by default), or a disk
check fails. Health reports continue. No old bundles, checkouts, logs or result
journals are automatically removed. Operators must monitor and clean retained
files themselves while no affected build is active.

Service output is under the application's `logs` directory. Each unique job
folder retains its bundle, numbered command logs and any agent error diagnostic.
Checkouts persist per repository and parallel slot under
`<build root>/runs/slotN_<owner>__<repo>-<hash>/source`, the same layout as the
legacy Windows dispatcher, so build output and a script's dependency trees are
reused instead of rebuilt (a fresh MakerCAD/Qt/OCCT build took about an hour).
Each job fetches its exact commit into the slot, checks it out with `--force`,
removes untracked files but keeps git-ignored build output (`git clean -ffd`,
without `-x`), and verifies HEAD before running anything. A slot checkout that
cannot be updated is renamed to `*.broken-<job folder>` and replaced, never
deleted. Common .NET, vcpkg (`VCPKG_DEFAULT_BINARY_CACHE`, `VCPKG_DOWNLOADS`),
ccache and sccache caches use the physical path of the configured cache root.
Command logs retain at most 8 MiB each and note
truncation; uploaded logs retain bounded head/tail excerpts. Result journals
under `assignments` preserve completion bytes across retries. Restarted workers
report interrupted known assignments rather than rerunning a partial checkout.
A claim is marked in private `claim.json` state before its request is sent. A
lost response or interrupted claim triggers a 185-second quiet recovery interval
so unknown assignments can expire. Restarting with an unresolved marker waits
a fresh full interval before any health, claim or journal-recovery requests;
repeated restarts extend that pause. Do not remove the marker to skip recovery.
Extended connectivity loss terminates local work before reassignment.

Verification modes:

- `dotnet`: one unambiguous root solution (or root project), restore, Release
  build and tests. A test summary must confirm runnable tests.
- `cmake`: one OS-prefixed configure preset (the first whose name contains
  `release`, else the first declared) and the build and test presets of the same
  name, including presets resolved through native includes/conditions. Prefixes
  are `win-`/`windows`, `linux-` or `macos-`. The presets choose the build
  configuration. Declared submodules are updated from their remotes first
  (local `file:` URLs are refused). Configure steps are serialized across
  assignments. CTest retries until pass up to three times; no discovered tests
  is an error.
- `script`: on Windows `.ownership/windows_verify.ps1 -MaxJobs N` (or
  `.ownership/verify.ps1` when that is absent), elsewhere
  `/bin/sh .ownership/verify.sh --max-jobs N`. Shell scripts must obey the POSIX
  shell contract. Scripts must resolve inside the checkout. PowerShell 7
  (`pwsh`) is used when installed, else Windows PowerShell.

On Windows, `.ownership/windows_verify.ps1` also runs after the `dotnet` and
`cmake` steps whenever it exists, as with the legacy dispatcher.

Each build, test and script command is limited to three hours. Desktop test
commands that write no output for 20 minutes are stopped as hung and reported
as failed commands with their actual exit code.

Bundles are length/digest checked before Git sees them. Only the exact commit
is fetched from the local bundle with hooks disabled; LFS content is never
downloaded, so LFS repositories need an explicit verification script. A
mismatch is an infrastructure failure.
Windows commands enter a kill-on-close Job Object while suspended and run below
normal priority. POSIX workloads have a process-group guardian. Stops terminate
ordinary descendants, including descendants whose parent exits early. Deliberate
daemonization is outside this containment model.

Desktop tests are serialized across assignments with an exclusive lock on
`<build root>/desktop.lock`; on Windows that is every `dotnet test`, `ctest` and
script step, while builds run in parallel. A script whose text mentions
`OWNERSHIP_DESKTOP_LOCK` receives that file's path in the variable and runs
without the lock; it must lock byte 0 of the file itself (for example
`[IO.File]::Open(...).Lock(0,1)`) around its UI tests. GL work also requires an
interactive session. Missing optional facts remain unknown and do not imply
working OpenGL.

To run manually, use the installed venv interpreter:

```sh
/path/to/venv/bin/python /path/to/build_host.py run --config /path/to/config.json
```

For uninstall, stop and unregister the service before removing application
files. On Linux use `systemctl --user disable --now ownership-build-host.service`,
remove its unit from `$XDG_CONFIG_HOME/systemd/user` (default `~/.config/systemd/user`),
then run `systemctl --user daemon-reload`. Leave lingering enabled if other user
services depend on it. On macOS use
`launchctl bootout gui/$(id -u)/org.ownership.build-host`, then remove the plist
from `~/Library/LaunchAgents`. On Windows use `Stop-ScheduledTask -TaskName
OwnershipBuildHost` and `Unregister-ScheduledTask -TaskName OwnershipBuildHost`.
The identity, application directory and build/cache roots are retained unless
you explicitly remove them; revoke the host in the server when retiring it.

Development checks use standard-library unittest:

```sh
python -m pip install --require-hashes -r agent/build_host/requirements.txt
python -m unittest discover -s agent/build_host/tests -v
BUILD_HOST_PYTHON="$(command -v python)" go test ./internal/httpapi -run TestBuildHostPythonInterop
```

Windows verification runs `.ownership/windows_verify.ps1 -MaxJobs 2` with a
non-elevated desktop token. It requires native Job Object and installer tests;
it must not pass with all Windows tests skipped. Actual GPU rendering and
reboot/login behavior need a suitably provisioned machine and separate checks.
