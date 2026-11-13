from pathlib import Path
import tempfile
import unittest
from repair_aof_replay_acl import aof_commands, updated_acl


class AofAclTests(unittest.TestCase):
    def test_network_lockout_and_runtime_permissions_preserved(self):
        source = 'user default off -@all\nuser app on #redacted ~test:* -@all +get +set\n'
        result = updated_acl(source, 'app', {'SET', 'DEL', 'MULTI', 'EXEC'})
        rule = result.splitlines()[0].split()
        self.assertIn('off', rule)
        self.assertIn('resetpass', rule)
        self.assertNotIn('on', rule)
        self.assertNotIn('nopass', rule)
        self.assertNotIn('+@all', rule)
        self.assertIn('~test:*', rule)
        self.assertEqual(result.splitlines()[1], source.splitlines()[1])

    def test_unscoped_or_enabled_default_refused(self):
        for source in ('user default on -@all\nuser app on ~test:* +set\n',
                       'user default off -@all\nuser app on ~* +set\n'):
            with self.assertRaises(ValueError):
                updated_acl(source, 'app', {'SET'})

    def test_parser_skips_payload_and_refuses_dangerous_commands(self):
        with tempfile.TemporaryDirectory() as directory:
            file = Path(directory) / 'incr.aof'
            file.write_bytes(b'*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$6\r\nsecret\r\n')
            self.assertEqual(aof_commands(file), {'SET'})
            file.write_bytes(b'*1\r\n$8\r\nFLUSHALL\r\n')
            with self.assertRaises(ValueError):
                aof_commands(file)
            file.write_bytes(b'*3\r\n$3\r\nSET\r\n$1\r\nk\r\n$6\r\nsec')
            with self.assertRaises(ValueError):
                aof_commands(file)


if __name__ == '__main__':
    unittest.main()
