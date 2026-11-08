import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import unittest
from common import host

class BundleBoard:
    def __init__(self, raw):
        self.raw = raw
    def open(self, *args, **kwargs):
        return contextlib.closing(io.BytesIO(self.raw))


def make_bundle(root, script='printf "verified\\n"\n'):
    repo = root/'source'
    repo.mkdir()
    def git(*args):
        return subprocess.check_output(['git','-C',str(repo),*args], stderr=subprocess.STDOUT).decode().strip()
    git('init')
    git('config','user.name','Fixture')
    git('config','user.email','fixture@example.invalid')
    (repo/'.ownership').mkdir()
    (repo/'.ownership/verify.sh').write_text(script)
    (repo/'.ownership/verify.ps1').write_text('param([int]$MaxJobs)\nWrite-Output "verified $MaxJobs"\n')
    git('add','.')
    git('commit','-m','Fixture')
    sha = git('rev-parse','HEAD')
    bundle = root/'fixture.bundle'
    git('bundle','create',str(bundle),'HEAD')
    raw = bundle.read_bytes()
    job = {'id':'a'*64,'attempt':1,'host_id':7,'sha':sha,'bundle_bytes':len(raw),'bundle_sha256':hashlib.sha256(raw).hexdigest(),'preset':'script','target':{'os':host.OS,'required_capabilities':[]}}
    return raw, job


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
        for patch in ({'id':'../escape'},{'attempt':0},{'host_id':8},{'sha':'HEAD'},{'target':{'os':'other'}},{'bundle_bytes':0},{'preset':'sh'}):
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

    def test_preset_arguments_and_phase_order(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            raw,job=make_bundle(root)
            runner=self.runner(root,raw,job)
            runner.checkout=root/'source'
            commands=[]
            def command(label,args,phase='preparing',**kw):
                commands.append((label,args,phase))
                if '--list-presets=configure' in args:
                    return 'Available configure presets:\n  "'+host.OS+'-configure"\n'
                if '--list-presets=build' in args:
                    return 'Available build presets:\n  "'+host.OS+'-compile"\n'
                if '--list-presets' in args:
                    return 'Available test presets:\n  "'+host.OS+'-test"\n'
                return 'Total tests: 1\n' if label == 'dotnet Release tests' else ''
            runner.command=command
            runner.cmake()
            self.assertEqual([p for _,_,p in commands][-3:],['preparing','building','testing'])
            self.assertIn('--parallel',commands[-2][1])
            self.assertIn('until-pass:3',commands[-1][1])
            self.assertIn('--no-tests=error',commands[-1][1])
            commands.clear()
            (runner.checkout/'test.sln').write_text('fixture')
            runner.dotnet()
            self.assertEqual([p for _,_,p in commands],['preparing','building','testing'])
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
