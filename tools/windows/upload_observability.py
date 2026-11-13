"""Owner-installed, upload-only Nginx telemetry and a read-only local report.

No bodies, credentials, resource paths, client addresses or arbitrary headers
are logged. Existing system journal retention applies; no public admin port.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

TAG = 'speechful_upload'
FIELDS = {
    'schema': 'upload-v1', 'time': '$time_iso8601',
    'entry': '$speechful_upload_entry', 'route': '$speechful_upload_route',
    'status': '$status', 'request_length': '$request_length',
    'declared_body_length': '$speechful_upload_length',
    'completion': '$request_completion', 'duration_s': '$request_time',
    'upstream_status': '$upstream_status',
    'upstream_connect_s': '$upstream_connect_time',
    'upstream_header_s': '$upstream_header_time',
    'upstream_response_s': '$upstream_response_time',
    'request_id': '$speechful_upload_request_id',
}
ROUTES = {'post_create', 'upload_state', 'upload_grant', 'image_bytes'}


def build_config(original, old_host, frp_host):
    for host in (old_host, frp_host):
        if not re.fullmatch(r'[a-z0-9]+(?:[.-][a-z0-9]+)*', host):
            raise ValueError('Expected plain lowercase DNS hostnames')
    if old_host == frp_host:
        raise ValueError('Old and Frp hosts must differ')
    marker = ' access_log off;\n location / {'
    if (original.count('http {') != 1 or original.count(marker) != 2
            or 'speechful_upload' in original):
        raise ValueError('Unexpected or already instrumented two-server config')
    # Route names are constants. UUIDs and command IDs never enter the log.
    header = '''
map "$request_method:$uri" $speechful_upload_route {
 default "";
 "~^PUT:/api/v1/post-commands/[A-Za-z0-9_-]+$" post_create;
 "~^GET:/api/v1/me/post-tasks/[0-9a-f-]+/attempts/[0-9]+/images$" upload_state;
 "~^POST:/api/v1/me/post-tasks/[0-9a-f-]+/attempts/[0-9]+/images/[0-9a-f-]+/upload-grant$" upload_grant;
 "~^PUT:/api/v1/me/post-tasks/[0-9a-f-]+/attempts/[0-9]+/images/[0-9a-f-]+/bytes$" image_bytes;
}
map $host $speechful_upload_entry {
 default other;
 127.0.0.1 loopback;
 OLD_HOST old_domain;
 FRP_HOST frp_domain;
}
map $http_content_length $speechful_upload_length {
 default "-";
 "~^[0-9]{1,8}$" $http_content_length;
}
map $upstream_http_x_request_id $speechful_upload_request_id {
 default "-";
 "~^[A-Za-z0-9_-]{21}[AQgw]$" $upstream_http_x_request_id;
}
'''.replace('OLD_HOST', old_host).replace('FRP_HOST', frp_host)
    header += "log_format speechful_upload escape=json '" + json.dumps(FIELDS, separators=(',', ':')) + "';\n"
    access = (' access_log syslog:server=unix:/dev/log,facility=local6,'
              'tag=' + TAG + ',severity=info,nohostname speechful_upload '
              'if=$speechful_upload_route;\n location / {')
    return original.replace('http {', 'http {' + header, 1).replace(marker, access, 1)


def install(config, nginx, backup, expected, old_host, frp_host):
    if os.geteuid() == 0 or config.is_symlink() or config.stat().st_uid != os.getuid():
        raise ValueError('Run as the existing config owner, not root')
    original = config.read_bytes()
    if hashlib.sha256(original).hexdigest() != expected:
        raise ValueError('Configuration changed; inspect before retrying')
    candidate = build_config(original.decode(), old_host, frp_host).encode()
    backup.mkdir(mode=0o700, exist_ok=False)
    (backup / 'nginx.before.conf').write_bytes(original)
    check = backup / 'nginx.candidate.conf'
    check.write_bytes(candidate)
    check.chmod(0o600)
    (backup / 'nginx.before.conf').chmod(0o600)
    cmd = [str(nginx), '-p', str(config.parent), '-c']

    def run(*args):
        subprocess.run(list(args), check=True, capture_output=True, timeout=15)

    def replace(data):
        swap = backup / 'config.swap'
        with swap.open('xb') as file:
            file.write(data)
            file.flush()
            os.fsync(file.fileno())
        swap.chmod(config.stat().st_mode & 0o777)
        os.replace(swap, config)

    run(*cmd, str(check), '-t')
    # Recheck after syntax validation, before touching the live configuration.
    if config.read_bytes() != original:
        raise RuntimeError('Concurrent configuration change; nothing applied')
    replace(candidate)
    try:
        run(*cmd, str(config), '-t')
        run(*cmd, str(config), '-s', 'reload')
    except BaseException:
        if config.read_bytes() == candidate:
            replace(original)
            run(*cmd, str(config), '-t')
            run(*cmd, str(config), '-s', 'reload')
        raise
    print(json.dumps({'result': 'PASS', 'operation': 'upload_telemetry_installed',
                      'config_sha256': hashlib.sha256(candidate).hexdigest()}))


def safe_event(value):
    if not isinstance(value, dict) or value.get('schema') != 'upload-v1':
        return None
    if value.get('route') not in ROUTES or value.get('entry') not in {'old_domain', 'frp_domain', 'loopback', 'other'}:
        return None
    event = {}
    for key in FIELDS:
        text = value.get(key)
        if not isinstance(text, str) or len(text) > 64:
            return None
        pattern = {
            'schema': r'upload-v1', 'route': r'[a-z_]+', 'entry': r'[a-z_]+',
            'time': r'[0-9T:+.-]+', 'completion': r'(?:OK|)',
            'request_id': r'(?:-|[A-Za-z0-9_-]{22})',
        }.get(key, r'[0-9., :\-]+')
        if not re.fullmatch(pattern, text):
            return None
        event[key] = text
    status = event['status']
    if status == '499':
        hint = 'CLIENT_CLOSED_CONNECTION'
    elif status in {'400', '408'} and event['completion'] != 'OK' and event['upstream_status'] == '-':
        hint = 'INCOMPLETE_REQUEST_BEFORE_BACKEND'
    elif status in {'401', '403'}:
        hint = 'AUTHORIZATION_REJECTED'
    elif status == '413':
        hint = 'REQUEST_TOO_LARGE'
    elif status == '204' and event['route'] == 'image_bytes':
        hint = 'IMAGE_UPLOAD_RECEIPT_OK'
    elif status.startswith('5'):
        hint = 'SERVER_OR_PROXY_ERROR'
    else:
        hint = 'CHECK_STATUS_AND_UPSTREAM'
    event['hint'] = hint
    return event


def report(root):
    config = root / 'proxy/nginx.conf'
    enabled = config.exists() and 'tag=' + TAG in config.read_text()
    print('UPLOAD_TELEMETRY=' + ('ENABLED' if enabled else 'NOT_CONFIGURED'))
    result = subprocess.run(['journalctl', '-t', TAG, '--since', '2 hours ago',
                             '-n', '60', '--no-pager', '-o', 'json'],
                            capture_output=True, text=True, timeout=10)
    if result.returncode:
        raise RuntimeError('Cannot read upload journal; run the logs tool with journal access')
    events = []
    for line in result.stdout.splitlines():
        try:
            message = json.loads(line)['MESSAGE']
            if not isinstance(message, str) or len(message) > 4096:
                continue
            event = safe_event(json.loads(message[message.index('{'):]))
            if event:
                events.append(event)
        except (ValueError, KeyError, TypeError):
            continue
    for event in events[-20:]:
        entry = {'old_domain': '旧域名', 'frp_domain': 'Frp 域名', 'loopback': '本机', 'other': '其他入口'}[event['entry']]
        route = {'post_create': '提交帖子', 'upload_state': '查询图片状态', 'upload_grant': '获取上传许可', 'image_bytes': '上传原图'}[event['route']]
        print('[{}] {} 入口={} HTTP={} 总耗时={}秒 后端状态={} 后端耗时={}秒 收体完成={}'.format(
            route, event['time'], entry, event['status'], event['duration_s'],
            event['upstream_status'], event['upstream_response_s'],
            '是' if event['completion'] == 'OK' else '否'))
        print(json.dumps(event, ensure_ascii=True))
    if not events:
        print('NO_RECENT_UPLOAD_EVENTS: this is not proof of successful uploads; logging starts after installation and writes when a request finishes.')
    # Preserve useful historical evidence while never printing full error lines.
    error = root / 'proxy/nginx-error.log'
    if error.exists():
        with error.open('rb') as file:
            file.seek(max(0, error.stat().st_size - 65536))
            lines = file.read().decode('utf8', 'replace').splitlines()
        for line in [line for line in lines if 'a client request body is buffered' in line and '/images/' in line][-5:]:
            host = re.search(r'host: "([a-z0-9.-]+)(?::[0-9]+)?"', line)
            print(json.dumps({'historical_time': line[:19], 'event': 'BODY_BUFFERED',
                              'host': host.group(1) if host else 'unknown',
                              'meaning': 'buffer warning alone does not prove upload success or failure'}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    read = sub.add_parser('report')
    read.add_argument('--root', type=Path, required=True)
    change = sub.add_parser('install')
    change.add_argument('--config', type=Path, required=True)
    change.add_argument('--nginx', type=Path, required=True)
    change.add_argument('--backup', type=Path, required=True)
    change.add_argument('--expected-sha256', required=True)
    change.add_argument('--old-host', required=True)
    change.add_argument('--frp-host', required=True)
    args = parser.parse_args()
    if args.action == 'report':
        report(args.root)
    else:
        install(args.config, args.nginx, args.backup, args.expected_sha256, args.old_host, args.frp_host)


if __name__ == '__main__':
    main()
