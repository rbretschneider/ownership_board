#!/usr/bin/env python3
"""User-level build worker. Python 3.10+ and PyNaCl are required."""
from __future__ import annotations

import argparse
import base64
import contextlib
import ctypes
import hashlib
import ipaddress
import json
import math
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

from nacl.signing import SigningKey

VERSION = '1.0'
API = '/api/v1/build-host'
GIB = 1024 ** 3
BUNDLE_LIMIT = 100 * 1024 ** 2
OS = {'Windows': 'windows', 'Darwin': 'macos', 'Linux': 'linux'}.get(platform.system(), '')


def json_bytes(value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'), allow_nan=False).encode('utf-8')


def signature_message(method, path, timestamp, body):
    return f'{method}\n{path}\n{timestamp}\n{hashlib.sha256(body).hexdigest()}'.encode()


def signed_headers(host_id, key, method, path, body, timestamp=None):
    ts = str(int(time.time()) if timestamp is None else timestamp)
    if type(host_id) is not int or host_id <= 0 or not re.fullmatch('[1-9][0-9]*', ts):
        raise ValueError('invalid signing identity or timestamp')
    return {'X-Build-Host': str(host_id), 'X-Timestamp': ts,
            'X-Signature': base64.b64encode(key.sign(signature_message(method, path, ts, body)).signature).decode()}


def board_origin(value):
    p = urllib.parse.urlsplit(value)
    if p.scheme not in ('http', 'https') or not p.hostname or p.username is not None or p.password is not None or p.query or p.fragment or p.path not in ('', '/') or '?' in value or '#' in value:
        raise ValueError('board must be an HTTP(S) origin without credentials, path, query or fragment')
    # Resolve neither hostnames nor DNS aliases when allowing local plaintext.
    if p.scheme == 'http':
        try:
            local = ipaddress.ip_address(p.hostname).is_loopback
        except ValueError:
            local = p.hostname == 'localhost'
        if not local:
            raise ValueError('HTTPS is required except on loopback')
    _ = p.port  # Validate port syntax.
    return value.rstrip('/')


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


class BoardError(RuntimeError):
    def __init__(self, status, retry_after=0):
        super().__init__(f'board request rejected (HTTP {status})')
        self.status = status
        self.retry_after = retry_after


class BoardClient:
    def __init__(self, origin, host_id=None, key=None):
        self.origin = board_origin(origin)
        self.host_id, self.key = host_id, key
        self.opener = urllib.request.build_opener(NoRedirect())

    def open(self, method, path, body=None, signed=True, timeout=4):
        if not re.fullmatch(r'/api/v1/build-host(?:/[a-z0-9/-]+)?', path):
            raise ValueError('invalid machine API path')
        raw = b'' if body is None else (body if isinstance(body, bytes) else json_bytes(body))
        headers = {'Content-Type': 'application/json'}
        if signed:
            headers.update(signed_headers(self.host_id, self.key, method, path, raw))
        req = urllib.request.Request(self.origin + path, data=raw if method != 'GET' else None, headers=headers, method=method)
        try:
            return self.opener.open(req, timeout=timeout)
        except urllib.error.HTTPError as exc:
            delay = exc.headers.get('Retry-After', '0')
            exc.close()
            raise BoardError(exc.code, min(120, int(delay)) if delay.isdecimal() else 0) from None
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            # Never include request headers or enrollment bodies in errors.
            raise OSError('board connection failed') from None

    def request(self, method, path, body=None, **kwargs):
        with self.open(method, path, body, **kwargs) as response:
            if response.status == 204:
                return None
            raw = response.read(1024 * 1024 + 1)
            if len(raw) > 1024 * 1024:
                raise ValueError('board response too large')
            value = json.loads(raw)
            if not isinstance(value, dict):
                raise ValueError('invalid board response')
            return value


if os.name == 'nt':
    from ctypes import wintypes as wt
    K32 = ctypes.WinDLL('kernel32', use_last_error=True)
    ADV = ctypes.WinDLL('advapi32', use_last_error=True)
    def win_function(dll, name, result, *args):
        f = getattr(dll, name)
        f.restype, f.argtypes = result, list(args)
        return f
    close_handle = win_function(K32, 'CloseHandle', wt.BOOL, wt.HANDLE)
    current_process = win_function(K32, 'GetCurrentProcess', wt.HANDLE)
    open_token = win_function(ADV, 'OpenProcessToken', wt.BOOL, wt.HANDLE, wt.DWORD, ctypes.POINTER(wt.HANDLE))
    token_info = win_function(ADV, 'GetTokenInformation', wt.BOOL, wt.HANDLE, ctypes.c_int, wt.LPVOID, wt.DWORD, ctypes.POINTER(wt.DWORD))
    local_free = win_function(K32, 'LocalFree', wt.HANDLE, wt.HANDLE)

    def token_value(kind):
        handle = wt.HANDLE()
        if not open_token(current_process(), 8, ctypes.byref(handle)):
            raise ctypes.WinError(ctypes.get_last_error())
        try:
            size = wt.DWORD()
            token_info(handle, kind, None, 0, ctypes.byref(size))
            buf = ctypes.create_string_buffer(size.value)
            if not token_info(handle, kind, buf, size, ctypes.byref(size)):
                raise ctypes.WinError(ctypes.get_last_error())
            return buf
        finally:
            close_handle(handle)

    def windows_sid():
        buf = token_value(1)
        sid = ctypes.cast(buf, ctypes.POINTER(ctypes.c_void_p))[0]
        value = wt.LPWSTR()
        convert = win_function(ADV, 'ConvertSidToStringSidW', wt.BOOL, wt.LPVOID, ctypes.POINTER(wt.LPWSTR))
        if not convert(sid, ctypes.byref(value)):
            raise ctypes.WinError(ctypes.get_last_error())
        try:
            return value.value
        finally:
            local_free(value)

    def windows_protect(path, validate=False):
        sid = windows_sid()
        convert = win_function(ADV, 'ConvertStringSecurityDescriptorToSecurityDescriptorW', wt.BOOL, wt.LPCWSTR, wt.DWORD, ctypes.POINTER(wt.LPVOID), ctypes.POINTER(wt.DWORD))
        descriptor = wt.LPVOID()
        if validate:
            get = win_function(ADV, 'GetNamedSecurityInfoW', wt.DWORD, wt.LPCWSTR, ctypes.c_int, wt.DWORD, wt.LPVOID, wt.LPVOID, wt.LPVOID, wt.LPVOID, ctypes.POINTER(wt.LPVOID))
            if get(str(path), 1, 5, None, None, None, None, ctypes.byref(descriptor)):
                raise PermissionError('cannot inspect identity DACL')
            to_text = win_function(ADV, 'ConvertSecurityDescriptorToStringSecurityDescriptorW', wt.BOOL, wt.LPVOID, wt.DWORD, wt.DWORD, ctypes.POINTER(wt.LPWSTR), ctypes.POINTER(wt.DWORD))
            text = wt.LPWSTR()
            try:
                if not to_text(descriptor, 1, 5, ctypes.byref(text), None):
                    raise PermissionError('cannot inspect identity DACL')
                sddl = text.value
                # Require a protected, single-user allow DACL and matching owner.
                owner, dacl = sddl.split('D:', 1)
                aces = re.findall(r'\([^)]*\)', dacl)
                if owner != 'O:' + sid or not dacl.startswith('P') or len(aces) != 1 or aces[0].split(';')[0] != '(A' or not aces[0].endswith(';' + sid + ')'):
                    raise PermissionError('identity must be owned and accessible only by the build user')
            finally:
                if text:
                    local_free(text)
                local_free(descriptor)
        else:
            if not convert(f'O:{sid}D:P(A;;FA;;;{sid})', 1, ctypes.byref(descriptor), None):
                raise ctypes.WinError(ctypes.get_last_error())
            try:
                set_security = win_function(ADV, 'SetFileSecurityW', wt.BOOL, wt.LPCWSTR, wt.DWORD, wt.LPVOID)
                if not set_security(str(path), 0x80000005, descriptor):
                    raise ctypes.WinError(ctypes.get_last_error())
            finally:
                local_free(descriptor)


def is_elevated():
    if os.name != 'nt':
        return os.geteuid() == 0
    return bool(ctypes.cast(token_value(20), ctypes.POINTER(ctypes.c_ulong))[0]) or windows_sid() in ('S-1-5-18', 'S-1-5-19', 'S-1-5-20')


def refuse_elevated():
    if is_elevated():
        raise PermissionError('refusing elevated execution: run as an unprivileged build user; desktop tests need that user session')


def check_private(path, directory=False):
    info = path.lstat()
    if stat.S_ISLNK(info.st_mode) or getattr(info, 'st_file_attributes', 0) & 0x400:
        raise PermissionError('identity paths cannot be links or reparse points')
    if directory != stat.S_ISDIR(info.st_mode) or (not directory and not stat.S_ISREG(info.st_mode)):
        raise PermissionError('invalid identity file type')
    if os.name == 'nt':
        windows_protect(path, validate=True)
    elif info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise PermissionError('identity must be owned and accessible only by the build user')


def private_dir(path):
    path = Path(path).absolute()
    if path.exists() or path.is_symlink():
        check_private(path, directory=True)
    else:
        path.mkdir(mode=0o700, parents=True)
        if os.name == 'nt':
            windows_protect(path)
    return path


def atomic_private(path, raw):
    if path.exists() or path.is_symlink():
        check_private(path)
    temp = path.with_name(path.name + '.' + uuid.uuid4().hex + '.tmp')
    fd = os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        if os.name == 'nt':
            windows_protect(temp)
        with os.fdopen(fd, 'wb') as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temp, path)
    except BaseException:
        # Only remove our own incomplete atomic-write file, never build artifacts.
        temp.unlink(missing_ok=True)
        raise


class FileLock:
    def __init__(self, path):
        self.path, self.file = Path(path), None

    def try_acquire(self):
        if self.file is None:
            if self.path.exists() or self.path.is_symlink():
                check_private(self.path)
            fd = os.open(self.path, os.O_RDWR | os.O_CREAT | getattr(os, 'O_NOFOLLOW', 0), 0o600)
            if os.name == 'nt':
                windows_protect(self.path)
            self.file = os.fdopen(fd, 'r+b', buffering=0)
            if self.path.stat().st_size == 0:
                self.file.write(b'0')
        try:
            if os.name == 'nt':
                import msvcrt
                self.file.seek(0)
                msvcrt.locking(self.file.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(self.file, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return True
        except OSError:
            return False

    def close(self):
        if self.file:
            self.file.close()
            self.file = None

    def __enter__(self):
        if not self.try_acquire():
            self.close()
            raise RuntimeError('another instance is using this identity')
        return self

    def __exit__(self, *args):
        self.close()


class State:
    def __init__(self, root, config=None):
        self.root = private_dir(root)
        self.config = Path(config) if config else self.root / 'config.json'
        if self.config.parent != self.root:
            raise ValueError('configuration must live in the private application directory')
        self.seed = self.root / 'identity.seed'

    def load(self):
        if not self.config.exists():
            return {}
        check_private(self.config)
        return json.loads(self.config.read_bytes())

    def save(self, value):
        atomic_private(self.config, json_bytes(value))

    def create_key(self):
        if self.seed.exists() or self.seed.is_symlink():
            return self.key()
        fd = os.open(self.seed, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        if os.name == 'nt':
            windows_protect(self.seed)
        with os.fdopen(fd, 'wb') as output:
            output.write(os.urandom(32))
            output.flush()
            os.fsync(output.fileno())
        return self.key()

    def key(self):
        check_private(self.seed)
        return SigningKey(self.seed.read_bytes())

    def lock(self):
        return FileLock(self.root / 'identity.lock')


def pair(state, client, code, name, facts, sleep=time.sleep):
    config = state.load()
    key = state.create_key()
    if not config.get('host_id'):
        if config.get('enrollment_started'):
            raise RuntimeError('ambiguous enrollment: archive this identity explicitly and use a fresh enrollment code')
        config['enrollment_started'] = True
        state.save(config)
        reply = client.request('POST', API + '/pair', {'code': code, 'public_key': base64.b64encode(bytes(key.verify_key)).decode(), 'name': clean_text(name, 200) or 'builder', 'facts': facts}, signed=False)
        if type(reply.get('id')) is not int or reply['id'] <= 0:
            raise ValueError('invalid enrollment response; local identity retained')
        config['host_id'] = reply['id']
        state.save(config)
    client.host_id, client.key = config['host_id'], key
    while True:
        status = client.request('GET', API + '/status')
        if status.get('status') == 'approved':
            return
        if status.get('status') != 'pending':
            raise RuntimeError('identity is no longer pending or approved')
        print('Waiting for build host approval.', flush=True)
        sleep(5)


def clean_text(value, limit):
    return ''.join(c for c in str(value) if c.isprintable())[:limit]


def probe(args):
    try:
        result = subprocess.run(args, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=4, check=False)
        return result.stdout[:16384].decode('utf-8', 'replace') if result.returncode == 0 else ''
    except (OSError, subprocess.TimeoutExpired):
        return ''


def volumes():
    home = Path.home()
    paths = [home]
    if OS == 'linux':
        try:
            for line in Path('/proc/self/mountinfo').read_text().splitlines():
                fields = line.split()
                fs = fields[fields.index('-') + 1]
                if fs in ('ext2','ext3','ext4','xfs','btrfs','zfs','overlay','f2fs'):
                    paths.append(Path(re.sub(r'\\([0-7]{3})', lambda m: chr(int(m[1], 8)), fields[4])))
        except (OSError, ValueError):
            pass
    elif OS == 'windows':
        for c in 'ABCDEFGHIJKLMNOPQRSTUVWXYZ':
            path = c + ':\\'
            if K32.GetDriveTypeW(ctypes.c_wchar_p(path)) == 3:
                paths.append(Path(path))
    elif OS == 'macos':
        paths += [Path('/')] + list(Path('/Volumes').glob('*'))
    found = {}
    for path in paths:
        try:
            if not path.is_dir():
                continue
            info = path.stat()
            usage = shutil.disk_usage(path)
            if info.st_dev not in found:
                found[info.st_dev] = (path, usage)
        except OSError:
            continue
    return list(found.values())[:128]


def default_roots():
    choices = []
    for mount, usage in volumes():
        if os.access(mount, os.W_OK | os.X_OK):
            choices.append((usage.total, mount))
    suffix = '.ownership-build-host-' + (windows_sid().replace('-', '_') if os.name == 'nt' else str(os.geteuid()))
    for _,mount in sorted(choices,key=lambda entry:entry[0],reverse=True):
        base = mount/suffix
        try:
            # access() alone cannot establish Windows ACL writability.
            private_dir(base)
        except OSError:
            continue
        return base/'builds',base/'cache'
    raise OSError('no writable local volume; specify --build-root and --cache-root')


def interactive_session():
    if os.name == 'nt':
        session = ctypes.c_ulong()
        if not K32.ProcessIdToSessionId(os.getpid(), ctypes.byref(session)) or session.value == 0:
            return False
        user32 = ctypes.WinDLL('user32', use_last_error=True)
        desktop = win_function(user32, 'OpenInputDesktop', wt.HANDLE, wt.DWORD, wt.BOOL, wt.DWORD)(0, False, 0x0100)
        if not desktop:
            return False
        win_function(user32, 'CloseDesktop', wt.BOOL, wt.HANDLE)(desktop)
        return True
    return bool(os.environ.get('DISPLAY') or os.environ.get('WAYLAND_DISPLAY') or (OS == 'macos' and probe(['stat', '-f', '%Su', '/dev/console']).strip() == os.environ.get('USER')))


class FactsCollector:
    def __init__(self, build_root, probe=probe):
        self.build_root, self.probe = Path(build_root), probe
        self.cached, self.last = None, -math.inf

    def gpu_facts(self):
        gpus = []
        try:
            if OS == 'windows':
                raw = self.probe(['powershell.exe','-NoProfile','-NonInteractive','-Command', 'ConvertTo-Json -Compress -InputObject @(Get-CimInstance Win32_VideoController | Select-Object Name,DriverVersion)'])
                for value in json.loads(raw):
                    name = clean_text(value.get('Name') or '',256)
                    if name:
                        gpus.append(dict(name=name,driver=clean_text(value.get('DriverVersion') or '',256),opengl_version=''))
            elif OS == 'macos':
                raw = self.probe(['system_profiler','SPDisplaysDataType','-json'])
                for value in json.loads(raw).get('SPDisplaysDataType',[]):
                    name = clean_text(value.get('sppci_model') or value.get('_name') or '',256)
                    if name:
                        gpus.append(dict(name=name,driver='',opengl_version=''))
            elif OS == 'linux':
                raw = self.probe(['glxinfo','-B'])
                renderer = re.search(r'^OpenGL renderer string:\s*(.+)$',raw,re.MULTILINE)
                version = re.search(r'^OpenGL version string:\s*(.+)$',raw,re.MULTILINE)
                if renderer:
                    gpus.append(dict(name=clean_text(renderer[1],256),driver='',opengl_version=clean_text(version[1],256) if version else ''))
        except (ValueError,TypeError,KeyError,AttributeError):
            pass
        return gpus[:32]

    def collect(self):
        if self.cached is None or time.monotonic() - self.last > 3600:
            threads = max(1, os.cpu_count() or 1)
            cores, ram, model = 1, 0, platform.processor()
            if OS == 'linux':
                try:
                    cpu = Path('/proc/cpuinfo').read_text()
                    pairs = set()
                    for block in cpu.split('\n\n'):
                        values = dict(line.split(':', 1) for line in block.splitlines() if ':' in line)
                        values = {k.strip():v.strip() for k,v in values.items()}
                        if 'physical id' in values and 'core id' in values:
                            pairs.add((values['physical id'], values['core id']))
                        model = values.get('model name', model)
                    cores = len(pairs) or 1
                    ram = int(re.search(r'MemTotal:\s+(\d+)', Path('/proc/meminfo').read_text())[1]) * 1024
                except (OSError, ValueError, TypeError):
                    pass
            elif OS == 'macos':
                for key, label in [('hw.physicalcpu', 'cores'), ('hw.memsize', 'ram')]:
                    value = self.probe(['sysctl','-n',key]).strip()
                    if value.isdecimal():
                        if label == 'cores':
                            cores = int(value)
                        else:
                            ram = int(value)
                model = self.probe(['sysctl','-n','machdep.cpu.brand_string']).strip()
            elif OS == 'windows':
                raw = self.probe(['powershell.exe','-NoProfile','-NonInteractive','-Command', '$c=Get-CimInstance Win32_Processor; @{cores=($c|Measure-Object NumberOfCores -Sum).Sum;model=($c.Name -join "; ");ram=(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory}|ConvertTo-Json -Compress'])
                try:
                    value = json.loads(raw)
                    cores, ram, model = int(value['cores']), int(value['ram']), value['model']
                except (ValueError, TypeError, KeyError):
                    pass
            tools = {'python': platform.python_version()}
            for tool, args in {'git':['git','--version'], 'dotnet':['dotnet','--version'], 'cmake':['cmake','--version'], 'ninja':['ninja','--version'], 'gcc':['gcc','--version'], 'clang':['clang','--version'], 'docker':['docker','--version'], 'xcode':['xcodebuild','-version'], 'qt':['qmake','-query','QT_VERSION']}.items():
                value = clean_text(self.probe(args).split('\n')[0], 256)
                if value:
                    tools[tool] = value
            arch = platform.machine().lower()
            arch = {'x86_64':'amd64', 'aarch64':'arm64'}.get(arch, arch)
            if not re.fullmatch('[a-z0-9][a-z0-9_-]{0,63}', arch):
                arch = 'unknown'
            self.cached = dict(os=OS, os_version=clean_text(platform.version(),256), arch=arch, hostname=clean_text(socket.gethostname(),255) or 'builder', lan_ip='', cpu_model=clean_text(model,512), cores=max(1,min(cores,threads)), threads=threads, ram_bytes=max(0,ram), disks=[], gpus=self.gpu_facts(), toolchains=tools, interactive_session=interactive_session(), build_user_is_admin=is_elevated(), agent_version=VERSION)
            self.last = time.monotonic()
        facts = dict(self.cached)
        facts['disks'] = []
        for mount, usage in volumes():
            try:
                is_build = mount.stat().st_dev == self.build_root.stat().st_dev
                facts['disks'].append(dict(mount=clean_text(mount,1024), total=usage.total, free=usage.free, is_build_volume=is_build))
            except OSError:
                continue
        return facts


class Cancelled(RuntimeError):
    pass


class Excerpt:
    """Drain everything; keep a bounded head and tail independent of log retention."""
    def __init__(self):
        self.head, self.tail, self.size = bytearray(), bytearray(), 0

    def add(self, raw):
        self.size += len(raw)
        need = max(0, 7000 - len(self.head))
        self.head.extend(raw[:need])
        self.tail.extend(raw[need:])
        if len(self.tail) > 7000:
            del self.tail[:-7000]

    def text(self):
        middle = b'\n[output truncated]\n' if self.size > 14000 else b''
        # Ignore boundary fragments, retaining valid UTF-8 with a strict byte ceiling.
        return self.head.decode('utf-8','ignore') + middle.decode() + self.tail.decode('utf-8','ignore')


def guardian_main(control_fd, status_fd, command):
    """Keep a control pipe outside the workload group; EOF means the owner died."""
    import select
    child = None
    try:
        child = subprocess.Popen(command, stdin=subprocess.DEVNULL, start_new_session=True, close_fds=True)
        while child.poll() is None:
            readable, _, _ = select.select([control_fd], [], [], .1)
            if readable:
                os.read(control_fd, 1)  # Both an explicit byte and EOF cancel.
                break
        code = child.poll()
        # Clean descendants even when the original command exited before them.
        try:
            os.killpg(child.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        deadline = time.monotonic() + 1.5
        while time.monotonic() < deadline:
            child.poll()
            try:
                os.killpg(child.pid, 0)
            except ProcessLookupError:
                break
            time.sleep(.05)
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        child.wait()
        os.write(status_fd, str(child.returncode if code is None else code).encode())
    except BaseException:
        if child:
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        os.write(status_fd, b'-127')
    finally:
        os.close(control_fd)
        os.close(status_fd)


if os.name == 'nt':
    class BasicLimit(ctypes.Structure):
        _fields_ = [('PerProcessUserTimeLimit',ctypes.c_int64),('PerJobUserTimeLimit',ctypes.c_int64),('LimitFlags',wt.DWORD),('MinimumWorkingSetSize',ctypes.c_size_t),('MaximumWorkingSetSize',ctypes.c_size_t),('ActiveProcessLimit',wt.DWORD),('Affinity',ctypes.c_size_t),('PriorityClass',wt.DWORD),('SchedulingClass',wt.DWORD)]
    class IOCounters(ctypes.Structure):
        _fields_ = [(name,ctypes.c_uint64) for name in ('ReadOperationCount','WriteOperationCount','OtherOperationCount','ReadTransferCount','WriteTransferCount','OtherTransferCount')]
    class ExtendedLimit(ctypes.Structure):
        _fields_ = [('BasicLimitInformation',BasicLimit),('IoInfo',IOCounters),('ProcessMemoryLimit',ctypes.c_size_t),('JobMemoryLimit',ctypes.c_size_t),('PeakProcessMemoryUsed',ctypes.c_size_t),('PeakJobMemoryUsed',ctypes.c_size_t)]
    class JobAccounting(ctypes.Structure):
        _fields_ = [(name,ctypes.c_int64) for name in ('TotalUserTime','TotalKernelTime','ThisPeriodTotalUserTime','ThisPeriodTotalKernelTime')] + [(name,wt.DWORD) for name in ('TotalPageFaultCount','TotalProcesses','ActiveProcesses','TotalTerminatedProcesses')]
    class ThreadEntry(ctypes.Structure):
        _fields_ = [('dwSize',wt.DWORD),('cntUsage',wt.DWORD),('th32ThreadID',wt.DWORD),('th32OwnerProcessID',wt.DWORD),('tpBasePri',wt.LONG),('tpDeltaPri',wt.LONG),('dwFlags',wt.DWORD)]

    class WindowsJob:
        def __init__(self):
            create = win_function(K32,'CreateJobObjectW',wt.HANDLE,wt.LPVOID,wt.LPCWSTR)
            self.handle = create(None,None)
            if not self.handle:
                raise ctypes.WinError(ctypes.get_last_error())
            limit = ExtendedLimit()
            limit.BasicLimitInformation.LimitFlags = 0x2000 | 0x20  # Kill on close and priority class.
            limit.BasicLimitInformation.PriorityClass = 0x4000  # BELOW_NORMAL_PRIORITY_CLASS.
            set_info = win_function(K32,'SetInformationJobObject',wt.BOOL,wt.HANDLE,ctypes.c_int,wt.LPVOID,wt.DWORD)
            if not set_info(self.handle,9,ctypes.byref(limit),ctypes.sizeof(limit)):
                self.close()
                raise ctypes.WinError(ctypes.get_last_error())

        def assign(self, process):
            assign = win_function(K32,'AssignProcessToJobObject',wt.BOOL,wt.HANDLE,wt.HANDLE)
            if not assign(self.handle,wt.HANDLE(int(process._handle))):
                raise ctypes.WinError(ctypes.get_last_error())

        def resume(self, process):
            snapshot = win_function(K32,'CreateToolhelp32Snapshot',wt.HANDLE,wt.DWORD,wt.DWORD)(4,0)
            if snapshot == ctypes.c_void_p(-1).value:
                raise ctypes.WinError(ctypes.get_last_error())
            first = win_function(K32,'Thread32First',wt.BOOL,wt.HANDLE,ctypes.POINTER(ThreadEntry))
            next_thread = win_function(K32,'Thread32Next',wt.BOOL,wt.HANDLE,ctypes.POINTER(ThreadEntry))
            open_thread = win_function(K32,'OpenThread',wt.HANDLE,wt.DWORD,wt.BOOL,wt.DWORD)
            resume = win_function(K32,'ResumeThread',wt.DWORD,wt.HANDLE)
            found = False
            try:
                entry = ThreadEntry()
                entry.dwSize = ctypes.sizeof(entry)
                available = first(snapshot,ctypes.byref(entry))
                while available:
                    if entry.th32OwnerProcessID == process.pid:
                        thread = open_thread(2,False,entry.th32ThreadID)
                        if not thread:
                            raise ctypes.WinError(ctypes.get_last_error())
                        try:
                            if resume(thread) == 0xFFFFFFFF:
                                raise ctypes.WinError(ctypes.get_last_error())
                            found = True
                        finally:
                            close_handle(thread)
                    available = next_thread(snapshot,ctypes.byref(entry))
                if not found:
                    raise RuntimeError('suspended workload has no resumable thread')
            finally:
                close_handle(snapshot)

        def active_count(self):
            info = JobAccounting()
            query = win_function(K32,'QueryInformationJobObject',wt.BOOL,wt.HANDLE,ctypes.c_int,wt.LPVOID,wt.DWORD,wt.LPVOID)
            if not query(self.handle,1,ctypes.byref(info),ctypes.sizeof(info),None):
                raise ctypes.WinError(ctypes.get_last_error())
            return info.ActiveProcesses

        def wait_empty(self):
            deadline = time.monotonic()+2
            while self.active_count():
                if time.monotonic() >= deadline:
                    raise RuntimeError('Windows workload did not terminate within its cleanup deadline')
                time.sleep(.02)

        def terminate(self):
            if self.handle:
                terminate = win_function(K32,'TerminateJobObject',wt.BOOL,wt.HANDLE,wt.UINT)
                if not terminate(self.handle,1):
                    raise ctypes.WinError(ctypes.get_last_error())

        def close(self):
            if self.handle:
                close_handle(self.handle)
                self.handle = None


class ProcessTree:
    def __init__(self, cancel_event):
        self.event = cancel_event
        self.guard = threading.RLock()
        self.process, self.control = None, None
        self.job = WindowsJob() if os.name == 'nt' else None

    def cancel(self):
        self.event.set()
        self._terminate()

    def _terminate(self):
        with self.guard:
            if self.job:
                self.job.terminate()
            if self.control is not None:
                os.close(self.control)
                self.control = None

    def run(self, command, cwd, env, log_path=None, timeout=None):
        if self.event.is_set():
            raise Cancelled('assignment stopped')
        status_r = status_w = control_r = None
        with self.guard:
            if self.event.is_set():
                raise Cancelled('assignment stopped')
            if os.name == 'nt':
                self.process = subprocess.Popen(command, cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, creationflags=0x4 | 0x4000, close_fds=True)
                try:
                    self.job.assign(self.process)
                    self.job.resume(self.process)
                except BaseException:
                    self.process.kill()
                    self.process.wait()
                    self.process.stdout.close()
                    raise
            else:
                control_r, self.control = os.pipe()
                status_r, status_w = os.pipe()
                try:
                    self.process = subprocess.Popen([sys.executable,str(Path(__file__).resolve()),'--guardian',str(control_r),str(status_w),'--',*command], cwd=cwd, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, pass_fds=(control_r,status_w), start_new_session=True)
                finally:
                    os.close(control_r)
                    os.close(status_w)
            process = self.process
        excerpt = Excerpt()
        reader_error = []
        def drain():
            output = None
            written = 0
            try:
                if log_path:
                    output = open(log_path, 'xb')
                while True:
                    raw = process.stdout.read(8192)
                    if not raw:
                        break
                    excerpt.add(raw)
                    if output and written < 8*1024*1024:
                        part = raw[:8*1024*1024-written]
                        output.write(part)
                        written += len(part)
                if output and excerpt.size > written:
                    output.write(b'\n[local log truncated at 8 MiB]\n')
            except BaseException as exc:
                reader_error.append(exc)
                self.cancel()
            finally:
                if output:
                    output.close()
                process.stdout.close()
        reader = threading.Thread(target=drain, daemon=True)
        reader.start()
        start = time.monotonic()
        try:
            while process.poll() is None:
                if self.event.is_set() or timeout is not None and time.monotonic()-start > timeout:
                    self.cancel()
                    process.wait(timeout=6)
                    break
                time.sleep(.05)
            if self.job:
                self.job.terminate()  # An early parent exit must not orphan descendants.
                self.job.wait_empty()
            reader.join(6)
            if reader.is_alive():
                self.cancel()
                raise RuntimeError('workload output pipe did not close after termination')
            if reader_error:
                raise OSError('cannot retain workload log') from reader_error[0]
            code = process.returncode
            if status_r is not None:
                raw = os.read(status_r,64)
                code = int(raw) if raw else -127
            if timeout is not None and time.monotonic()-start > timeout:
                raise TimeoutError('command exceeded its time limit')
            return code, excerpt.text()
        finally:
            with self.guard:
                if self.control is not None:
                    os.close(self.control)
                    self.control = None
                self.process = None
            if status_r is not None:
                os.close(status_r)

    def close(self):
        with self.guard:
            self._terminate()
            if self.job:
                self.job.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()


class Assignment:
    def __init__(self, job, journal):
        self.job, self.journal = job, journal
        self.cancel = threading.Event()
        self.tree = None
        self.phase = 'preparing'
        self.done = threading.Event()
        self.payload = None
        self.obsolete = False
        self.failure_kind = ''
        self.thread = None


def poll_assignment(client, assignment):
    reply = client.request('POST', API + '/jobs/' + assignment.job['id'] + '/progress', {'attempt':assignment.job['attempt'], 'phase':assignment.phase, 'detail':''})
    if not isinstance(reply,dict) or type(reply.get('stop')) is not bool or type(reply.get('attempt')) is not int:
        assignment.failure_kind = 'infrastructure'
        assignment.cancel.set()
        if assignment.tree:
            assignment.tree.cancel()
        raise ValueError('malformed assignment-control response')
    if reply.get('stop') is True or reply.get('attempt') != assignment.job['attempt']:
        assignment.obsolete = reply.get('attempt') != assignment.job['attempt']
        assignment.cancel.set()
        if assignment.tree:
            assignment.tree.cancel()


def main(argv=None):
    refuse_elevated()
    return run_cli(argv)


# Guardian entry is private: no board credentials or alternate execution backend.
if __name__ == '__main__' and len(sys.argv)>1 and sys.argv[1] == '--guardian':
    refuse_elevated()
    guardian_main(int(sys.argv[2]), int(sys.argv[3]), sys.argv[5:])
    raise SystemExit(0)


class InfrastructureError(RuntimeError):
    pass


class CommandFailed(RuntimeError):
    pass


def validate_job(job, host_id):
    try:
        valid = (isinstance(job,dict) and re.fullmatch('[a-f0-9]{64}', job['id']) and
                 type(job['attempt']) is int and 1 <= job['attempt'] <= 3 and
                 type(job['host_id']) is int and job['host_id'] == host_id and
                 re.fullmatch('(?:[a-f0-9]{40}|[a-f0-9]{64})',job['sha']) and
                 re.fullmatch('[a-f0-9]{64}',job['bundle_sha256']) and
                 type(job['bundle_bytes']) is int and 0 < job['bundle_bytes'] <= BUNDLE_LIMIT and
                 job['preset'] in ('dotnet','cmake','script') and job['target']['os'] == OS and
                 isinstance(job['target']['required_capabilities'],list) and
                 len(job['target']['required_capabilities']) <= 32 and
                 all(isinstance(c,str) and re.fullmatch('[a-z][a-z0-9_-]{0,31}',c) for c in job['target']['required_capabilities']))
    except (KeyError, TypeError):
        valid = False
    if not valid:
        raise ValueError('malformed or foreign build assignment')
    return job


def completion_bytes(attempt, commands, failure_kind=''):
    def encoded(value):
        # The board normalizes results with Go encoding/json. Match its HTML
        # and JavaScript separator escaping when budgeting and sending reports.
        raw = json_bytes(value)
        for char in ('<', '>', '&', '\u2028', '\u2029'):
            raw = raw.replace(char.encode(), ('\\u%04x' % ord(char)).encode())
        return raw

    commands = [dict(c, log=c['log'].encode()[:16384].decode('utf-8','ignore')) for c in commands]
    if len(commands) > 128:
        raise ValueError('too many commands for one report')
    result = {'commands':commands}
    if failure_kind:
        result['failure_kind'] = failure_kind
    payload = {'attempt':attempt, 'status':'failed' if failure_kind or any(c['exit_code'] != 0 for c in commands) else 'passed', 'result':result}
    if not commands and not failure_kind:
        raise ValueError('no commands executed')
    size = len(encoded(payload))
    log_sizes = [len(encoded(c['log'])) for c in commands]
    # Includes JSON escaping and the completion envelope, leaving protocol headroom.
    # Only log strings change, so their encoded size deltas avoid repeatedly
    # serializing every command in a large report during truncation.
    while size >= 250*1024:
        index = max(range(len(commands)), key=log_sizes.__getitem__)
        command = commands[index]
        data = command['log'].encode()
        if len(data) < 256:
            raise ValueError('result metadata too large')
        command['log'] = data[:len(data)//4].decode('utf-8','ignore') + '\n[report truncated]\n' + data[-len(data)//4:].decode('utf-8','ignore')
        new_size = len(encoded(command['log']))
        size += new_size-log_sizes[index]
        log_sizes[index] = new_size
    return encoded(payload)


def cpu_budget():
    budget = min(os.cpu_count() or 1, 2 if OS == 'windows' else 3)
    if OS == 'linux':
        try:
            quota, period = Path('/sys/fs/cgroup/cpu.max').read_text().split()
            if quota != 'max':
                budget = min(budget,max(1,int(quota)//int(period)))
        except (OSError, ValueError):
            try:
                quota = int(Path('/sys/fs/cgroup/cpu/cpu.cfs_quota_us').read_text())
                period = int(Path('/sys/fs/cgroup/cpu/cpu.cfs_period_us').read_text())
                if quota > 0:
                    budget = min(budget,max(1,quota//period))
            except (OSError, ValueError):
                pass
        if hasattr(os,'sched_getaffinity'):
            budget = min(budget,len(os.sched_getaffinity(0)))
    return max(1,budget)


def resource_limits(config, environ=None):
    env = dict(os.environ if environ is None else environ)
    limit = min(config.get('max_jobs',1),cpu_budget())
    if type(limit) is not int or limit < 1:
        raise ValueError('max-jobs must be a positive integer')
    for name in ('CMAKE_BUILD_PARALLEL_LEVEL','CTEST_PARALLEL_LEVEL','OMP_NUM_THREADS'):
        if name in env:
            if not re.fullmatch('[1-9][0-9]*',env[name]):
                raise ValueError(f'{name} must be a bounded positive integer')
            limit = min(limit,int(env[name]))
    flags = env.get('MAKEFLAGS','')
    if 'jobserver' in flags:
        raise ValueError('inherited MAKEFLAGS jobserver cannot safely be shared; set an explicit -j limit')
    jobs = re.findall(r'(?:^|\s)(?:-?j|--jobs(?:=|\s))\s*([0-9]*)(?=\s|$)',flags)
    if re.search(r'(?:^|\s)(?:-?j|--jobs)(?:\s|$|=)',flags) and not jobs:
        raise ValueError('MAKEFLAGS must specify an explicit jobs limit')
    for value in jobs:
        if not value or int(value) < 1:
            raise ValueError('MAKEFLAGS must specify an explicit jobs limit')
        limit = min(limit,int(value))
    flags = re.sub(r'(?:^|\s)(?:-?j|--jobs(?:=|\s))\s*[0-9]+(?=\s|$)',' ',flags).strip()
    env['MAKEFLAGS'] = (flags + f' -j{limit}').strip()
    for name in ('CMAKE_BUILD_PARALLEL_LEVEL','CTEST_PARALLEL_LEVEL','OMP_NUM_THREADS','DOTNET_PROCESSOR_COUNT'):
        env[name] = str(limit)
    return limit,env


def app_root():
    if OS == 'windows':
        return Path(os.environ['LOCALAPPDATA'])/'OwnershipBuildHost'
    if OS == 'macos':
        return Path.home()/'Library/Application Support/OwnershipBuildHost'
    return Path(os.environ.get('XDG_DATA_HOME',str(Path.home()/'.local/share')))/'ownership-build-host'


class DesktopLock:
    def __init__(self, event, path=None):
        self.event, self.handle, self.file = event, None, None
        if os.name == 'nt':
            create = win_function(K32,'CreateMutexW',wt.HANDLE,wt.LPVOID,wt.BOOL,wt.LPCWSTR)
            self.handle = create(None,False,'Global\\OwnershipBuildHostDesktop-' + windows_sid())
            if not self.handle:
                raise ctypes.WinError(ctypes.get_last_error())
        else:
            root = private_dir(app_root()) if path is None else Path(path).parent
            self.file = FileLock(Path(path) if path else root/'desktop.lock')

    def __enter__(self):
        while not self.event.is_set():
            if self.handle:
                wait = win_function(K32,'WaitForSingleObject',wt.DWORD,wt.HANDLE,wt.DWORD)
                status = wait(self.handle,100)
                if status in (0,0x80):
                    return self
                if status != 0x102:
                    self.close()
                    raise RuntimeError('cannot acquire desktop mutex')
            elif self.file.try_acquire():
                return self
            else:
                self.event.wait(.1)
        self.close()
        raise Cancelled('stopped while waiting for desktop')

    def close(self):
        if self.handle:
            win_function(K32,'ReleaseMutex',wt.BOOL,wt.HANDLE)(self.handle)
            close_handle(self.handle)
            self.handle = None
        if self.file:
            self.file.close()

    def __exit__(self,*args):
        self.close()


class JobRunner:
    def __init__(self, client, config, assignment):
        self.client, self.config, self.assignment = client, config, assignment
        self.job = validate_job(assignment.job,config['host_id'])
        self.limit,self.env = resource_limits(config)
        # Git preparation is entirely local and must not inherit external configuration.
        self.env = {k:v for k,v in self.env.items() if not k.startswith('GIT_')}
        self.env.update(GIT_CONFIG_GLOBAL=os.devnull,GIT_CONFIG_SYSTEM=os.devnull,GIT_CONFIG_NOSYSTEM='1',GIT_TERMINAL_PROMPT='0',GIT_NO_REPLACE_OBJECTS='1',GIT_LFS_SKIP_SMUDGE='1',DOTNET_CLI_UI_LANGUAGE='en-US',VSLANG='1033',DOTNET_CLI_TELEMETRY_OPTOUT='1')
        for name,directory in {'NUGET_PACKAGES':'nuget','NUGET_HTTP_CACHE_PATH':'nuget-http','DOTNET_CLI_HOME':'dotnet','CCACHE_DIR':'ccache','SCCACHE_DIR':'sccache'}.items():
            self.env[name] = str(Path(config['cache_root'])/directory)
        self.commands = []
        self.folder = None
        self.checkout = None

    def check_cancel(self):
        if self.assignment.cancel.is_set():
            raise Cancelled('assignment stopped')

    def command(self,label,args,phase='preparing',timeout=None,infrastructure=False):
        self.check_cancel()
        if len(self.commands) >= 128:
            raise InfrastructureError('command plan exceeds the report limit')
        label = clean_text(label,200).encode()[:200].decode('utf-8','ignore') or 'command'
        self.assignment.phase = phase
        try:
            code,log = self.assignment.tree.run(args,self.checkout,self.env,self.folder/f'{len(self.commands):03d}.log',timeout=timeout)
        except OSError as exc:
            raise InfrastructureError(f'cannot start {label}: required tool or log location unavailable') from exc
        self.commands.append({'label':label,'exit_code':code,'log':log})
        self.check_cancel()
        if code != 0:
            raise (InfrastructureError if infrastructure else CommandFailed)(f'{label} failed (exit {code})')
        return log

    def prepare(self):
        self.check_cancel()
        self.folder = Path(tempfile.mkdtemp(prefix=f"{self.job['id']}-{self.job['attempt']}-",dir=self.config['build_root']))
        if os.name == 'nt':
            windows_protect(self.folder)
        self.checkout = private_dir(self.folder/'checkout')
        bundle = self.folder/'source.bundle'
        count = 0
        digest = hashlib.sha256()
        with self.client.open('GET', API + '/jobs/' + self.job['id'] + '/bundle') as source, bundle.open('xb') as target:
            while True:
                self.check_cancel()
                raw = (source.read1 if hasattr(source,'read1') else source.read)(64*1024)
                if not raw:
                    break
                count += len(raw)
                if count > self.job['bundle_bytes'] or count > BUNDLE_LIMIT:
                    raise InfrastructureError('bundle exceeds declared size')
                digest.update(raw)
                target.write(raw)
        if count != self.job['bundle_bytes'] or digest.hexdigest() != self.job['bundle_sha256']:
            raise InfrastructureError('bundle length or SHA-256 mismatch')
        hooks = private_dir(self.folder/'empty-hooks')
        git = ['git','-c','core.hooksPath='+str(hooks),'-c','protocol.file.allow=always']
        fmt = 'sha256' if len(self.job['sha']) == 64 else 'sha1'
        self.command('initialize checkout',git+['init','--object-format='+fmt],timeout=30,infrastructure=True)
        self.command('verify bundle',git+['bundle','verify',str(bundle)],timeout=60,infrastructure=True)
        self.command('fetch exact commit',git+['fetch','--no-tags',str(bundle),self.job['sha']],timeout=120,infrastructure=True)
        obj = self.command('verify commit type',git+['cat-file','-t',self.job['sha']],timeout=30,infrastructure=True).strip()
        if obj != 'commit':
            raise InfrastructureError('requested object is not a commit')
        self.command('checkout exact commit',git+['-c','advice.detachedHead=false','checkout','--detach',self.job['sha']],timeout=120,infrastructure=True)
        actual = self.command('verify checkout SHA',git+['rev-parse','HEAD'],timeout=30,infrastructure=True).strip()
        if actual != self.job['sha']:
            raise InfrastructureError('checkout SHA mismatch')
        if self.job['preset'] != 'script':
            attrs = self.checkout/'.gitattributes'
            if (self.checkout/'.gitmodules').exists() or attrs.exists() and 'filter=lfs' in attrs.read_text(errors='replace'):
                raise InfrastructureError('external submodule/LFS material requires an explicit verification script')

    def dotnet(self):
        solutions = sorted([*self.checkout.glob('*.sln'),*self.checkout.glob('*.slnx')])
        targets = solutions or sorted(p for p in self.checkout.glob('*.*proj') if p.is_file())
        if len(targets) != 1:
            raise InfrastructureError('missing or ambiguous root .NET solution/project')
        target = str(targets[0])
        limits = [f'-maxcpucount:{self.limit}']
        self.command('dotnet restore',['dotnet','restore',target,*limits],infrastructure=True)
        self.command('dotnet Release build',['dotnet','build',target,'--configuration','Release','--no-restore',*limits],'building')
        log = self.command('dotnet Release tests',['dotnet','test',target,'--configuration','Release','--no-build','--no-restore',*limits,'--',f'RunConfiguration.MaxCpuCount={self.limit}'],'testing')
        if not re.search(r'(?:Total tests:|total:)\s*[1-9][0-9]*',log,re.IGNORECASE):
            raise InfrastructureError('no runnable .NET tests were confirmed by the test summary')

    def cmake(self):
        categories = {}
        for category,tool in [('configure','cmake'),('build','cmake'),('test','ctest')]:
            log = self.command('list '+category+' presets',[tool,'--list-presets='+category] if tool == 'cmake' else [tool,'--list-presets'],timeout=30,infrastructure=True)
            names = re.findall(r'^\s*"([^"\r\n]+)"',log,re.MULTILINE)
            if OS == 'windows':
                names = [n for n in names if n.startswith(('win-','windows'))]
            else:
                names = [n for n in names if n.startswith(OS+'-')]
            if not names or '[output truncated]' in log:
                raise InfrastructureError('missing available '+category+' presets for '+OS)
            categories[category] = sorted(set(names))
        if sum(map(len,categories.values())) + len(self.commands) > 128:
            raise InfrastructureError('too many presets for one report')
        for name in categories['configure']:
            self.command('configure '+name,['cmake','--preset',name],infrastructure=True)
        for name in categories['build']:
            self.command('build '+name,['cmake','--build','--preset',name,'--parallel',str(self.limit),'--config','Release'],'building')
        for name in categories['test']:
            self.command('test '+name,['ctest','--preset',name,'--parallel',str(self.limit),'--build-config','Release','--output-on-failure','--no-tests=error','--repeat','until-pass:3'],'testing')

    def script(self):
        path = self.checkout/'.ownership'/('verify.ps1' if OS == 'windows' else 'verify.sh')
        try:
            path.resolve(strict=True).relative_to(self.checkout.resolve())
        except (OSError,ValueError) as exc:
            raise InfrastructureError('verification script must resolve within the checkout') from exc
        if not path.is_file():
            raise InfrastructureError('verification script is not a regular file')
        if OS == 'windows':
            args = ['powershell.exe','-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',str(path),'-MaxJobs',str(self.limit)]
        else:
            args = ['/bin/sh',str(path),'--max-jobs',str(self.limit)]
        self.command('verify script',args,'building')

    def execute(self):
        failure = ''
        try:
            self.assignment.tree = ProcessTree(self.assignment.cancel)
            self.prepare()
            desktop = OS == 'windows' or 'gl' in self.job['target']['required_capabilities']
            if 'gl' in self.job['target']['required_capabilities'] and not interactive_session():
                raise InfrastructureError('GL workload requires an interactive desktop session')
            with DesktopLock(self.assignment.cancel) if desktop else contextlib.nullcontext():
                getattr(self,self.job['preset'])()
            self.assignment.phase = 'verifying'
            self.check_cancel()
        except Cancelled:
            failure = self.assignment.failure_kind or 'cancelled'
        except CommandFailed:
            pass  # The actual nonzero command is already recorded.
        except Exception as exc:
            failure = 'infrastructure'
            # No synthetic successful command or guessed exit code.
            if self.folder:
                atomic_private(self.folder/'agent-error.txt',clean_text(str(exc),2048).encode())
        finally:
            if self.assignment.tree:
                self.assignment.tree.close()
        if self.assignment.cancel.is_set() and not failure:
            failure = self.assignment.failure_kind or 'cancelled'
        return completion_bytes(self.job['attempt'],self.commands,failure)


def free_bytes(config):
    # Check both roots even if they happen to resolve to the same filesystem.
    return min(shutil.disk_usage(config[name]).free for name in ('build_root','cache_root'))


class FactsService:
    """Slow optional probes never block assignment-control polling."""
    def __init__(self, collector):
        self.collector = collector
        self.value = collector.collect()
        self.next = time.monotonic()+3600
        self.thread = None

    def collect(self):
        if time.monotonic() >= self.next and (self.thread is None or not self.thread.is_alive()):
            self.next = time.monotonic()+3600
            def refresh():
                try:
                    self.value = self.collector.collect()
                except Exception:
                    pass  # Preserve the most recent successful facts.
            self.thread = threading.Thread(target=refresh,daemon=True)
            self.thread.start()
        value = dict(self.value)
        disks = []
        for disk in value['disks']:
            try:
                usage = shutil.disk_usage(disk['mount'])
                disks.append(dict(disk,total=usage.total,free=usage.free))
            except OSError:
                pass
        value['disks'] = disks
        return value


class AgentLoop:
    def __init__(self,state,client,config,facts,clock=time.monotonic):
        self.state,self.client,self.config,self.facts,self.clock = state,client,config,facts,clock
        self.active = {}
        self.journals = private_dir(state.root/'assignments')
        self.status = None
        self.next_health = 0
        self.next_claim = 0
        self.quiet_until = 0
        self.claim_marker = state.root/'claim.json'
        self.claim_pending = False
        if self.claim_marker.exists() or self.claim_marker.is_symlink():
            check_private(self.claim_marker)
            marker = json.loads(self.claim_marker.read_bytes())
            if not isinstance(marker,dict) or type(marker.get('pending')) is not bool:
                raise ValueError('invalid claim recovery marker; preserve it for operator recovery')
            self.claim_pending = marker['pending']
            if self.claim_pending:
                # A fresh full interval also covers reboot/clock changes and repeated
                # restarts, without trusting a persisted wall-clock deadline.
                self.quiet_until = self.clock()+185
        self.failure_since = None
        self.last_health = self.clock()
        self.connected = False
        self.backoff = 1
        self.shutdown = threading.Event()

    def capacity(self):
        limit,_ = resource_limits(self.config)
        approved = self.status.get('max_parallel',1) if self.status else 0
        if type(approved) is not int or not 1 <= approved <= 8:
            raise ValueError('invalid host capacity')
        return min(approved,self.config.get('max_parallel',1),max(1,cpu_budget()//limit))

    def persist(self,assignment,settled=False):
        atomic_private(assignment.journal,json_bytes({'job':assignment.job,'completion':base64.b64encode(assignment.payload).decode() if assignment.payload else '', 'settled':settled}))

    def mark_claim(self,pending):
        atomic_private(self.claim_marker,json_bytes({'pending':pending}))
        self.claim_pending = pending

    def release_claim_barrier(self):
        if self.clock() < self.quiet_until:
            return False
        if self.claim_pending:
            self.mark_claim(False)
        return True

    def cancel_all(self,reason='infrastructure'):
        for assignment in list(self.active.values()):
            assignment.failure_kind = reason
            assignment.cancel.set()
            if assignment.tree:
                assignment.tree.cancel()

    def upload(self,assignment):
        if assignment.obsolete:
            self.persist(assignment,settled=True)
            return
        if not assignment.payload:
            raise RuntimeError('missing persisted completion')
        # A full disk must not turn an upload into an unrecoverable one-shot report.
        self.persist(assignment)
        try:
            self.client.request('POST',API+'/jobs/'+assignment.job['id']+'/result',assignment.payload)
        except BoardError as exc:
            if exc.status not in (403,404,409):
                raise
            # A stale/conflicting response cannot overwrite a new assignment.
        self.persist(assignment,settled=True)

    def recover(self):
        if not self.release_claim_barrier():
            return False
        for path in sorted(self.journals.glob('*.json')):
            check_private(path)
            value = json.loads(path.read_bytes())
            if value['settled']:
                continue
            assignment = Assignment(value['job'],path)
            raw = value.get('completion','')
            if raw:
                assignment.payload = base64.b64decode(raw,validate=True)
            else:
                try:
                    reply = self.client.request('POST',API+'/jobs/'+assignment.job['id']+'/progress',{'attempt':assignment.job['attempt'],'phase':'verifying','detail':'Runner interrupted; previous work will not be resumed.'})
                    assignment.obsolete = reply.get('attempt') != assignment.job['attempt']
                except BoardError as exc:
                    if exc.status not in (403,404,409):
                        raise
                    assignment.obsolete = True
                assignment.payload = completion_bytes(assignment.job['attempt'],[],'infrastructure')
                self.persist(assignment)
            self.upload(assignment)
        return True

    def start(self,job):
        validate_job(job,self.config['host_id'])
        if job['id'] in self.active:
            raise ValueError('duplicate active assignment')
        assignment = Assignment(job,self.journals/(job['id']+'-'+str(job['attempt'])+'-'+uuid.uuid4().hex+'.json'))
        self.persist(assignment)
        def work():
            try:
                assignment.payload = JobRunner(self.client,self.config,assignment).execute()
            except Exception:
                assignment.payload = completion_bytes(job['attempt'],[],'infrastructure')
            finally:
                try:
                    self.persist(assignment)
                except OSError:
                    pass  # The controller must persist successfully before upload.
                finally:
                    assignment.done.set()
        assignment.thread = threading.Thread(target=work,name='build-'+job['id'][:12])
        self.active[job['id']] = assignment
        assignment.thread.start()

    def _tick(self,now):
        if not self.release_claim_barrier():
            return
        if now >= self.next_health or not self.connected:
            # Status applies owner changes; disabled hosts continue health/current work.
            status = self.client.request('GET',API+'/status')
            if status.get('status') != 'approved':
                raise BoardError(401)
            self.status = status
            try:
                free = shutil.disk_usage(self.config['build_root']).free
            except OSError:
                free = 0
            self.client.request('POST',API+'/heartbeat',{'facts':self.facts.collect(),'running_job_ids':sorted(self.active),'free_disk_bytes':free})
            self.connected = True
            self.last_health = self.clock()
            self.next_health = now+60
        for job_id,assignment in list(self.active.items()):
            if assignment.done.is_set():
                self.upload(assignment)
                if assignment.thread:
                    assignment.thread.join()
                del self.active[job_id]
            else:
                try:
                    poll_assignment(self.client,assignment)
                except BoardError as exc:
                    if exc.status not in (400,403,404,409):
                        raise
                    assignment.obsolete = True
                    assignment.cancel.set()
                    if assignment.tree:
                        assignment.tree.cancel()
        if not self.status.get('enabled') or len(self.active) >= self.capacity() or now < self.next_claim or self.shutdown.is_set():
            return
        try:
            if free_bytes(self.config) < self.config.get('reserve_gib',50)*GIB:
                return
        except OSError:
            return
        self.next_claim = now+5
        # Persist before sending: termination anywhere in the request/response
        # window must not allow a restarted worker to extend unknown ownership.
        self.mark_claim(True)
        self.quiet_until = self.clock()+185
        try:
            reply = self.client.request('POST',API+'/claim',{})
        except (OSError,ValueError,BoardError) as exc:
            if isinstance(exc,BoardError) and 400 <= exc.status < 500 and exc.status != 408:
                self.mark_claim(False)
                self.quiet_until = 0
                raise
            # Unknown claim ownership must age out before any health can extend it.
            self.quiet_until = self.clock()+185
            self.connected = False
            self.cancel_all()
            raise
        try:
            if reply is not None:
                self.start(reply['job'])
            # Assignment persistence in start precedes clearing the marker.
            # Idle responses are definitive too; neither needs a restart pause.
            self.mark_claim(False)
            self.quiet_until = 0
        except (KeyError,ValueError,OSError):
            # A claim whose journal cannot be saved is also untracked work.
            # Withhold health so the board can expire that assignment.
            self.quiet_until = self.clock()+185
            self.cancel_all()
            raise

    def enforce_lease_safety(self,now):
        if now-self.last_health >= 120 or self.failure_since is not None and now-self.failure_since >= 120:
            self.cancel_all()

    def tick(self):
        now = self.clock()
        self.enforce_lease_safety(now)
        try:
            self._tick(now)
            if now >= self.quiet_until:
                self.failure_since = None
                self.backoff = 1
            return 2
        except (OSError,ValueError,BoardError) as exc:
            self.connected = False
            if isinstance(exc,BoardError) and exc.status == 401:
                self.cancel_all()
                raise
            if self.failure_since is None:
                self.failure_since = now
            wait = max(self.backoff,exc.retry_after if isinstance(exc,BoardError) else 0)
            self.backoff = min(30,self.backoff*2)
            return min(120,wait)

    def run(self):
        # Previous native containment closes on owner death. Give guardians their
        # bounded cleanup interval before reporting interrupted assignments.
        time.sleep(2)
        while not self.shutdown.is_set() and not self.recover():
            self.shutdown.wait(2)
        watchdog_stop = threading.Event()
        def watch():
            while not watchdog_stop.wait(.2):
                self.enforce_lease_safety(self.clock())
        watchdog = threading.Thread(target=watch,name='lease-safety',daemon=True)
        watchdog.start()
        try:
            while not self.shutdown.is_set():
                self.shutdown.wait(self.tick())
        finally:
            watchdog_stop.set()
            watchdog.join(1)
            self.cancel_all()
            for assignment in list(self.active.values()):
                if assignment.thread:
                    assignment.thread.join(8)
                # Journals survive clean shutdown and any control-plane outage.
                if assignment.payload:
                    self.persist(assignment)


def run_cli(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command',choices=('pair','run'))
    parser.add_argument('--config',type=Path,default=app_root()/'config.json')
    parser.add_argument('--board')
    parser.add_argument('--code')
    parser.add_argument('--name',default=socket.gethostname())
    parser.add_argument('--build-root',type=Path)
    parser.add_argument('--cache-root',type=Path)
    parser.add_argument('--reserve-gib',type=float)
    parser.add_argument('--max-jobs',type=int)
    parser.add_argument('--max-parallel',type=int)
    args = parser.parse_args(argv)
    if not OS:
        raise RuntimeError('unsupported operating system')
    if not shutil.which('git'):
        raise RuntimeError('Git is required; install it for the build user')
    origin = board_origin(args.board) if args.board else None
    config_path = args.config.absolute()
    state = State(config_path.parent,config_path)
    with state.lock():
        config = state.load()
        if origin and config.get('board') and origin != config['board']:
            raise ValueError('existing identity belongs to a different board; archive it explicitly first')
        if args.command == 'pair' and (not origin or not args.code):
            parser.error('pair requires --board and --code')
        if args.command == 'run' and not config.get('host_id'):
            raise RuntimeError('pair and obtain approval before starting run')
        config['board'] = origin or config['board']
        for name in ('build_root','cache_root','reserve_gib','max_jobs','max_parallel'):
            value = getattr(args,name)
            if value is not None:
                config[name] = str(value.absolute()) if isinstance(value,Path) else value
        if not config.get('build_root') or not config.get('cache_root'):
            build,cache = default_roots()
            config.setdefault('build_root',str(build))
            config.setdefault('cache_root',str(cache))
        config.setdefault('reserve_gib',50)
        config.setdefault('max_jobs',1)
        config.setdefault('max_parallel',1)
        if not math.isfinite(config['reserve_gib']) or config['reserve_gib'] < 0 or not 1 <= config['max_parallel'] <= 8:
            raise ValueError('reserve must be nonnegative and max-parallel must be 1..8')
        resource_limits(config)
        for name in ('build_root','cache_root'):
            private_dir(config[name])
        state.save(config)
        client = BoardClient(config['board'],config.get('host_id'),state.key() if config.get('host_id') else None)
        collector = FactsCollector(Path(config['build_root']))
        if args.command == 'pair':
            pair(state,client,args.code,args.name,collector.collect())
            print('Build host approved.',flush=True)
        else:
            loop = AgentLoop(state,client,config,FactsService(collector))
            def stop(signum, frame):
                loop.shutdown.set()
                loop.cancel_all()
            signal.signal(signal.SIGINT,stop)
            signal.signal(signal.SIGTERM,stop)
            loop.run()
    return 0


if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except KeyboardInterrupt:
        raise SystemExit(130)
    except (OSError,RuntimeError,ValueError) as exc:
        print('Build host: '+str(exc),file=sys.stderr)
        raise SystemExit(1)
