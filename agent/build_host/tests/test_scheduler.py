import base64
import json
from pathlib import Path
import tempfile
import threading
import unittest
from unittest.mock import patch
from common import host

class Clock:
    def __init__(self): self.now=0
    def __call__(self): return self.now

class Board:
    def __init__(self):
        self.calls=[]
        self.status={'status':'approved','enabled':True,'max_parallel':3}
        self.claims=[]
        self.error=None
    def request(self,method,path,body=None):
        self.calls.append((method,path,body))
        if self.error: raise self.error
        if path.endswith('/status'): return dict(self.status)
        if path.endswith('/claim'): return {'job':self.claims.pop(0)} if self.claims else None
        if path.endswith('/progress'): return {'stop':False,'attempt':body['attempt']}
        return {}

class Facts:
    def collect(self): return {'disks':[]}

class SchedulerTests(unittest.TestCase):
    def setup_loop(self,tmp):
        root=Path(tmp)
        state=host.State(root/'state')
        config={'host_id':7,'build_root':str(root/'builds'),'cache_root':str(root/'cache'),'max_jobs':1,'max_parallel':3,'reserve_gib':0}
        host.private_dir(root/'builds'); host.private_dir(root/'cache')
        board,clock=Board(),Clock()
        loop=host.AgentLoop(state,board,config,Facts(),clock=clock)
        return loop,board,clock

    def test_heartbeat_cadence_during_active_work_and_disabled(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            board.status['enabled']=False
            a=host.Assignment({'id':'b'*64,'attempt':1},None)
            loop.active[a.job['id']]=a
            for at in (0,2,59,60,120):
                clock.now=at
                loop.tick()
            reports=[body for _,path,body in board.calls if path.endswith('/heartbeat')]
            self.assertEqual(len(reports),3)
            self.assertEqual(reports[0]['running_job_ids'],['b'*64])
            self.assertFalse(any(p.endswith('/claim') for _,p,_ in board.calls))

    def test_disk_reserve_pause_resume_and_capacity(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            loop.config['reserve_gib']=50
            with patch.object(host,'free_bytes',return_value=1): loop.tick()
            self.assertFalse(any(p.endswith('/claim') for _,p,_ in board.calls))
            clock.now=10
            with patch.object(host,'free_bytes',return_value=100*host.GIB): loop.tick()
            self.assertTrue(any(p.endswith('/claim') for _,p,_ in board.calls))
            loop.config['max_jobs']=2
            loop.config['max_parallel']=8
            with patch.object(host,'cpu_budget',return_value=3):
                self.assertEqual(loop.capacity(),1)

    def test_connectivity_cutoff_kills_work_and_ambiguous_claim_waits(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            board.status['enabled']=False
            loop.tick()
            assignment=host.Assignment({'id':'c'*64,'attempt':1},None)
            loop.active[assignment.job['id']]=assignment
            board.error=OSError('offline')
            clock.now=60; loop.tick()
            self.assertFalse(assignment.cancel.is_set())
            clock.now=181; loop.tick()
            self.assertTrue(assignment.cancel.is_set())
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            original=board.request
            def lost(method,path,body=None):
                if path.endswith('/claim'): raise OSError('ambiguous')
                return original(method,path,body)
            board.request=lost
            loop.tick()
            count=len(board.calls)
            clock.now=120; loop.tick()
            self.assertEqual(len(board.calls),count)
            self.assertGreaterEqual(loop.quiet_until,185)

    def test_restart_retries_exact_completion_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            job={'id':'d'*64,'attempt':1}
            raw=host.completion_bytes(1,[], 'infrastructure')
            assignment=host.Assignment(job,loop.journals/'saved.json')
            assignment.payload=raw
            assignment.done.set()
            loop.persist(assignment)
            loop.recover()
            completions=[body for _,p,body in board.calls if p.endswith('/result')]
            self.assertEqual(completions,[raw])
            self.assertTrue(json.loads(assignment.journal.read_bytes())['settled'])

    def test_lost_claim_response_withholds_all_control_traffic_after_restart(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            marker=loop.state.root/'claim.json'
            original=board.request
            def lost(method,path,body=None):
                if path.endswith('/claim'):
                    self.assertTrue(json.loads(marker.read_bytes())['pending'])
                    raise OSError('response lost')
                return original(method,path,body)
            board.request=lost
            loop.tick()
            self.assertTrue(marker.exists())
            # Known journals must also wait: progress/completion can touch health.
            assignment=host.Assignment({'id':'d'*64,'attempt':1},loop.journals/'saved.json')
            assignment.payload=host.completion_bytes(1,[],'infrastructure')
            loop.persist(assignment)
            restarted,board,clock=self.setup_loop(tmp)
            for at in (0,2,60,120,184):
                clock.now=at
                restarted.recover()
                restarted.tick()
                self.assertEqual(board.calls,[])
            # Another restart cannot bypass the barrier; clocks may reset at boot.
            restarted,board,clock=self.setup_loop(tmp)
            restarted.tick()
            self.assertEqual(board.calls,[])
            clock.now=185
            restarted.recover()
            restarted.tick()
            self.assertTrue(any(p.endswith('/result') for _,p,_ in board.calls))
            self.assertTrue(any(p.endswith('/heartbeat') for _,p,_ in board.calls))
            self.assertFalse(json.loads(marker.read_bytes())['pending'])
            restarted,board,clock=self.setup_loop(tmp)
            restarted.tick()
            self.assertTrue(any(p.endswith('/heartbeat') for _,p,_ in board.calls))

    def test_interruption_during_claim_preserves_restart_barrier(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            original=board.request
            def interrupted(method,path,body=None):
                if path.endswith('/claim'): raise KeyboardInterrupt()
                return original(method,path,body)
            board.request=interrupted
            with self.assertRaises(KeyboardInterrupt): loop.tick()
            restarted,board,clock=self.setup_loop(tmp)
            restarted.tick()
            self.assertEqual(board.calls,[])

    def test_claim_marker_write_failure_prevents_claim_request(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            with patch.object(host,'atomic_private',side_effect=OSError('disk full')):
                loop.tick()
            self.assertFalse(any(p.endswith('/claim') for _,p,_ in board.calls))

    def test_definitive_claim_rejection_does_not_delay_restart(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            original=board.request
            def rejected(method,path,body=None):
                if path.endswith('/claim'): raise host.BoardError(429)
                return original(method,path,body)
            board.request=rejected
            loop.tick()
            restarted,board,clock=self.setup_loop(tmp)
            restarted.tick()
            self.assertTrue(any(p.endswith('/heartbeat') for _,p,_ in board.calls))

    def test_failed_marker_settlement_preserves_barrier_after_restart(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            write=host.atomic_private
            def fail_settlement(path,raw):
                if path == loop.state.root/'claim.json' and not json.loads(raw)['pending']:
                    raise OSError('disk full')
                return write(path,raw)
            with patch.object(host,'atomic_private',side_effect=fail_settlement):
                loop.tick()
                self.assertTrue(any(p.endswith('/claim') for _,p,_ in board.calls))
            restarted,board,clock=self.setup_loop(tmp)
            restarted.tick()
            self.assertEqual(board.calls,[])

    def test_successful_claim_is_journaled_before_marker_settlement(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            job={'id':'a'*64,'attempt':1,'host_id':7,'repo':'acme/app','sha':'b'*40,
                 'bundle_sha256':'c'*64,'bundle_bytes':1,'preset':'script',
                 'target':{'os':host.OS,'required_capabilities':[]}}
            board.claims.append(job)
            write=host.atomic_private
            def observe_settlement(path,raw):
                if path == loop.state.root/'claim.json' and not json.loads(raw)['pending']:
                    journals=[json.loads(p.read_bytes()) for p in loop.journals.glob('*.json')]
                    self.assertTrue(any(j['job'] == job for j in journals))
                return write(path,raw)
            with patch.object(host,'atomic_private',side_effect=observe_settlement), \
                 patch.object(host.JobRunner,'execute',return_value=host.completion_bytes(1,[],'infrastructure')):
                loop.tick()
                loop.active[job['id']].thread.join(5)
            self.assertFalse(json.loads((loop.state.root/'claim.json').read_bytes())['pending'])

    def test_malformed_claim_marker_fails_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            marker=loop.state.root/'claim.json'
            for value in ({}, {'pending':'false'}, [], None):
                host.atomic_private(marker,host.json_bytes(value))
                with self.subTest(value=value),self.assertRaises(ValueError):
                    self.setup_loop(tmp)

    def test_resource_limits_honor_smaller_environment(self):
        config={'max_jobs':3}
        with patch.object(host,'cpu_budget',return_value=3):
            limit,env=host.resource_limits(config,{'MAKEFLAGS':'-j2','OMP_NUM_THREADS':'1'})
            self.assertEqual(limit,1)
            self.assertEqual(env['CMAKE_BUILD_PARALLEL_LEVEL'],'1')
            self.assertIn('-j1',env['MAKEFLAGS'])
            for bad in ({'MAKEFLAGS':'-j'},{'MAKEFLAGS':'--jobs=0'},{'OMP_NUM_THREADS':'2,1'},{'CTEST_PARALLEL_LEVEL':''}):
                with self.subTest(bad=bad),self.assertRaises(ValueError): host.resource_limits(config,bad)

    def test_server_error_after_claim_is_ambiguous(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            original=board.request
            def broken(method,path,body=None):
                if path.endswith('/claim'): raise host.BoardError(500)
                return original(method,path,body)
            board.request=broken
            loop.tick()
            self.assertGreaterEqual(loop.quiet_until,185)

    def test_health_deadline_is_measured_from_last_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            board.status['enabled']=False
            loop.tick()
            assignment=host.Assignment({'id':'e'*64,'attempt':1},None)
            loop.active[assignment.job['id']]=assignment
            board.error=OSError('offline')
            clock.now=60;loop.tick()
            clock.now=121;loop.tick()
            self.assertTrue(assignment.cancel.is_set())

    def test_malformed_progress_fails_closed(self):
        assignment=host.Assignment({'id':'f'*64,'attempt':1},None)
        class Broken:
            def request(self,*args): return {'attempt':1}
        with self.assertRaises(ValueError): host.poll_assignment(Broken(),assignment)
        self.assertTrue(assignment.cancel.is_set())

    def test_upload_requires_durable_completion_before_network(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            assignment=host.Assignment({'id':'a'*64,'attempt':1},loop.journals/'completion.json')
            assignment.payload=host.completion_bytes(1,[],'infrastructure')
            loop.persist(assignment)
            with patch.object(host,'atomic_private',side_effect=OSError('disk full')):
                with self.assertRaises(OSError): loop.upload(assignment)
            self.assertFalse(any(p.endswith('/result') for _,p,_ in board.calls))

    def test_claim_journal_failure_withholds_health_until_assignment_expires(self):
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            job={'id':'a'*64,'attempt':1,'host_id':7,'repo':'acme/app','sha':'b'*40,
                 'bundle_sha256':'c'*64,'bundle_bytes':1,'preset':'script',
                 'target':{'os':host.OS,'required_capabilities':[]}}
            board.claims.append(job)
            write=host.atomic_private
            def fail_assignment(path,raw):
                if path.parent == loop.journals: raise OSError('disk full')
                return write(path,raw)
            with patch.object(host,'atomic_private',side_effect=fail_assignment), \
                 patch.object(host.JobRunner,'execute') as execute:
                loop.tick()
                self.assertFalse(loop.active)
                self.assertFalse(list(loop.journals.glob('*.json')))
                execute.assert_not_called()
                count=len(board.calls)
                for at in (2,60,120,184):
                    clock.now=at
                    loop.tick()
                    self.assertEqual(len(board.calls),count)
                self.assertGreaterEqual(loop.quiet_until,185)
            restarted,restarted_board,_=self.setup_loop(tmp)
            restarted.tick()
            self.assertEqual(restarted_board.calls,[])
            clock.now=186
            loop.tick()
            self.assertGreater(len(board.calls),count)
            self.assertEqual(sum(p.endswith('/heartbeat') for _,p,_ in board.calls),2)

    def test_health_reports_build_volume_while_cache_reserve_pauses_claims(self):
        from collections import namedtuple
        Usage=namedtuple('Usage','free')
        with tempfile.TemporaryDirectory() as tmp:
            loop,board,clock=self.setup_loop(tmp)
            loop.config['reserve_gib']=50
            def usage(path):
                return Usage((100 if str(path)==loop.config['build_root'] else 10)*host.GIB)
            with patch.object(host.shutil,'disk_usage',side_effect=usage): loop.tick()
            reports=[body for _,p,body in board.calls if p.endswith('/heartbeat')]
            self.assertEqual(reports[0]['free_disk_bytes'],100*host.GIB)
            self.assertFalse(any(p.endswith('/claim') for _,p,_ in board.calls))
