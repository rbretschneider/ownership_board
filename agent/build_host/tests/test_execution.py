import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from common import host

class BundleBoard:
    def __init__(self, raw):
        self.raw = raw
    def open(self, *args, **kwargs):
        return contextlib.closing(io.BytesIO(self.raw))


class SourceRepo:
    """A real Git repository whose commits are shipped to the runner as bundles."""
    def __init__(self, root):
        self.root = root
        self.path = root/'source'
        self.path.mkdir()
        self.git('init')
        self.git('config','user.name','Fixture')
        self.git('config','user.email','fixture@example.invalid')
        self.count = 0

    def git(self, *args):
        return subprocess.check_output(['git','-C',str(self.path),*args], stderr=subprocess.STDOUT).decode().strip()

    def commit(self, files, preset='script'):
        for name, text in files.items():
            (self.path/name).parent.mkdir(parents=True, exist_ok=True)
            (self.path/name).write_text(text)
        self.git('add','.')
        self.git('commit','-m','Fixture')
        self.count += 1
        sha = self.git('rev-parse','HEAD')
        bundle = self.root/f'fixture{self.count}.bundle'
        self.git('bundle','create',str(bundle),'HEAD')
        raw = bundle.read_bytes()
        job = {'id':format(self.count,'x')*64,'attempt':1,'host_id':7,'repo':'acme/app','sha':sha,'bundle_bytes':len(raw),'bundle_sha256':hashlib.sha256(raw).hexdigest(),'preset':preset,'target':{'os':host.OS,'required_capabilities':[]}}
        return raw, job


def scripts(sh='printf "verified\\n"\n', ps1='param([int]$MaxJobs)\nWrite-Output "verified $MaxJobs"\n'):
    return {'.ownership/verify.sh':sh,'.ownership/verify.ps1':ps1}


def make_bundle(root, script='printf "verified\\n"\n'):
    return SourceRepo(root).commit(scripts(sh=script))


class ExecutionTests(unittest.TestCase):
    def runner(self, root, raw, job):
        config = {'build_root':str(root/'builds'),'cache_root':str(root/'cache'),'host_id':7,'max_jobs':1}
        host.private_dir(root/'builds')
        host.private_dir(root/'cache')
        return host.JobRunner(BundleBoard(raw), config, host.Assignment(job,None))

    def test_real_bundle_exact_checkout_script_and_result(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            raw, job = make_bundle(root)
            runner = self.runner(root,raw,job)
            payload = json.loads(runner.execute())
            self.assertEqual(payload['status'],'passed', payload)
            self.assertEqual(subprocess.check_output(['git','-C',str(runner.checkout),'rev-parse','HEAD']).decode().strip(), job['sha'])
            self.assertTrue((runner.folder/'source.bundle').exists())
            self.assertIn('verified',payload['result']['commands'][-1]['log'])
            self.assertEqual(payload['result']['commands'][-1]['exit_code'],0)

    def test_corrupt_or_wrong_sha_never_runs_script(self):
        for corrupt in ('digest','sha','length'):
            with self.subTest(corrupt=corrupt), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                raw, job = make_bundle(root)
                if corrupt == 'digest':
                    job['bundle_sha256'] = '0'*64
                elif corrupt == 'sha':
                    job['sha'] = '0'*40
                else:
                    job['bundle_bytes'] += 1
                payload = json.loads(self.runner(root,raw,job).execute())
                self.assertEqual(payload['status'],'failed')
                self.assertEqual(payload['result']['failure_kind'],'infrastructure')
                self.assertFalse(any(c['label']=='verify script' for c in payload['result']['commands']))

    def test_failed_script_records_actual_exit(self):
        if os.name == 'nt':
            self.skipTest('POSIX fixture')
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            raw,job = make_bundle(root, 'exit 23\n')
            payload = json.loads(self.runner(root,raw,job).execute())
            self.assertEqual(payload['status'],'failed')
            self.assertEqual(payload['result']['commands'][-1]['exit_code'],23)
            self.assertNotIn('failure_kind',payload['result'])

    def test_validation_rejects_unassigned_and_path_metadata(self):
        for patch in ({'id':'../escape'},{'attempt':0},{'host_id':8},{'sha':'HEAD'},{'target':{'os':'other'}},{'bundle_bytes':0},{'preset':'sh'},{'repo':''},{'repo':7}):
            with self.subTest(patch=patch), tempfile.TemporaryDirectory() as tmp:
                raw,job = make_bundle(Path(tmp))
                job.update(patch)
                with self.assertRaises(ValueError):
                    host.validate_job(job,7)

    def test_report_json_budget_including_escape_overhead(self):
        commands=[{'label':str(i),'exit_code':0,'log':'🌍\\\"\x00'*4000} for i in range(128)]
        raw=host.completion_bytes(1,commands)
        self.assertLess(len(raw),256*1024)
        payload=json.loads(raw)
        self.assertEqual(len(payload['result']['commands']),128)
        self.assertTrue(all(len(c['log'].encode())<=16384 for c in payload['result']['commands']))

    def test_report_budget_includes_server_html_and_separator_escaping(self):
        for text in ('<', '>', '&', '\u2028', '\u2029'):
            with self.subTest(text=text):
                commands=[{'label':str(i),'exit_code':0,'log':text*14000} for i in range(16)]
                raw=host.completion_bytes(1,commands)
                result=json.loads(raw)['result']
                # Go encoding/json escapes HTML and JavaScript line separators.
                normalized=json.dumps(result,ensure_ascii=False,separators=(',',':'))
                for char in ('<', '>', '&', '\u2028', '\u2029'):
                    normalized=normalized.replace(char,'\\u%04x'%ord(char))
                self.assertLess(len(normalized.encode()),256*1024)
                self.assertLess(len(raw),256*1024)
                self.assertEqual(len(result['commands']),16)

    def recording_runner(self, root, presets=None):
        """A runner whose commands are recorded instead of executed; presets maps category to listed names."""
        prefix='win-' if host.OS=='windows' else host.OS+'-'
        presets=presets or {c:[prefix+'debug',prefix+'release'] for c in ('configure','build','test')}
        raw,job=make_bundle(root)
        runner=self.runner(root,raw,job)
        runner.checkout=root/'source'
        runner.git=['git']
        commands=[]
        def command(label,args,phase='preparing',**kw):
            commands.append((label,args,phase,kw))
            for category in ('configure','build'):
                if '--list-presets='+category in args:
                    return 'Available presets:\n'+''.join(f'  "{n}"  - {n}\n' for n in presets[category])
            if '--list-presets' in args:
                return 'Available presets:\n'+''.join(f'  "{n}"\n' for n in presets['test'])
            return 'Total tests: 1\n' if label == 'dotnet Release tests' else ''
        runner.command=command
        return runner,commands,prefix

    def test_cmake_verifies_one_release_preset_like_the_legacy_dispatcher(self):
        with tempfile.TemporaryDirectory() as tmp:
            runner,commands,prefix=self.recording_runner(Path(tmp))
            runner.cmake()
            work=[(label,args,phase,kw) for label,args,phase,kw in commands if not label.startswith('list ')]
            self.assertEqual([label for label,*_ in work],['configure '+prefix+'release','build '+prefix+'release','test '+prefix+'release'])
            self.assertEqual([phase for _,_,phase,_ in work],['preparing','building','testing'])
            configure,build,test=(args for _,args,_,_ in work)
            self.assertEqual(configure,['cmake','--preset',prefix+'release'])
            # The preset's own configuration decides; the agent forces none.
            self.assertNotIn('--config',build)
            self.assertNotIn('--build-config',test)
            for flag in ('--output-on-failure','until-pass:3','--no-tests=error'):
                self.assertIn(flag,test)
            self.assertEqual(work[2][3]['silence'],host.DESKTOP_SILENCE_LIMIT if host.OS=='windows' else None)
            self.assertEqual(work[2][3]['timeout'],host.COMMAND_TIMEOUT)

    def test_cmake_without_release_uses_first_declared_preset_and_needs_matching_build_and_test(self):
        with tempfile.TemporaryDirectory() as tmp:
            prefix='win-' if host.OS=='windows' else host.OS+'-'
            names=[prefix+'zeta',prefix+'alpha']
            runner,commands,_=self.recording_runner(Path(tmp),{'configure':names,'build':names,'test':names})
            runner.cmake()
            self.assertIn('configure '+prefix+'zeta',[c[0] for c in commands])
        with tempfile.TemporaryDirectory() as tmp:
            runner,_,_=self.recording_runner(Path(tmp),{'configure':names,'build':[prefix+'alpha'],'test':names})
            with self.assertRaisesRegex(host.InfrastructureError,'missing build preset'):
                runner.cmake()

    def test_cmake_updates_declared_submodules_from_remotes_only(self):
        with tempfile.TemporaryDirectory() as tmp:
            runner,commands,_=self.recording_runner(Path(tmp))
            (runner.checkout/'.gitmodules').write_text('[submodule "external/vcpkg"]\n')
            runner.cmake()
            labels=[c[0] for c in commands]
            update=commands[labels.index('update submodules')][1]
            self.assertLess(labels.index('update submodules'),next(i for i,l in enumerate(labels) if l.startswith('configure ')))
            self.assertEqual(update[-4:],['submodule','update','--init','--recursive'])
            self.assertIn('protocol.file.allow=never',update)

    def test_dotnet_arguments_and_phase_order(self):
        with tempfile.TemporaryDirectory() as tmp:
            runner,commands,_=self.recording_runner(Path(tmp))
            (runner.checkout/'test.sln').write_text('fixture')
            runner.dotnet()
            self.assertEqual([p for _,_,p,_ in commands],['preparing','building','testing'])
            self.assertIn('--disable-parallel',commands[0][1])
            self.assertIn('--no-restore',commands[1][1])
            self.assertIn('--no-build',commands[2][1])
            (runner.checkout/'second.sln').write_text('fixture')
            with self.assertRaisesRegex(host.InfrastructureError,'ambiguous'):
                runner.dotnet()

    def test_empty_cmake_presets_fail(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=make_bundle(root)
            runner=self.runner(root,raw,job)
            runner.command=lambda *a,**k: ''
            with self.assertRaises(host.InfrastructureError):
                runner.cmake()

    def test_command_labels_obey_wire_byte_limit(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=make_bundle(root)
            runner=self.runner(root,raw,job)
            runner.checkout=root/'source';runner.folder=root
            class Tree:
                def run(self,*args,**kwargs): return 0,'ok'
            runner.assignment.tree=Tree()
            runner.command('é'*300,['fixture'])
            self.assertLessEqual(len(runner.commands[0]['label'].encode()),200)

    def test_common_tool_caches_stay_under_configured_root(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=make_bundle(root)
            runner=self.runner(root,raw,job)
            for name in ('NUGET_PACKAGES','NUGET_HTTP_CACHE_PATH','DOTNET_CLI_HOME','CCACHE_DIR','SCCACHE_DIR','VCPKG_DEFAULT_BINARY_CACHE','VCPKG_DOWNLOADS'):
                self.assertTrue(Path(runner.env[name]).is_relative_to(host.physical_path(root/'cache')),name)
            payload=json.loads(runner.execute())
            self.assertEqual(payload['status'],'passed',payload)
            # vcpkg rejects a binary cache directory that does not exist.
            self.assertTrue(Path(runner.env['VCPKG_DEFAULT_BINARY_CACHE']).is_dir())
            self.assertTrue(Path(runner.env['VCPKG_DOWNLOADS']).is_dir())

    def test_workload_environment_drops_secrets_and_git_configuration(self):
        with tempfile.TemporaryDirectory() as tmp:
            inherited={'PATH':'fixture','GH_TOKEN':'s','GITHUB_TOKEN':'s','OWNERSHIP_TOKEN_ACME':'s','Anthropic_Api_Key':'s','GIT_DIR':'elsewhere'}
            env=host.workload_environment(inherited,tmp,2)
            self.assertEqual(env['PATH'],'fixture')
            for name in ('GH_TOKEN','GITHUB_TOKEN','OWNERSHIP_TOKEN_ACME','Anthropic_Api_Key','GIT_DIR'):
                self.assertNotIn(name,env)
            self.assertEqual(env['MSBUILDDISABLENODEREUSE'],'1')
            self.assertEqual(env['VCPKG_MAX_CONCURRENCY'],'2')
            self.assertEqual(env['GIT_CONFIG_GLOBAL'],os.devnull)

    def test_checkout_keys_are_path_safe_and_distinct(self):
        self.assertNotEqual(host.checkout_key('a/b__c'),host.checkout_key('a__b/c'))
        self.assertTrue(host.checkout_key('rbretschneider/makercad').startswith('rbretschneider__makercad-'))
        for repo in ('../../escape','a/..','x\\y:z'):
            self.assertRegex(host.checkout_key(repo),r'^[A-Za-z0-9_.-]+$')
            self.assertNotIn('/',host.checkout_key(repo))

    def test_reused_checkout_keeps_ignored_build_output_and_drops_untracked_source(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            source=SourceRepo(root)
            raw,job=source.commit({**scripts(),'.gitignore':'build/\n','app.txt':'one\n'})
            first=self.runner(root,raw,job)
            self.assertEqual(json.loads(first.execute())['status'],'passed')
            checkout=first.checkout
            self.assertEqual(checkout.parent.parent.name,'runs')
            (checkout/'build').mkdir()
            (checkout/'build/object.o').write_text('incremental')
            (checkout/'stray.txt').write_text('untracked source')
            (checkout/'app.txt').write_text('locally modified')
            raw,job=source.commit({'app.txt':'two\n'})
            second=self.runner(root,raw,job)
            payload=json.loads(second.execute())
            self.assertEqual(payload['status'],'passed',payload)
            self.assertEqual(second.checkout,checkout)
            self.assertEqual(subprocess.check_output(['git','-C',str(checkout),'rev-parse','HEAD']).decode().strip(),job['sha'])
            self.assertEqual((checkout/'build/object.o').read_text(),'incremental')
            self.assertFalse((checkout/'stray.txt').exists())
            self.assertEqual((checkout/'app.txt').read_text().strip(),'two')
            self.assertNotIn('initialize checkout',[c['label'] for c in payload['result']['commands']])

    def test_parallel_assignments_of_one_repository_use_separate_checkouts(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=make_bundle(root)
            holder=self.runner(root,raw,job)
            held=holder.reserve_slot(host.physical_path(root/'builds'))
            try:
                runner=self.runner(root,raw,job)
                self.assertEqual(json.loads(runner.execute())['status'],'passed')
                self.assertNotEqual(runner.checkout.parent,held)
                self.assertEqual(runner.checkout.parent.parent,held.parent)
            finally:
                holder.slot_lock.close()

    def test_unusable_checkout_is_parked_not_deleted_and_job_still_passes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            source=SourceRepo(root)
            raw,job=source.commit(scripts())
            first=self.runner(root,raw,job)
            self.assertEqual(json.loads(first.execute())['status'],'passed')
            (first.checkout/'evidence.txt').write_text('keep')
            (first.checkout/'.git/HEAD').write_text('garbage')
            raw,job=source.commit({'app.txt':'two\n'})
            second=self.runner(root,raw,job)
            payload=json.loads(second.execute())
            self.assertEqual(payload['status'],'passed',payload)
            self.assertTrue(all(c['exit_code']==0 for c in payload['result']['commands']))
            parked=[p for p in first.checkout.parent.parent.iterdir() if '.broken-' in p.name]
            self.assertEqual(len(parked),1)
            self.assertEqual((parked[0]/'source/evidence.txt').read_text(),'keep')

    def test_command_silent_past_its_limit_is_recorded_as_failed_without_cancelling(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=make_bundle(root)
            runner=self.runner(root,raw,job)
            runner.checkout=runner.folder=root
            with host.ProcessTree(runner.assignment.cancel) as tree:
                runner.assignment.tree=tree
                with self.assertRaises(host.CommandFailed):
                    runner.command('hung test',[sys.executable,'-c','import time; print("started",flush=True); time.sleep(60)'],silence=1)
                self.assertFalse(runner.assignment.cancel.is_set())
                self.assertNotEqual(runner.commands[-1]['exit_code'],0)
                self.assertIn('started',runner.commands[-1]['log'])
                self.assertIn('hung',runner.commands[-1]['log'])
                self.assertEqual(runner.command('next',[sys.executable,'-c','print("still runs")']).strip(),'still runs')
                with self.assertRaises(host.InfrastructureError):
                    runner.command('slow preparation',[sys.executable,'-c','import time; time.sleep(60)'],timeout=1,infrastructure=True)

    def test_limits_report_time_and_silence(self):
        self.assertEqual(host.exceeded_limit(0,0,None,None,now=10**6),'')
        self.assertIn('time limit',host.exceeded_limit(0,99,10,None,now=11))
        self.assertIn('no output',host.exceeded_limit(0,0,None,60,now=61))
        self.assertEqual(host.exceeded_limit(0,30,100,60,now=61),'')


@unittest.skipUnless(host.OS=='windows','Windows verification script contract')
class WindowsScriptContractTests(unittest.TestCase):
    def runner(self, root, raw, job):
        return ExecutionTests.runner(self, root, raw, job)

    def test_script_preset_prefers_windows_verify_with_max_jobs(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=SourceRepo(root).commit({**scripts(),'.ownership/windows_verify.ps1':'param([int]$MaxJobs)\nWrite-Output "legacy contract $MaxJobs"\n'})
            payload=json.loads(self.runner(root,raw,job).execute())
            self.assertEqual(payload['status'],'passed',payload)
            self.assertIn('legacy contract 1',payload['result']['commands'][-1]['log'])

    def test_windows_verify_runs_after_a_standard_preset(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=SourceRepo(root).commit({'.ownership/windows_verify.ps1':'param([int]$MaxJobs)\nWrite-Output "after preset"\nexit 3\n'},preset='dotnet')
            runner=self.runner(root,raw,job)
            order=[]
            runner.dotnet=lambda:order.append('dotnet')
            payload=json.loads(runner.execute())
            self.assertEqual(order,['dotnet'])
            self.assertEqual(payload['status'],'failed')
            self.assertNotIn('failure_kind',payload['result'])
            self.assertEqual(payload['result']['commands'][-1]['label'],'verify script')
            self.assertEqual(payload['result']['commands'][-1]['exit_code'],3)
            self.assertIn('after preset',payload['result']['commands'][-1]['log'])

    def test_opted_in_script_gets_the_lock_file_and_runs_while_desktop_is_busy(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=SourceRepo(root).commit({'.ownership/windows_verify.ps1':'param([int]$MaxJobs)\nWrite-Output "lock=$env:OWNERSHIP_DESKTOP_LOCK"\n'})
            runner=self.runner(root,raw,job)
            lock=host.physical_path(root/'builds')/'desktop.lock'
            with host.HostLock(threading.Event(),lock):
                payload=json.loads(runner.execute())
            self.assertEqual(payload['status'],'passed',payload)
            self.assertIn('lock='+str(lock),payload['result']['commands'][-1]['log'])
            host.check_private(lock)

    def test_other_scripts_wait_for_the_whole_desktop(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=SourceRepo(root).commit({'.ownership/windows_verify.ps1':'Write-Output "ran"\n'})
            runner=self.runner(root,raw,job)
            result=[]
            with host.HostLock(threading.Event(),host.physical_path(root/'builds')/'desktop.lock'):
                worker=threading.Thread(target=lambda:result.append(json.loads(runner.execute())))
                worker.start()
                deadline=time.monotonic()+30
                while 'verify checkout SHA' not in [c['label'] for c in runner.commands] and time.monotonic()<deadline:
                    time.sleep(.05)
                time.sleep(1)
                self.assertEqual(runner.assignment.phase,'preparing')
                runner.assignment.cancel.set()
                worker.join(10)
            self.assertEqual(result[0]['result']['failure_kind'],'cancelled')
            self.assertEqual(result[0]['result']['commands'][-1]['label'],'verify checkout SHA')
