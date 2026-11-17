import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


class PerformanceGateTests(unittest.TestCase):
    def check_gate(self, mutation, expected):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for round_number in range(1, 4):
                for mode in ("legacy", "batch"):
                    value = 10.0 if mode == "legacy" else 5.0
                    data = {"Source": "1d829b2623a6c32830ee1b74fae6c99a436ad3b3" if mode == "legacy" else "candidate",
                            "Complete": True, "Warmup": 10, "Samples": 80, "Rounds": 1, "Threshold": .35, "Clock": "CLOCK_MONOTONIC_RAW",
                            "Cohorts": [{"round": round_number, "Mode": mode, "Kind": kind, "Samples": [value]*80, "P95": value, "Errors": [],
                                         "Clocks": [{"RawMS": value, "GoLowerMS": value, "GoUpperMS": value, "UncertaintyMS": 0, "Valid": True} for _ in range(90)]}
                                        for kind in ("home20", "next20", "home10", "next5", "incrementalHome10", "incrementalNext5", "incrementalRefresh5")]}
                    if mode == "batch" and round_number == 3:
                        mutation(data)
                    (root / f"{mode}-{round_number}.json").write_text(json.dumps(data))
            result = subprocess.run([sys.executable, str(Path(__file__).with_name("compare-feed-performance.py")), directory], capture_output=True)
            report = json.loads((root / "comparison.json").read_text())
            self.assertEqual(result.returncode, 0 if expected else 1, result.stderr)
            self.assertEqual(report["PASS"], expected)

    def test_rejects_instrumented_diagnostic(self):
        self.check_gate(lambda data: data.update(Diagnostic=True), False)

    def test_every_cohort_must_pass(self):
        self.check_gate(lambda _: None, True)
        def fail_one(data):
            data["Cohorts"][-1].update(Samples=[7.0]*80, P95=7.0)
            for clock in data["Cohorts"][-1]["Clocks"]:
                clock.update(RawMS=7.0, GoLowerMS=7.0, GoUpperMS=7.0)
        self.check_gate(fail_one, False)

    def test_policy_boundary(self):
        def value_at(value):
            def mutate(data):
                data["Cohorts"][-1].update(Samples=[value]*80, P95=value)
                for clock in data["Cohorts"][-1]["Clocks"]:
                    clock.update(RawMS=value, GoLowerMS=value, GoUpperMS=value)
            return mutate
        self.check_gate(value_at(6.5), True)
        self.check_gate(value_at(6.51), False)

    def test_rejects_incomplete_or_invalid_samples(self):
        self.check_gate(lambda data: data.update(Complete=False), False)
        self.check_gate(lambda data: data["Cohorts"][-1].update(P95=1.0), False)
        self.check_gate(lambda data: data["Cohorts"][-1].update(Samples=[float("nan")]*80), False)

    def test_rejects_unstable_or_inconsistent_clock_intervals(self):
        self.check_gate(lambda data: data["Cohorts"][-1]["Clocks"][0].update(Valid=False), False)
        self.check_gate(lambda data: data["Cohorts"][-1]["Clocks"][10].update(RawMS=4.0), False)
        self.check_gate(lambda data: data["Cohorts"][-1]["Clocks"][0].update(GoLowerMS=7.0, GoUpperMS=7.0), False)


if __name__ == "__main__":
    unittest.main()
