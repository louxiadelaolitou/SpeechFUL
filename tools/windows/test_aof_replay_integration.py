"""Real Redis restart regression. Uses only a fresh temporary Linux directory."""
import hashlib
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import unittest
from repair_aof_replay_acl import aof_commands, updated_acl


@unittest.skipUnless(os.environ.get('REDIS_SERVER') and os.environ.get('REDIS_CLI'), 'Set explicit Redis binaries')
class ReplayIntegration(unittest.TestCase):
    def test_restart_replays_transactions_without_network_permission_expansion(self):
        with tempfile.TemporaryDirectory(prefix='speechful-aof-regression-') as directory:
            root = Path(directory)
            password = secrets.token_hex(24)
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                port = sock.getsockname()[1]
            acl = root / 'users.acl'
            runtime = 'user app on #' + hashlib.sha256(password.encode()).hexdigest() + ' ~test:* -@all +get +set +setnx +eval +ping\n'
            acl.write_text('user default off -@all\n' + runtime)
            config = root / 'redis.conf'
            config.write_text('bind 127.0.0.1\nport ' + str(port) + '\nsave ""\nappendonly yes\ndir ' + str(root) + '\naclfile ' + str(acl) + '\n')
            environment = dict(os.environ, REDISCLI_AUTH=password)
            log = (root / 'redis.log').open('ab')
            process = None

            def cli(*args, authenticate=True):
                command = [os.environ['REDIS_CLI'], '--raw', '-h', '127.0.0.1', '-p', str(port)]
                env = dict(environment)
                if authenticate:
                    command += ['--user', 'app']
                else:
                    env.pop('REDISCLI_AUTH', None)
                result = subprocess.run(command + list(args), env=env, capture_output=True, text=True, timeout=5)
                return (result.stdout + result.stderr).strip()

            def start():
                nonlocal process
                process = subprocess.Popen([os.environ['REDIS_SERVER'], str(config)], stdout=log, stderr=subprocess.STDOUT)
                for _ in range(50):
                    if process.poll() is not None:
                        self.fail('Fresh fixture Redis exited')
                    if cli('PING') == 'PONG':
                        return
                    time.sleep(.1)
                self.fail('Fixture Redis not ready')

            def stop():
                nonlocal process
                if process is not None:
                    process.terminate()
                    process.wait(timeout=10)
                    process = None

            try:
                start()
                self.assertEqual(cli('EVAL', "redis.call('SET',KEYS[1],ARGV[1]);redis.call('SET',KEYS[2],ARGV[2]);return 1", '2', 'test:a', 'test:b', 'one', 'two'), '1')
                self.assertEqual(cli('SETNX', 'test:seed', 'seed'), '1')
                stop()
                start()
                before_repair = cli('GET', 'test:a')
                self.assertIn(before_repair, ('', 'one'))
                if before_repair == '':
                    self.assertIn('AOF-loading-client', (root / 'redis.log').read_text())
                stop()
                commands = set()
                for file in (root / 'appendonlydir').glob('*.aof'):
                    commands |= aof_commands(file)
                acl.write_text(updated_acl(acl.read_text(), 'app', commands))
                self.assertEqual(acl.read_text().splitlines()[1] + '\n', runtime)
                for _ in range(2):
                    start()
                    self.assertEqual(cli('GET', 'test:a'), 'one')
                    self.assertEqual(cli('GET', 'test:b'), 'two')
                    self.assertEqual(cli('GET', 'test:seed'), 'seed')
                    self.assertIn('NOAUTH', cli('PING', authenticate=False))
                    self.assertIn('WRONGPASS', cli('AUTH', 'default', password, authenticate=False))
                    self.assertIn('NOPERM', cli('FLUSHALL'))
                    self.assertIn('NOPERM', cli('GET', 'outside:key'))
                    stop()
            finally:
                stop()
                log.close()


if __name__ == '__main__':
    unittest.main()
