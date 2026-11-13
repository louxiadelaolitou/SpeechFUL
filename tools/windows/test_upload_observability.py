import json
import unittest
from upload_observability import FIELDS, build_config, safe_event


class UploadTelemetryTests(unittest.TestCase):
    def test_adds_metadata_only_and_preserves_routes_tls_and_verifier(self):
        original = 'http {\nserver {\n access_log off;\n location / { proxy_pass https://127.0.0.1:443; }\n} server {\n access_log off;\n location / { proxy_ssl_verify on; }\n} }'
        value = build_config(original, 'old.example', 'frp.example')
        self.assertIn('tag=speechful_upload', value)
        self.assertTrue(value.endswith('server {\n access_log off;\n location / { proxy_ssl_verify on; }\n} }'))
        self.assertIn('proxy_pass https://127.0.0.1:443;', value)
        line = next(line for line in value.splitlines() if line.startswith('log_format'))
        for forbidden in ('$uri', '$request"', '$remote_addr', '$http_authorization', '$request_body', '$args', '$host'):
            self.assertNotIn(forbidden, line)
        for bad in ('bad;host', 'bad\nhost', 'a.example:443'):
            with self.assertRaises(ValueError):
                build_config(original, bad, 'frp.example')
        with self.assertRaises(ValueError):
            build_config(value, 'old.example', 'frp.example')

    def event(self, **changes):
        value = dict.fromkeys(FIELDS, '-')
        value.update(schema='upload-v1', time='2026-11-13T12:00:00+08:00',
                     entry='frp_domain', route='image_bytes', completion='',
                     status='400', duration_s='15.001', request_length='123')
        value.update(changes)
        return value

    def test_distinguishes_incomplete_body_authorization_and_success(self):
        self.assertEqual(safe_event(self.event())['hint'], 'INCOMPLETE_REQUEST_BEFORE_BACKEND')
        self.assertEqual(safe_event(self.event(status='499'))['hint'], 'CLIENT_CLOSED_CONNECTION')
        self.assertEqual(safe_event(self.event(status='403', completion='OK', upstream_status='403'))['hint'], 'AUTHORIZATION_REJECTED')
        self.assertEqual(safe_event(self.event(status='204', completion='OK', upstream_status='204'))['hint'], 'IMAGE_UPLOAD_RECEIPT_OK')

    def test_untrusted_or_extra_values_never_escape_report(self):
        for changes in ({'request_id': 'secret\nvalue'}, {'entry': 'https://private'},
                        {'route': '/api/v1/private'}, {'status': 'body-secret'},
                        {'time': 'private'}, {'completion': 'unknown'}):
            self.assertIsNone(safe_event(self.event(**changes)))
        event = self.event()
        event['body'] = 'never-output'
        self.assertNotIn('never-output', json.dumps(safe_event(event)))


if __name__ == '__main__':
    unittest.main()
