import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
import urllib.parse
from unittest.mock import Mock

spec = importlib.util.spec_from_file_location('environment', Path(__file__).parents[1] / 'environment.py')
environment = importlib.util.module_from_spec(spec)
spec.loader.exec_module(environment)


class IsolatedEnvironmentTest(unittest.TestCase):
    def test_probe_keeps_verifier_captcha_connection_on_its_own_restored_cluster(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = environment.Lab(Path(directory) / 'new-root', Path('/source'))
            lab.state = dict(target='restore', tag='hnuhole_runtime_synthetic',
                             ports=dict(c=14443, v=14444, mail=18025),
                             pgPorts={'restore': dict(c=15432, v=15433)})
            lab.password_env = Mock(side_effect=lambda party, role: {
                'PGPASSWORD': 'synthetic-' + party + '-password',
                'AUTHRUNTIME_ISOLATED': '1',
            })
            lab.run = Mock()
            lab.probe('lifecycle')
            args, kwargs = lab.run.call_args
            env = kwargs['env']
            self.assertIn('@127.0.0.1:15432/hnuhole_c?', env['AUTHRUNTIME_C_DSN'])
            self.assertIn('@127.0.0.1:15433/hnuhole_v?', env['AUTHRUNTIME_V_DSN'])
            self.assertEqual(env['PGPASSWORD'], 'synthetic-c-password')
            self.assertEqual(env['AUTHRUNTIME_V_PASSWORD'], 'synthetic-v-password')
            self.assertNotIn('synthetic-v-password', str(args))

    @unittest.skipUnless(shutil.which('git'), 'Git executable required')
    def test_rollback_export_does_not_smudge_unrelated_mobile_assets(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / 'source'
            source.mkdir()
            git = ['git', '-C', str(source)]
            subprocess.run(git + ['init', '--quiet'], check=True)
            for key, value in (
                ('user.name', 'Test Fixture'),
                ('user.email', 'fixture@example.invalid'),
                ('filter.no-download.clean', 'cat'),
                ('filter.no-download.smudge', 'false'),
                ('filter.no-download.required', 'true'),
            ):
                subprocess.run(git + ['config', key, value], check=True)
            (source / '.gitattributes').write_text('*.ttf filter=no-download\n')
            (source / 'mobile.ttf').write_text('unavailable mobile font\n')
            api = source / 'services/api'
            api.mkdir(parents=True)
            (api / 'go.mod').write_text('module rollback_fixture\n')
            subprocess.run(git + ['add', '.gitattributes', 'mobile.ttf', 'services/api'], check=True)
            subprocess.run(git + ['commit', '--quiet', '-m', 'test fixture'], check=True)
            revision = subprocess.check_output(git + ['rev-parse', 'HEAD'], text=True).strip()
            # A full export would require the unavailable font filter.
            full = subprocess.run(git + ['archive', revision], stdout=subprocess.DEVNULL,
                                  stderr=subprocess.PIPE)
            self.assertNotEqual(full.returncode, 0)
            root = Path(directory) / 'lab'
            lab = environment.Lab(root, source)
            (root / 'rollback-source').mkdir(parents=True)
            lab.export_rollback_source(revision)
            self.assertEqual((root / 'rollback-source/services/api/go.mod').read_text(),
                             'module rollback_fixture\n')
            self.assertFalse((root / 'rollback.tar').exists())
            self.assertFalse((root / 'rollback-source/mobile.ttf').exists())

    def test_cleanup_does_not_treat_pg_status_error_as_stopped(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'new-root'
            lab = environment.Lab(root, Path('/source'))
            lab.state = dict(processes={}, pgPorts={'source': {'c': 15432, 'v': 15433}})
            for party in 'cv':
                (root / ('source-' + party) / 'data').mkdir(parents=True)
            lab.run = Mock(return_value=4)
            with self.assertRaisesRegex(RuntimeError, 'could not prove owned PostgreSQL stopped'):
                lab.stop()
            self.assertEqual(lab.run.call_count, 2)

    def test_resume_failure_keeps_both_api_listeners_stopped(self):
        lab = environment.Lab(Path(tempfile.gettempdir()) / 'nonexistent-prerelease-resume-unit-root', Path('/source'))
        lab.state = dict(target='source', processes={}, ports={'mail': 18025, 'smtp': 11025})
        lab.pg_start = Mock()
        lab.spawn = Mock()
        lab.operator = Mock(side_effect=[None, RuntimeError('V checkpoint mismatch')])
        with self.assertRaisesRegex(RuntimeError, 'V checkpoint mismatch'):
            lab.start()
        # Even with C healthy, neither protected API may accept traffic while
        # V's independent resume has failed. No fallback recovery is invoked.
        self.assertEqual([c.args for c in lab.operator.call_args_list], [('resume', 'c'), ('resume', 'v')])
        self.assertFalse(any(c.args[0] in ('c', 'v') for c in lab.spawn.call_args_list))

    def test_migrator_dsn_works_with_libpq_percent_decoding(self):
        lab = environment.Lab(Path(tempfile.gettempdir()) / 'nonexistent-prerelease-unit-root', Path('/source'))
        lab.state = dict(target='source', tag='hnuhole_runtime_unit', pgPorts={'source': {'c': 15432}})
        raw = lab.dsn('c', 'migrator')
        # libpq URI parameters percent-decode, but do not form-decode '+'.
        params = {k: urllib.parse.unquote(v) for k, v in (part.split('=', 1) for part in raw.split('?', 1)[1].split('&'))}
        self.assertEqual(params['options'], '-c role=hnuhole_c_owner')
        self.assertNotIn('+', raw)

    @unittest.skipUnless(os.name == 'posix', 'Linux ownership boundary')
    def test_reject_shared_or_symlink_root_before_reading_state(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'shared'
            root.mkdir(mode=0o755)
            root.chmod(0o755)  # Fixture must stay shared even under an owned 077 umask.
            with self.assertRaises(RuntimeError):
                environment.Lab(root, Path('/source'))
            root.chmod(0o700)
            link = Path(directory) / 'link'
            link.symlink_to(root)
            with self.assertRaises(RuntimeError):
                environment.Lab(link, Path('/source'))


if __name__ == '__main__':
    unittest.main()
