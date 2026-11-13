"""Repair Redis 7 AOF replay ACL while keeping the default user locked out.

Operator-only, offline action. Mirrors Redis issue #14156 with a narrower
command/key allowlist: only commands already present in this instance's AOF,
and only the existing runtime user's key patterns. No network user is enabled.
"""
import argparse
import os
from pathlib import Path
import re
import shutil
import socket


ALLOWED = {'SELECT', 'MULTI', 'EXEC', 'SET', 'SETNX', 'SETRANGE', 'DEL', 'UNLINK',
           'PEXPIREAT', 'EXPIREAT', 'HSET', 'HDEL', 'HINCRBY', 'ZADD', 'ZREM',
           'ZREMRANGEBYSCORE', 'ZREMRANGEBYRANK', 'SADD', 'SREM', 'INCRBY',
           'DECRBY', 'INCR', 'DECR', 'APPEND'}


def aof_commands(path):
    commands = set()
    with path.open('rb') as stream:
        while True:
            header = stream.readline()
            if not header:
                break
            if not header.startswith(b'*'):
                raise ValueError('Unexpected AOF encoding; review manually')
            count = int(header[1:])
            if not 1 <= count <= 100000:
                raise ValueError('Invalid AOF argument count')
            for index in range(count):
                size_line = stream.readline()
                if not size_line.startswith(b'$'):
                    raise ValueError('Unexpected AOF argument')
                size = int(size_line[1:])
                if size < 0 or size > 64 * 1024 * 1024:
                    raise ValueError('AOF argument exceeds inspection budget')
                if index == 0:
                    if size > 64:
                        raise ValueError('Unexpected AOF command length')
                    commands.add(stream.read(size).decode('ascii').upper())
                else:
                    stream.seek(size, 1)
                if stream.read(2) != b'\r\n':
                    raise ValueError('Truncated AOF; never repair automatically')
    if commands - ALLOWED:
        raise ValueError('Unexpected persisted command; review manually')
    return commands


def updated_acl(text, scope_from_user, commands):
    lines = text.splitlines()
    default = [i for i, line in enumerate(lines) if line.startswith('user default ')]
    runtime = [line.split() for line in lines if line.startswith('user ' + scope_from_user + ' ')]
    if len(default) != 1 or len(runtime) != 1 or 'off' not in lines[default[0]].split():
        raise ValueError('Expected a single disabled default user')
    patterns = [v for v in runtime[0] if v.startswith('~')]
    if not patterns or any(v == '~*' or not v.endswith('*') for v in patterns):
        raise ValueError('Runtime key permissions must be explicitly scoped')
    if not commands or commands - ALLOWED:
        raise ValueError('Invalid persisted command allowlist')
    rule = ['user', 'default', 'off', 'resetpass', 'resetkeys', 'resetchannels', '-@all']
    rule += patterns + ['+' + v.lower() for v in sorted(commands)]
    lines[default[0]] = ' '.join(rule)
    return '\n'.join(lines) + '\n'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('config', type=Path)
    parser.add_argument('backup', type=Path)
    parser.add_argument('--target-user', choices=['default'], required=True)
    parser.add_argument('--scope-from-user', required=True,
                        help='Read this existing user key scope; never modify its ACL line')
    args = parser.parse_args()
    config = args.config.read_text()
    port_match = re.search(r'^port\s+(\d+)$', config, re.M)
    if not port_match:
        raise ValueError('Missing explicit loopback port')
    try:
        with socket.create_connection(('127.0.0.1', int(port_match.group(1))), timeout=.2):
            raise ValueError('Redis is running; offline repair refused')
    except ConnectionRefusedError:
        pass
    def setting(name):
        match = re.search('^' + name + r'\s+(.+)$', config, re.M)
        if not match:
            raise ValueError('Missing explicit Redis ' + name)
        return Path(match.group(1).strip().strip('"')).resolve()
    data = setting('dir')
    acl = setting('aclfile')
    if not acl.is_relative_to(data):
        raise ValueError('ACL must belong to this Redis data directory')
    commands = set()
    for aof in (data / 'appendonlydir').glob('*.aof'):
        commands |= aof_commands(aof)
    original = acl.read_text()
    replacement = updated_acl(original, args.scope_from_user, commands)
    before_other = [line for line in original.splitlines() if not line.startswith('user default ')]
    after_other = [line for line in replacement.splitlines() if not line.startswith('user default ')]
    if before_other != after_other:
        raise ValueError('Non-default account changed; refuse modification')
    # Offline copy includes ACL and every AOF part for an exact rollback.
    if args.backup.exists():
        parser.error('Backup exists; preserve it')
    os.umask(0o077)
    shutil.copytree(data, args.backup)
    temporary = acl.with_name(acl.name + '.frp-repair.tmp')
    with temporary.open('x', encoding='utf-8') as stream:
        stream.write(replacement)
    temporary.chmod(acl.stat().st_mode & 0o777)
    temporary.replace(acl)
    print('PASS: disabled default user; runtime key scope retained; AOF commands: ' + ','.join(sorted(commands)))
