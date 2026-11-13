import unittest
from patch_host_startup_cleanup import transform


SAMPLE = '''lab.halt() # validates each owned PID identity before any signal
lab.start()
def stop(*_):
 events.append('cleanup')
 raise SystemExit(0)
signal.signal(signal.SIGTERM,stop);signal.signal(signal.SIGINT,stop)
while True:
 break
'''


class CleanupTests(unittest.TestCase):
    def run_host(self, fail):
        events = []
        class Lab:
            def halt(self):
                events.append('halt')
            def start(self):
                events.append('start')
                if fail:
                    raise RuntimeError('startup failed')
        class Signals:
            SIGTERM, SIGINT = 1, 2
            def signal(self, kind, handler):
                events.append('handler installed')
        scope = dict(lab=Lab(), signal=Signals(), events=events)
        try:
            exec(transform(SAMPLE), scope)
        except RuntimeError:
            events.append('failure propagated')
        return events

    def test_failure_cleans_up_and_still_fails(self):
        self.assertEqual(self.run_host(True), ['handler installed', 'handler installed', 'halt', 'start', 'cleanup', 'failure propagated'])

    def test_success_has_handlers_before_children_start(self):
        self.assertEqual(self.run_host(False), ['handler installed', 'handler installed', 'halt', 'start'])

    def test_unknown_or_already_patched_layout_rejected(self):
        with self.assertRaises(ValueError):
            transform('print("unrelated script")')
        with self.assertRaises(ValueError):
            transform(transform(SAMPLE))


if __name__ == '__main__':
    unittest.main()
