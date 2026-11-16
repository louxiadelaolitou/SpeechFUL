import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import unittest


MODULE_PATH = Path(__file__).resolve().parents[1] / 'prerelease' / 'environment.py'
SPEC = importlib.util.spec_from_file_location('gate_environment_under_test', MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ResumeDiagnosticTests(unittest.TestCase):
    def test_checkpoint_rollback_guard_requires_matching_reader_protocol(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            candidate, rollback = root / 'candidate', root / 'rollback'
            candidate.mkdir(); rollback.mkdir()
            self.assertFalse(MODULE.checkpoint_rollback_incompatible(candidate, rollback))
            relative = Path('services/api/internal/authprivacy/authorization_checkpoint.go')
            (candidate / relative).parent.mkdir(parents=True)
            (candidate / relative).write_text('new checkpoint protocol')
            self.assertTrue(MODULE.checkpoint_rollback_incompatible(candidate, rollback))
            (rollback / relative).parent.mkdir(parents=True)
            (rollback / relative).write_text('compatible checkpoint protocol')
            self.assertFalse(MODULE.checkpoint_rollback_incompatible(candidate, rollback))

    def fixture(self, directory):
        # Exercise only subprocess/log handling, with no runtime or database.
        lab = object.__new__(MODULE.Lab)
        lab.root = lab.api = Path(directory)
        lab.env = dict(os.environ)
        lab.state = None
        return lab

    def test_current_resume_failure_reaches_service_log_without_stale_output(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.fixture(directory)
            (lab.root / 'resume-c.private.log').write_text('authdev: historical diagnostic\n')
            command = [sys.executable, '-c', "import sys; print('authdev: ordinary resume blocked: c Gate is FROZEN (TIME_ROLLBACK)'); sys.exit(1)"]
            with self.assertRaises(RuntimeError) as failure:
                lab.run(command, 'resume-c')
            self.assertIn('TIME_ROLLBACK', str(failure.exception))
            self.assertNotIn('historical', str(failure.exception))

    def test_other_private_command_output_and_arguments_are_not_exposed(self):
        with tempfile.TemporaryDirectory() as directory:
            lab = self.fixture(directory)
            command = [sys.executable, '-c', "import sys; print('private-output-canary'); sys.exit(1)"]
            with self.assertRaises(RuntimeError) as failure:
                lab.run(command, 'migration-c')
            self.assertEqual(str(failure.exception), 'migration-c failed (see private log)')
            self.assertNotIn('private-output-canary', str(failure.exception))


if __name__ == '__main__':
    unittest.main()
