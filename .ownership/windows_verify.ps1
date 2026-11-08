[CmdletBinding()]
param([ValidateRange(1,2)][int]$MaxJobs = 2)
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run native agent verification with a non-elevated desktop token.' }
foreach ($name in @('CMAKE_BUILD_PARALLEL_LEVEL','CTEST_PARALLEL_LEVEL','OMP_NUM_THREADS')) {
    $current = [Environment]::GetEnvironmentVariable($name)
    if ($current -and $current -notmatch '^[1-9][0-9]*$') { throw "Unbounded $name is not accepted." }
    $limit = $MaxJobs
    if ($current) { $limit = [Math]::Min($limit,[int]$current) }
    [Environment]::SetEnvironmentVariable($name,"$limit",'Process')
}
if ($env:MAKEFLAGS -and $env:MAKEFLAGS -notmatch '^\s*-j[1-9][0-9]*\s*$') { throw 'Set MAKEFLAGS to an explicit -jN limit for this verification.' }
if ($env:MAKEFLAGS) { $MaxJobs = [Math]::Min($MaxJobs,[int]($env:MAKEFLAGS.Trim().Substring(2))) }
$env:MAKEFLAGS = "-j$MaxJobs"
$env:GOMAXPROCS = "$MaxJobs"
$env:GOFLAGS = "-p=$MaxJobs"
$python = Get-Command python.exe -ErrorAction Stop
& $python.Source -c 'import sys; sys.exit(sys.version_info < (3,10))'
if ($LASTEXITCODE) { throw 'Python 3.10+ is required.' }
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('ownership-agent-verify-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixture | Out-Null
try {
    & $python.Source -m venv (Join-Path $fixture 'venv')
    if ($LASTEXITCODE) { throw 'Verification venv creation failed.' }
    $interpreter = Join-Path $fixture 'venv/Scripts/python.exe'
    & $interpreter -m pip install --disable-pip-version-check --require-hashes -r agent/build_host/requirements.txt
    if ($LASTEXITCODE) { throw 'Dependency verification failed.' }
    & $interpreter -m pip check
    if ($LASTEXITCODE) { throw 'Dependency consistency check failed.' }
    $errors = $null; $tokens = $null
    [System.Management.Automation.Language.Parser]::ParseFile((Join-Path $PWD 'agent/build_host/install/build-host.ps1'),[ref]$tokens,[ref]$errors) | Out-Null
    if ($errors.Count) { $errors | Write-Error; throw 'Installer syntax check failed.' }
    & $interpreter -m unittest discover -s agent/build_host/tests -v
    if ($LASTEXITCODE) { throw 'Portable agent suite failed.' }
    # Explicit discovery prevents a green result with every native test skipped.
    $nativeCheck = @'
import os,sys,unittest
sys.path.insert(0,'agent/build_host/tests')
suite=unittest.defaultTestLoader.discover('agent/build_host/tests',pattern='test_windows.py')
result=unittest.TextTestRunner(verbosity=2).run(suite)
if os.name!='nt' or result.testsRun<3 or result.skipped or not result.wasSuccessful():
    raise SystemExit('mandatory Windows native coverage did not pass')
'@
    & $interpreter -c $nativeCheck
    if ($LASTEXITCODE) { throw 'Mandatory Windows native suite failed.' }
} finally {
    # This directory contains only disposable verification fixtures, never user workloads.
    if (Test-Path -LiteralPath $fixture) { Remove-Item -LiteralPath $fixture -Recurse -Force }
}
