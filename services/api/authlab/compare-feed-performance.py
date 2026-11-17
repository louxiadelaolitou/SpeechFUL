#!/usr/bin/env python3
"""Hard gate for every frozen-baseline/current endpoint cohort; keep all samples."""
import json
import math
from pathlib import Path
import sys

BASELINE = "1d829b2623a6c32830ee1b74fae6c99a436ad3b3"
KINDS = ("home20", "next20", "home10", "next5", "incrementalHome10", "incrementalNext5", "incrementalRefresh5")
root = Path(sys.argv[1])
report = {"Baseline": BASELINE, "Threshold": .35, "Cohorts": [], "Comparisons": [], "Complete": False, "PASS": False}


def require(condition, message):
    if not condition:
        raise ValueError(message)


try:
    indexed = {}
    for round_number in range(1, 4):
        for mode in ("legacy", "batch"):
            data = json.loads((root / f"{mode}-{round_number}.json").read_text())
            require(not data.get("Diagnostic", False), "diagnostic samples cannot be used for acceptance")
            require(data["Complete"] and data["Warmup"] == 10 and data["Samples"] == 80 and data["Rounds"] == 1, "incomplete or changed measurement protocol")
            require(data["Threshold"] == .35 and len(data["Cohorts"]) == len(KINDS), "changed threshold or cohort count")
            require(data["Clock"] == "CLOCK_MONOTONIC_RAW", "wrong endpoint interval clock")
            require(data["Source"] and (mode != "legacy" or data["Source"] == BASELINE), "wrong source provenance")
            if mode == "batch":
                report.setdefault("CurrentSource", data["Source"])
                require(report["CurrentSource"] == data["Source"], "candidate changed between rounds")
            for c in data["Cohorts"]:
                require(c["round"] == round_number and c["Mode"] == mode and c["Kind"] in KINDS and not c["Errors"], "invalid or failed cohort")
                samples = c["Samples"]
                clocks = c["Clocks"]
                require(len(clocks) == 90 and all(clock["Valid"] for clock in clocks), "invalid or missing clock intervals")
                for clock in clocks:
                    raw, lower, upper = clock["RawMS"], clock["GoLowerMS"], clock["GoUpperMS"]
                    tolerance = max(.05, .01 * raw)
                    require(all(math.isfinite(v) for v in (raw, lower, upper)) and raw > 0 and lower > 0 and upper >= lower and upper-lower <= tolerance and lower-tolerance <= raw <= upper+tolerance, "clock bracket disagrees with RAW")
                    require(clock["UncertaintyMS"] == upper-lower, "incorrect clock uncertainty")
                require(samples == [clock["RawMS"] for clock in clocks[10:]], "endpoint samples differ from RAW intervals")
                require(len(samples) == 80 and all(math.isfinite(v) and v > 0 for v in samples), "invalid samples")
                require(c["P95"] == sorted(samples)[math.ceil(.95 * len(samples)) - 1], "p95 does not match raw samples")
                key = (round_number, c["Kind"], mode)
                require(key not in indexed, "duplicate cohort")
                indexed[key] = c["P95"]
                report["Cohorts"].append(dict(c, Source=data["Source"]))
    for round_number in range(1, 4):
        for kind in KINDS:
            old = indexed[round_number, kind, "legacy"]
            new = indexed[round_number, kind, "batch"]
            gain = 1 - new / old
            result = {"round": round_number, "Kind": kind, "LegacyP95": old, "BatchP95": new, "Improvement": gain, "PASS": gain >= .35}
            report["Comparisons"].append(result)
            print(f"round{round_number} {kind}: {old:.3f} -> {new:.3f} ms, {gain:.2%}, {'PASS' if result['PASS'] else 'FAIL'}")
    report["Complete"] = True
    report["PASS"] = all(c["PASS"] for c in report["Comparisons"])
except Exception as error:
    report["Error"] = f"{type(error).__name__}: {error}"
    print(report["Error"], file=sys.stderr)
finally:
    with (root / "comparison.json").open("x") as output:
        json.dump(report, output, indent=2)
sys.exit(0 if report["PASS"] else 1)
