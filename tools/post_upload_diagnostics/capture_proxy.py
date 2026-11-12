"""Bounded, owner-run upload-only Nginx capture; restores the exact config."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time


def build_config(original, output):
    if original.count('http {') != 1 or 'post_upload_diag' in original:
        raise ValueError('Unexpected config layout or diagnostics already enabled')
    marker = ' access_log off;\n location / {'
    if original.count(marker) != 2:
        raise ValueError('Expected separate community and verifier servers')
    if any(c not in '/abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-. ' for c in str(output)) or ' ' in str(output):
        raise ValueError('Unsafe output path')
    fields = {
        'route': 'post_image_bytes', 'time': '$time_iso8601', 'unix_s': '$msec',
        'status': '$status', 'request_length': '$request_length',
        'declared_body_length': '$post_upload_diag_length',
        'request_completion': '$request_completion', 'request_time': '$request_time',
        'upstream_status': '$upstream_status',
        'upstream_connect_time': '$upstream_connect_time',
        'upstream_header_time': '$upstream_header_time',
        'upstream_response_time': '$upstream_response_time',
        'upstream_bytes_sent': '$upstream_bytes_sent',
        'server_request_id': '$post_upload_diag_request_id',
    }
    header = '''
map "$request_method:$uri" $post_upload_diag_enabled {
 default 0;
 "~^PUT:/api/v1/me/post-tasks/[0-9a-f-]+/attempts/[0-9]+/images/[0-9a-f-]+/bytes$" 1;
}
map $http_content_length $post_upload_diag_length {
 default "-";
 "~^[0-9]{1,8}$" $http_content_length;
}
map $upstream_http_x_request_id $post_upload_diag_request_id {
 default "-";
 "~^[A-Za-z0-9_-]{21}[AQgw]$" $upstream_http_x_request_id;
}
'''
    header += "log_format post_upload_diag escape=json '" + json.dumps(fields, separators=(',', ':')) + "';\n"
    result = original.replace('http {', 'http {' + header, 1)
    return result.replace(marker, ' access_log ' + str(output) + ' post_upload_diag if=$post_upload_diag_enabled;\n location / {', 1)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--config', type=Path, required=True)
    p.add_argument('--nginx', type=Path, required=True)
    p.add_argument('--output-dir', type=Path, required=True)
    p.add_argument('--expected-sha256', required=True)
    p.add_argument('--seconds', type=int, default=240)
    a = p.parse_args()
    if os.geteuid() == 0 or not 30 <= a.seconds <= 600:
        raise ValueError('Use the existing deployment owner and a bounded duration')
    if a.config.is_symlink() or a.config.stat().st_uid != os.getuid():
        raise ValueError('Config must be owned and not a symlink')
    original = a.config.read_bytes()
    if hashlib.sha256(original).hexdigest() != a.expected_sha256:
        raise ValueError('Configuration changed; inspect before retrying')
    a.output_dir.mkdir(mode=0o700, parents=False, exist_ok=False)
    backup = a.output_dir / 'nginx.before.conf'
    backup.write_bytes(original)
    backup.chmod(0o600)
    log = a.output_dir / 'upload.jsonl'
    log.touch(mode=0o600, exist_ok=False)
    candidate = build_config(original.decode(), log).encode()
    check = a.output_dir / 'nginx.candidate.conf'
    check.write_bytes(candidate)
    check.chmod(0o600)
    cmd = [str(a.nginx), '-p', str(a.config.parent), '-c']
    def run(*args):
        subprocess.run(list(args), check=True, timeout=15, capture_output=True)
    run(*cmd, str(check), '-t')
    mode = a.config.stat().st_mode & 0o777
    def replace_config(data):
        temporary = a.output_dir / 'config.swap'
        with temporary.open('xb') as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        temporary.chmod(mode)
        os.replace(temporary, a.config)
    applied = False
    try:
        replace_config(candidate)
        applied = True
        run(*cmd, str(a.config), '-t')
        run(*cmd, str(a.config), '-s', 'reload')
        print('CAPTURE_READY ' + str(log), flush=True)
        until = time.monotonic() + a.seconds
        offset = 0
        while time.monotonic() < until and not (a.output_dir / 'stop').exists():
            with log.open() as f:
                f.seek(offset)
                for line in f:
                    value = json.loads(line)
                    print(json.dumps(value), flush=True)
                offset = f.tell()
            time.sleep(0.5)
    finally:
        if applied:
            if a.config.read_bytes() not in (candidate, original):
                raise RuntimeError('Concurrent config change; backup retained, not overwritten')
            replace_config(original)
            run(*cmd, str(a.config), '-t')
            run(*cmd, str(a.config), '-s', 'reload')
            print('RESTORED_EXACT_CONFIG ' + hashlib.sha256(original).hexdigest(), flush=True)


if __name__ == '__main__':
    main()
