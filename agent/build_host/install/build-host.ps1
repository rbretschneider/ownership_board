# Download, inspect, then run as the build user with -Board and -Code.
[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Board,
    [Parameter(Mandatory=$true)][string]$Code,
    [string]$Name = $env:COMPUTERNAME,
    [string]$BuildRoot,
    [string]$CacheRoot,
    [double]$ReserveGiB = 50,
    [ValidateRange(1,2)][int]$MaxJobs = 1,
    [ValidateRange(1,8)][int]$MaxParallel = 1
)
$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) -or $identity.User.Value -in @('S-1-5-18','S-1-5-19','S-1-5-20')) {
    throw 'Run this installer as an unprivileged desktop build user, without Administrator elevation.'
}
$python = $null
foreach ($candidate in @('python.exe','python3.exe','py.exe')) {
    $found = Get-Command $candidate -ErrorAction SilentlyContinue
    if ($found) {
        & $found.Source -c 'import sys; sys.exit(sys.version_info < (3,10))' 2>$null
        if ($LASTEXITCODE -eq 0) { $python = $found.Source; break }
    }
}
if (!$python) { throw 'Install Python 3.10+ for this user, including venv and pip, then rerun.' }
if (!(Get-Command git.exe -ErrorAction SilentlyContinue)) { throw 'Install Git for this user and add it to PATH, then rerun.' }
$application = Join-Path $env:LOCALAPPDATA 'OwnershipBuildHost'
function Protect-UserPath([string]$Path, [bool]$Directory, [bool]$Existing) {
    if ((Get-Item -LiteralPath $Path -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Installation paths cannot be reparse points.' }
    if ($Existing) {
        $acl = Get-Acl -LiteralPath $Path
        if (!$acl.AreAccessRulesProtected -or $acl.GetOwner([Security.Principal.SecurityIdentifier]).Value -ne $identity.User.Value) { throw 'Existing installation must be private and owned by the build user.' }
        foreach ($rule in $acl.GetAccessRules($true,$true,[Security.Principal.SecurityIdentifier])) {
            if ($rule.IdentityReference.Value -ne $identity.User.Value -or $rule.AccessControlType -ne 'Allow') { throw 'Existing installation grants access to another account.' }
        }
    }
    if ($Directory) { $acl = New-Object Security.AccessControl.DirectorySecurity } else { $acl = New-Object Security.AccessControl.FileSecurity }
    $acl.SetOwner($identity.User)
    $acl.SetAccessRuleProtection($true,$false)
    $rule = New-Object Security.AccessControl.FileSystemAccessRule($identity.User,'FullControl','Allow')
    $acl.AddAccessRule($rule)
    Set-Acl -LiteralPath $Path -AclObject $acl
}
$existing = Test-Path -LiteralPath $application
if (!$existing) { New-Item -ItemType Directory -Path $application | Out-Null }
Protect-UserPath $application $true $existing
$task = Get-ScheduledTask -TaskName 'OwnershipBuildHost' -ErrorAction SilentlyContinue
if ($task) { Stop-ScheduledTask -TaskName 'OwnershipBuildHost' }
foreach ($asset in @(@('build_host.py','@@AGENT_BASE64@@'),@('requirements.txt','@@REQUIREMENTS_BASE64@@'))) {
    $path = Join-Path $application $asset[0]
    $exists = Test-Path -LiteralPath $path
    if (!$exists) { [IO.File]::WriteAllBytes($path,[byte[]]@()) }
    Protect-UserPath $path $false $exists
    [IO.File]::WriteAllBytes($path,[Convert]::FromBase64String($asset[1]))
}
$venv = Join-Path $application 'venv'
if ((Test-Path -LiteralPath $venv) -and ((Get-Item -LiteralPath $venv -Force).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Virtual environment cannot be a reparse point.' }
& $python -m venv $venv
if ($LASTEXITCODE) { throw 'Python venv creation failed.' }
$interpreter = Join-Path $venv 'Scripts/python.exe'
& $interpreter -m pip install --disable-pip-version-check --require-hashes -r (Join-Path $application 'requirements.txt')
if ($LASTEXITCODE) { throw 'Hash-locked Python dependency installation failed.' }
$config = Join-Path $application 'config.json'
$agent = Join-Path $application 'build_host.py'
$pairArgs = @($agent,'pair','--config',$config,'--board',$Board,'--code',$Code,'--name',$Name,'--reserve-gib',$ReserveGiB.ToString([Globalization.CultureInfo]::InvariantCulture),'--max-jobs',"$MaxJobs",'--max-parallel',"$MaxParallel")
if ($BuildRoot) { $pairArgs += @('--build-root',$BuildRoot) }
if ($CacheRoot) { $pairArgs += @('--cache-root',$CacheRoot) }
& $interpreter @pairArgs
if ($LASTEXITCODE) { throw 'Pairing did not complete; identity retained and no task registered.' }
$logs = Join-Path $application 'logs'
if (!(Test-Path -LiteralPath $logs)) { New-Item -ItemType Directory -Path $logs | Out-Null }
Protect-UserPath $logs $true $false
function Quote-Literal([string]$Value) { return "'" + $Value.Replace("'","''") + "'" }
$wrapper = Join-Path $application 'run.ps1'
$line = '& ' + (Quote-Literal $interpreter) + ' ' + (Quote-Literal $agent) + ' run --config ' + (Quote-Literal $config) + ' *>> ' + (Quote-Literal (Join-Path $logs 'service.log')) + "`r`nexit `$LASTEXITCODE`r`n"
$wrapperExists = Test-Path -LiteralPath $wrapper
if (!$wrapperExists) { [IO.File]::WriteAllBytes($wrapper,[byte[]]@()) }
Protect-UserPath $wrapper $false $wrapperExists
[IO.File]::WriteAllText($wrapper,$line,(New-Object Text.UTF8Encoding($true)))
$action = New-ScheduledTaskAction -Execute (Join-Path $PSHOME 'powershell.exe') -Argument ('-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $wrapper + '"') -WorkingDirectory $application
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $identity.Name
$taskPrincipal = New-ScheduledTaskPrincipal -UserId $identity.Name -LogonType Interactive -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet -MultipleInstances IgnoreNew -ExecutionTimeLimit ([TimeSpan]::Zero) -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName 'OwnershipBuildHost' -Action $action -Trigger $trigger -Principal $taskPrincipal -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName 'OwnershipBuildHost'
Write-Output "Build worker registered for this user's interactive logon. Logs: $logs"
