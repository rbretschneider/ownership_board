import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from common import host

ROOT=Path(__file__).resolve().parents[1]

def render(kind):
    template=(ROOT/'install'/('build-host.'+kind)).read_text()
    return template.replace('@@AGENT_BASE64@@',base64.b64encode((ROOT/'build_host.py').read_bytes()).decode()).replace('@@REQUIREMENTS_BASE64@@',base64.b64encode((ROOT/'requirements.txt').read_bytes()).decode())

class InstallerTests(unittest.TestCase):
    @unittest.skipIf(os.name=='nt','POSIX shell')
    def test_shell_syntax_and_elevated_refusal_before_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            script=Path(tmp)/'install.sh'
            script.write_text(render('sh'))
            subprocess.run(['sh','-n',str(script)],check=True)
            # Native root refusal is checked only when the current token is root.
            if os.geteuid()==0:
                result=subprocess.run(['sh',str(script),'--board','http://127.0.0.1','--code','public-fixture'],capture_output=True,text=True)
                self.assertNotEqual(result.returncode,0)
                self.assertIn('unprivileged',result.stderr)

    @unittest.skipUnless(host.OS in ('linux','macos'),'native user-service fixture')
    def test_user_service_arguments_paths_and_identity_preservation(self):
        if os.geteuid()==0: self.skipTest('installer requires non-root user')
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            bindir=root/'tools';bindir.mkdir()
            home=root/'home space &';home.mkdir()
            data=root/"data ' % $ & ü \\";data.mkdir()
            calls=root/'calls.jsonl'
            shim=bindir/'python3'
            shim.write_text('#!'+sys.executable+'\n'+'''import json,os,pathlib,subprocess,sys
args=sys.argv[1:]
if args[:2]==['-m','venv']:
    dest=pathlib.Path(args[2]); (dest/'bin').mkdir(parents=True,exist_ok=True)
    if not (dest/'bin/python').exists(): (dest/'bin/python').symlink_to(pathlib.Path(__file__).resolve())
    sys.exit(0)
if args[:2]==['-m','pip'] or len(args)>1 and args[1]=='pair':
    with open(os.environ['FIXTURE_CALLS'],'a') as f: f.write(json.dumps(args)+'\\n')
    sys.exit(0)
os.execv(os.environ['FIXTURE_REAL_PYTHON'],[os.environ['FIXTURE_REAL_PYTHON'],*args])
''')
            shim.chmod(0o755)
            for name in ('systemctl','loginctl','launchctl'):
                file=bindir/name
                file.write_text('#!/bin/sh\ncase "$*" in *show-user*) printf "yes\\n";; esac\nexit 0\n');file.chmod(0o755)
            script=root/'install.sh';script.write_text(render('sh'))
            env=dict(os.environ,HOME=str(home),XDG_DATA_HOME=str(data),XDG_CONFIG_HOME=str(home/'.config'),PATH=str(bindir)+os.pathsep+os.environ['PATH'],FIXTURE_CALLS=str(calls),FIXTURE_REAL_PYTHON=sys.executable)
            command=['sh',str(script),'--board','http://127.0.0.1:8080','--code','public-fixture ;$&','--name',"Builder ü '$&",'--max-jobs','1']
            result=subprocess.run(command,env=env,capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            app=data/'ownership-build-host' if host.OS=='linux' else home/'Library/Application Support/OwnershipBuildHost'
            marker=app/'identity.seed';marker.write_bytes(b'identity fixture')
            result=subprocess.run(command,env=env,capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            self.assertEqual(marker.read_bytes(),b'identity fixture')
            self.assertEqual((app/'build_host.py').read_bytes(),(ROOT/'build_host.py').read_bytes())
            # An upgrade re-run keeps the enrolled identity and needs no code.
            upgrade=command[:command.index('--code')]+command[command.index('--code')+2:]
            result=subprocess.run(upgrade,env=env,capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            invocations=[json.loads(line) for line in calls.read_text().splitlines()]
            pairs=[args for args in invocations if len(args)>1 and args[1]=='pair']
            self.assertEqual(len(pairs),3)
            self.assertEqual(pairs[0][pairs[0].index('--code')+1],'public-fixture ;$&')
            self.assertNotIn('--code',pairs[2])
            if host.OS=='linux':
                unit=(home/'.config/systemd/user/ownership-build-host.service').read_text()
                self.assertIn('%%',unit)
                self.assertIn('$$',unit)
                self.assertIn('run',unit)
                self.assertIn('Environment="PATH=',unit)
                self.assertNotIn('public-fixture',unit)
                if shutil.which('systemd-analyze'):
                    runtime=root/'runtime';runtime.mkdir(mode=0o700)
                    verifyenv=dict(env,XDG_RUNTIME_DIR=str(runtime))
                    result=subprocess.run(['systemd-analyze','--user','verify',str(home/'.config/systemd/user/ownership-build-host.service')],env=verifyenv,capture_output=True,text=True)
                    self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            else:
                import plistlib
                plist=plistlib.loads((home/'Library/LaunchAgents/org.ownership.build-host.plist').read_bytes())
                self.assertEqual(plist['ProgramArguments'][-1],str(app/'config.json'))
                self.assertEqual(plist['EnvironmentVariables']['PATH'],env['PATH'])
                self.assertNotIn('public-fixture',json.dumps(plist))

    @unittest.skipUnless(os.name=='nt','PowerShell parser and isolated task fixture')
    def test_powershell_parser_and_argument_contract(self):
        with tempfile.TemporaryDirectory() as tmp:
            script=Path(tmp)/'install.ps1';script.write_text(render('ps1'),encoding='utf-8')
            code='$e=$null; $t=$null; [System.Management.Automation.Language.Parser]::ParseFile($args[0],[ref]$t,[ref]$e)|Out-Null; if($e.Count){$e|Write-Error;exit 1}'
            # Use a file so PowerShell receives native arguments without command-string interpolation.
            parser=Path(tmp)/'parse.ps1';parser.write_text(code)
            subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-File',str(parser),str(script)],check=True)

    @unittest.skipUnless(os.name=='nt','Windows installer fixture')
    def test_windows_installation_preserves_identity_and_uses_interactive_limited_task(self):
        if host.is_elevated(): self.skipTest('installer requires a filtered desktop token')
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            tools=root/'tools';tools.mkdir()
            local=root/"local ' & ü";local.mkdir()
            script=root/'install.ps1';script.write_text(render('ps1'),encoding='utf-8-sig')
            harness=root/'fixture.ps1'
            harness.write_text(r'''param([string]$Installer,[string]$Root)
$ErrorActionPreference='Stop'
$stub = @'
using System;
using System.IO;
using System.Text;
using System.Reflection;
public class PythonFixture {
 public static int Main(string[] args) {
  if(args.Length>2 && args[0]=="-m" && args[1]=="venv") {
   string dir=Path.Combine(args[2],"Scripts");Directory.CreateDirectory(dir);
   File.Copy(Assembly.GetExecutingAssembly().Location,Path.Combine(dir,"python.exe"),true);
  } else if(args.Length>1 && (args[1]=="pip" || args[1]=="pair")) {
   File.AppendAllText(Environment.GetEnvironmentVariable("FIXTURE_CALLS"),Convert.ToBase64String(Encoding.UTF8.GetBytes(String.Join("\0",args)))+"\n");
  }
  return 0;
 }
}
'@
Add-Type -TypeDefinition $stub -OutputAssembly (Join-Path $Root 'tools/python.exe') -OutputType ConsoleApplication
function Get-ScheduledTask { param($TaskName,$ErrorAction) return $null }
function Stop-ScheduledTask { throw 'Fixture has no existing task' }
function New-ScheduledTaskAction { param($Execute,$Argument,$WorkingDirectory) return [pscustomobject]@{Execute=$Execute;Argument=$Argument;WorkingDirectory=$WorkingDirectory} }
function New-ScheduledTaskTrigger { param([switch]$AtLogOn,$User) return [pscustomobject]@{User=$User;AtLogOn=$AtLogOn.IsPresent} }
function New-ScheduledTaskPrincipal { param($UserId,$LogonType,$RunLevel) return [pscustomobject]@{UserId=$UserId;LogonType=$LogonType;RunLevel=$RunLevel} }
function New-ScheduledTaskSettingsSet { param($MultipleInstances,$ExecutionTimeLimit,[switch]$StartWhenAvailable,$RestartCount,$RestartInterval,[switch]$AllowStartIfOnBatteries,[switch]$DontStopIfGoingOnBatteries) return [pscustomobject]@{MultipleInstances=$MultipleInstances;ExecutionTimeLimit=$ExecutionTimeLimit.TotalSeconds} }
function Register-ScheduledTask { param($TaskName,$Action,$Trigger,$Principal,$Settings,[switch]$Force) @{Action=$Action;Trigger=$Trigger;Principal=$Principal;Settings=$Settings}|ConvertTo-Json -Depth 5|Set-Content -LiteralPath $env:FIXTURE_TASK -Encoding UTF8 }
function Start-ScheduledTask { param($TaskName) }
& $Installer -Board 'http://127.0.0.1:8080' -Code 'public-fixture ;$&' -Name 'Builder ü & space'
if (!$?) { throw 'first installation failed' }
$seed=Join-Path $env:LOCALAPPDATA 'OwnershipBuildHost/identity.seed'
[IO.File]::WriteAllText($seed,'retained identity fixture')
& $Installer -Board 'http://127.0.0.1:8080' -Code 'public-fixture ;$&'
if (!$?) { throw 'second installation failed' }
& $Installer -Board 'http://127.0.0.1:8080'
if (!$?) { throw 'upgrade re-run without an enrollment code failed' }
''',encoding='utf-8-sig')
            calls=root/'calls.txt';task=root/'task.json'
            env=dict(os.environ,LOCALAPPDATA=str(local),PATH=str(tools)+os.pathsep+os.environ['PATH'],FIXTURE_CALLS=str(calls),FIXTURE_TASK=str(task))
            result=subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',str(harness),str(script),str(root)],env=env,capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stdout+result.stderr)
            app=local/'OwnershipBuildHost'
            self.assertEqual((app/'identity.seed').read_text(),'retained identity fixture')
            self.assertEqual((app/'build_host.py').read_bytes(),(ROOT/'build_host.py').read_bytes())
            entries=[base64.b64decode(line).decode().split('\0') for line in calls.read_text().splitlines()]
            pairs=[args for args in entries if len(args)>1 and args[1]=='pair']
            self.assertEqual(len(pairs),3)
            self.assertEqual(pairs[0][pairs[0].index('--code')+1],'public-fixture ;$&')
            self.assertNotIn('--code',pairs[2])
            # Re-runs rewrite owner and DACL only, and the result stays private.
            for path,directory in ((app,True),(app/'build_host.py',False),(app/'run.ps1',False)):
                host.check_private(path,directory=directory)
            saved=json.loads(task.read_text(encoding='utf-8-sig'))
            self.assertTrue(Path(saved['Action']['Execute']).is_file())
            self.assertEqual(Path(saved['Action']['Execute']).name.lower(),'powershell.exe')
            self.assertEqual(saved['Principal']['RunLevel'],'Limited')
            self.assertEqual(saved['Principal']['LogonType'],'Interactive')
            self.assertEqual(saved['Settings']['MultipleInstances'],'IgnoreNew')
            self.assertEqual(saved['Settings']['ExecutionTimeLimit'],0)
            self.assertNotIn('public-fixture',json.dumps(saved)+(app/'run.ps1').read_text(encoding='utf-8-sig'))
