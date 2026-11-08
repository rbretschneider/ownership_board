import tempfile
import unittest
from pathlib import Path
from common import host

class FactsTests(unittest.TestCase):
    def test_complete_schema_and_unknown_optional_probes(self):
        with tempfile.TemporaryDirectory() as tmp:
            facts = host.FactsCollector(Path(tmp), probe=lambda *a: '').collect()
            self.assertEqual(set(facts), {'os','os_version','arch','hostname','lan_ip','cpu_model','cores','threads','ram_bytes','disks','gpus','toolchains','interactive_session','build_user_is_admin','agent_version'})
            self.assertGreaterEqual(facts['cores'], 1)
            self.assertGreaterEqual(facts['threads'], facts['cores'])
            self.assertEqual(facts['gpus'], [])
            self.assertNotIn(None, facts.values())
            self.assertTrue(facts['disks'])
            for disk in facts['disks']:
                self.assertEqual(set(disk), {'mount','total','free','is_build_volume'})
                self.assertLessEqual(disk['free'], disk['total'])

    def test_measured_gpu_version_is_not_inferred_from_name(self):
        def probe(args):
            if args[0]=='glxinfo':
                return 'OpenGL renderer string: Fixture GPU\nOpenGL version string: 4.6 fixture\n'
            if args[0]=='powershell.exe' and 'VideoController' in args[-1]:
                return '[{"Name":"Fixture GPU","DriverVersion":"1.0"}]'
            if args[0]=='system_profiler':
                return '{"SPDisplaysDataType":[{"sppci_model":"Fixture GPU"}]}'
            return ''
        with tempfile.TemporaryDirectory() as tmp:
            facts=host.FactsCollector(Path(tmp),probe=probe).collect()
            self.assertTrue(facts['gpus'])
            if host.OS=='linux': self.assertEqual(facts['gpus'][0]['opengl_version'],'4.6 fixture')
            else: self.assertEqual(facts['gpus'][0]['opengl_version'],'')

    def test_largest_writable_volume_default(self):
        from collections import namedtuple
        from unittest.mock import patch
        Usage=namedtuple('Usage','total free')
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            first,second=root/'small',root/'large'
            first.mkdir();second.mkdir()
            with patch.object(host,'volumes',return_value=[(first,Usage(100,80)),(second,Usage(200,150))]):
                build,cache=host.default_roots()
            self.assertTrue(build.is_relative_to(second))
            self.assertEqual(build.parent,cache.parent)
