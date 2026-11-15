#!/usr/bin/env python3
"""Frozen comment/feed/media comparison; no external database or retry.

The baseline is fixed product 69d2adc with only the listed observation/harness
files overlaid. New product is an exact archive of the requested commit. Logs,
fixtures and raw JSON stay private; publication uses a fresh field whitelist.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import platform
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import subprocess
import sys
import tarfile
import time

BASELINE = "69d2adcf4b153601ba218de87bb6efccd91da5c8"
INSTRUMENTATION_COMMIT = "e8aeadd5134c0d1d2d6a5fc7cf9c9fca4059f832"
INSTRUMENTATION_SHA256 = {
    "services/api/internal/authprivacy/common.go": "cde78f3abb1f0cc96907c6b5d5486818f080b33f6b5d2828543fbcf8bc628418",
    "services/api/internal/authprivacy/authorization_gate.go": "82da22126427e9f61e35fc905c1738667e52fe6385a6b2cbf7a6dc1944afcc29",
    "services/api/internal/authprivacy/read_timing.go": "d724d3f48b24b62ef144dcc9547b3507815db5589afc44eaee68eecd2150a455",
    "services/api/internal/readtiming/readtiming.go": "83fa1fde6d5a9ecde2c530bd109690b269d106e3dee6c5e8d1fe9bc35bc2ad0c",
}
OVERLAY = (
    "services/api/internal/authprivacy/common.go",
    "services/api/internal/authprivacy/authorization_gate.go",
    "services/api/internal/authprivacy/read_timing.go",
    "services/api/internal/readtiming/readtiming.go",
    "services/api/internal/authprivacy/community_read_baseline_postgres_test.go",
    "services/api/authlab/run-read-critical-section-isolated.sh",
)
MAX_DRIFT_NS = 250_000_000
KINDS = ("comments", "feed", "image", "like")
PREPARED_FLAGS = ("preparedReads", "preparedFeedReads", "preparedMediaReads")
SHA = re.compile(r"[0-9a-f]{40}\Z")
STAGE = re.compile(r"[a-z_]+(?:\.[a-z_]+)*\Z")
ERROR_CODE = re.compile(r"[A-Z][A-Z0-9_]{0,79}\Z")
TRANSACTION_SETTINGS = {"deadlock_timeout":"1s","lock_timeout":"5s","statement_timeout":"10s"}


class ComparisonError(Exception):
    """The string is a fixed, safe diagnostic category, never child output."""


class InterruptedComparison(Exception):
    def __init__(self, signum):
        self.signum = signum


def commit_sha(value):
    if not SHA.fullmatch(value):
        raise ComparisonError("commit_must_be_full_lowercase_sha")
    return value


def inside(path, parent):
    return path == parent or parent in path.parents


def validate_paths(source, output):
    source = Path(source).resolve(strict=True)
    proposed = Path(output)
    if not proposed.is_absolute():
        raise ComparisonError("output_must_be_absolute")
    output = proposed.resolve()
    if output.exists() or proposed.is_symlink():
        raise ComparisonError("output_already_exists")
    if inside(output, source) or inside(source, output):
        raise ComparisonError("output_must_be_outside_source")
    if not output.parent.is_dir():
        raise ComparisonError("output_parent_missing")
    return source, output


def git(source, *args):
    result = subprocess.run(["git", "-C", str(source), *args], capture_output=True)
    if result.returncode:
        raise ComparisonError("git_operation_failed")
    return result.stdout.decode().strip()


def verify_repository(source, head):
    if Path(git(source, "rev-parse", "--show-toplevel")).resolve() != source:
        raise ComparisonError("source_must_be_git_root")
    for sha in (BASELINE, commit_sha(head)):
        if git(source, "rev-parse", "--verify", sha + "^{commit}") != sha:
            raise ComparisonError("commit_not_available")


def write_json(path, value):
    with path.open("x", encoding="utf-8") as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    path.chmod(0o600)


def extract_archive(archive, destination):
    """Reject traversal, devices and hardlinks; create safe symlinks last."""
    destination = destination.resolve()
    links = []
    with tarfile.open(archive) as stream:
        for member in stream:
            name = PurePosixPath(member.name)
            if name.is_absolute() or ".." in name.parts or "\\" in member.name:
                raise ComparisonError("unsafe_archive_path")
            target = destination.joinpath(*name.parts)
            if not inside(target.resolve(), destination):
                raise ComparisonError("archive_escape")
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
            elif member.isfile():
                target.parent.mkdir(parents=True, exist_ok=True)
                with target.open("xb") as output, stream.extractfile(member) as content:
                    shutil.copyfileobj(content, output)
                target.chmod(member.mode & 0o755)
            elif member.issym():
                link = PurePosixPath(member.linkname)
                if link.is_absolute() or "\\" in member.linkname or not inside((target.parent / member.linkname).resolve(), destination):
                    raise ComparisonError("unsafe_archive_link")
                links.append((target, member.linkname))
            else:
                raise ComparisonError("unsupported_archive_entry")
    created = []
    for target, link in links:
        try:
            if not inside(target.parent.resolve(), destination):
                raise ComparisonError("archive_link_parent_escape")
        except RuntimeError:
            raise ComparisonError("archive_link_cycle")
        target.parent.mkdir(parents=True, exist_ok=True)
        if target.exists() or target.is_symlink():
            raise ComparisonError("duplicate_archive_link")
        os.symlink(link, target)
        created.append(target)
        # A link which looked internal before another link existed may become
        # an escape afterwards (a->., b->a/../outside). Check the whole graph
        # after each insertion, before any later parent directory is created.
        for existing in created:
            try:
                if not inside(existing.resolve(),destination):
                    raise ComparisonError("archive_link_graph_escape")
            except RuntimeError:
                raise ComparisonError("archive_link_cycle")


def archive_tree(source, sha, destination, archive):
    destination.mkdir(mode=0o700)
    with archive.open("xb") as stream:
        result = subprocess.run(["git", "-C", str(source), "archive", commit_sha(sha)], stdout=stream, stderr=subprocess.PIPE)
    if result.returncode:
        raise ComparisonError("git_archive_failed")
    extract_archive(archive, destination)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def overlay_observation(old, new, paths=OVERLAY):
    if len(set(paths)) != len(paths) or any(path not in OVERLAY for path in paths):
        raise ComparisonError("overlay_not_whitelisted")
    records = []
    for relative in paths:
        source, target = new / relative, old / relative
        if source.is_symlink() or not source.is_file() or target.is_symlink():
            raise ComparisonError("overlay_requires_regular_files")
        if not inside(source.resolve(), new.resolve()) or not inside(target.resolve(), old.resolve()):
            raise ComparisonError("overlay_escape")
        if relative in INSTRUMENTATION_SHA256 and digest(source) != INSTRUMENTATION_SHA256[relative]:
            raise ComparisonError("instrumentation_content_not_frozen")
        before = digest(target) if target.exists() else None
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
        target.chmod(0o600)
        records.append({"path": relative, "baselineSHA256": before, "headSHA256": digest(source), "appliedSHA256": digest(target)})
    return records


def tree_hashes(root):
    result = {}
    for path in sorted(root.rglob("*")):
        if path.is_symlink():
            result[path.relative_to(root).as_posix()] = {"kind": "symlink", "sha256": hashlib.sha256(os.readlink(path).encode()).hexdigest()}
        elif path.is_file():
            result[path.relative_to(root).as_posix()] = {"kind": "file", "sha256": digest(path)}
    return result


def profiles():
    definitions = [("main-r" + str(n), "main", 1000, 80, 10, False, "") for n in range(1, 4)]
    definitions += [("fresh-" + mode, "fresh", 1000, 40, 4, True, mode) for mode in ("relevant", "likes")]
    definitions += [("size-" + str(size), "scale", size, 8, 1, False, "") for size in (50, 10000)]
    return [dict(label=name + "-" + side, pair=name, category=category, side=side, comments=size, samples=samples,
                 warmup=warmup, fresh=fresh, commentLike=fresh, sort=mode, prepared=side == "new")
            for name, category, size, samples, warmup, fresh, mode in definitions for side in ("old", "new")]


def integer(value):
    return type(value) is int and value >= 0


def quantiles(values):
    values = sorted(values)
    return {"p" + str(p) + "NS": values[math.ceil(p / 100 * len(values)) - 1] for p in (50, 95, 99)} if values else {}


def public_sample(sample):
    if not isinstance(sample,dict):sample={}
    result = {key: sample[key] for key in ("index", "elapsedNS", "wallStartNS", "wallEndNS", "wallElapsedNS", "rawElapsedNS", "wallGoDriftNS", "rawGoDriftNS")
              if type(sample.get(key)) is int}
    result["kind"] = sample.get("kind") if sample.get("kind") in KINDS else "unknown"
    result["clockStable"] = sample.get("clockStable") is True
    result["requestFailed"] = bool(sample.get("error"))
    if ERROR_CODE.fullmatch(str(sample.get("errorCode", ""))):
        result["errorCode"] = sample["errorCode"]
    if integer(sample.get("errorStatus")) and sample["errorStatus"] <= 599:
        result["errorStatus"] = sample["errorStatus"]
    observation = sample.get("observation", {})
    if not isinstance(observation,dict):observation={}
    stages=observation.get("stages",{})
    if not isinstance(stages,dict):stages={}
    result["observation"] = {
        "clockValid": observation.get("clockValid") is True,
        "truncated": observation.get("truncated") is True,
        "stages": {name: {key: stage[key] for key in ("nanoseconds", "calls") if integer(stage.get(key))}
                   for name, stage in stages.items() if isinstance(name,str) and STAGE.fullmatch(name) and isinstance(stage, dict)},
    }
    return result


def frozen_configuration(spec):
    return {"comments":spec["comments"],"commentLikeRows":min(spec["comments"],1000),"sampleWaves":spec["samples"],
            "warmupWaves":spec["warmup"],"poolMaxConns":12,"concurrency":4,"goMaxProcs":4,"posts":20,
            "pageSize":50,"feedPageSize":20,"freshNonce":spec["fresh"],"commentSort":spec["sort"],
            "mix":"comments:feed:image:like=1:1:1:1","snapshotBackend":"redis",
            "likeTarget":"comment-like-same-post" if spec["commentLike"] else "post-like-same-post",
            "goVersion":"go1.22.2", **{key:spec["prepared"] for key in PREPARED_FLAGS}}


def public_configuration(data, spec):
    result = {}
    for key, expected in frozen_configuration(spec).items():
        value=data.get(key)
        if type(expected) in (int,bool):
            result[key]=value if type(value) is type(expected) else "invalid_type"
        elif key=="goVersion":
            result[key]=value if re.fullmatch(r"go[0-9]+\.[0-9]+(?:\.[0-9]+)?",str(value)) else "unrecognized"
        else:
            allowed={expected}
            if key=="snapshotBackend":allowed|={"redis","postgres"}
            if key=="commentSort":allowed|={"","relevant","recent","likes"}
            if key=="likeTarget":allowed|={"comment-like-same-post","post-like-same-post"}
            result[key]=value if isinstance(value,str) and value in allowed else "unrecognized"
    settings=data.get("transactionSettings",{})
    if not isinstance(settings,dict):settings={}
    result["transactionSettings"]={key:settings.get(key) if re.fullmatch(r"[0-9]+(?:\.[0-9]+)?(?:ms|s|min|h|d)?",str(settings.get(key))) else "unrecognized" for key in TRANSACTION_SETTINGS}
    return result


def evaluate_cohort(data, spec, returncode, expected_source=None):
    reasons = []
    if returncode:
        reasons.append("cohort_process_failed")
    if data.get("result") != "PASS" or type(data.get("failures")) is not int or data.get("failures") != 0:
        reasons.append("cohort_reported_failure")
    clock = data.get("cohortClock", {})
    if data.get("environmentValid") is not True or data.get("environmentResult") != "PASS" or any(
        not integer(clock.get(key)) for key in ("wallElapsedNS", "goElapsedNS", "rawElapsedNS")):
        reasons.append("environment_invalid")
    elif max(clock["wallElapsedNS"], clock["goElapsedNS"], clock["rawElapsedNS"]) - min(clock["wallElapsedNS"], clock["goElapsedNS"], clock["rawElapsedNS"]) > MAX_DRIFT_NS:
        reasons.append("cohort_clock_drift")
    for flag in PREPARED_FLAGS:
        if data.get(flag) is not spec["prepared"]:
            reasons.append("wrong_"+flag)
    if expected_source is not None and data.get("sourceCommit") != expected_source:
        reasons.append("wrong_source_commit")
    if type(data.get("warmupFailures")) is not list or data.get("warmupFailures"):
        reasons.append("warmup_failed")
    if any(type(data.get(key)) is not type(expected) or data.get(key)!=expected for key,expected in frozen_configuration(spec).items()):
        reasons.append("wrong_frozen_configuration")
    if data.get("transactionSettings")!=TRANSACTION_SETTINGS:
        reasons.append("wrong_transaction_settings")
    raw_samples=data.get("samples",[])
    if not isinstance(raw_samples,list):raw_samples=[];reasons.append("samples_not_list")
    if len(raw_samples)!=4*spec["samples"]:reasons.append("wrong_total_sample_count")
    samples = [public_sample(sample) for sample in raw_samples]
    # Judge every request before grouping. Unknown kinds must not disappear
    # from the four accepted groups and silently hide failures or clock drift.
    for sample in samples:
        if sample["kind"] not in KINDS:reasons.append("unknown_sample_kind")
        if sample["requestFailed"]:reasons.append("request_failed")
        if not sample["clockStable"] or not sample["observation"]["clockValid"] or sample["observation"]["truncated"]:
            reasons.append("invalid_sample_clock_or_trace")
        times=[sample.get(key) for key in ("elapsedNS","wallElapsedNS","rawElapsedNS")]
        if not all(integer(value) for value in times) or max(times)-min(times)>MAX_DRIFT_NS:reasons.append("sample_clock_drift")
    metrics = {}
    for kind in KINDS:
        selected = [sample for sample in samples if sample["kind"] == kind]
        if sorted(sample.get("index", -1) for sample in selected) != list(range(spec["samples"])):
            reasons.append("missing_or_duplicate_samples_" + kind)
        elapsed, holds = [], []
        conflicts = rebuilds = 0
        for sample in selected:
            observation = sample["observation"]
            stages = observation["stages"]
            if sample["requestFailed"] or not observation["clockValid"] or observation["truncated"] or not sample["clockStable"]:
                reasons.append("invalid_request_" + kind)
            times = [sample.get(key) for key in ("elapsedNS", "wallElapsedNS", "rawElapsedNS")]
            if integer(sample.get("elapsedNS")):
                elapsed.append(sample["elapsedNS"])
            if not all(integer(value) for value in times) or max(times) - min(times) > MAX_DRIFT_NS:
                reasons.append("request_clock_drift_" + kind)
            if not integer(sample.get("wallStartNS")) or not integer(sample.get("wallEndNS")) or sample.get("wallEndNS",0)-sample.get("wallStartNS",0) != sample.get("wallElapsedNS"):
                reasons.append("inconsistent_wall_bounds_" + kind)
            hold = sum(stage.get("nanoseconds", 0) for name, stage in stages.items() if name.endswith("gate_hold"))
            hold_calls = sum(stage.get("calls", 0) for name, stage in stages.items() if name.endswith("gate_hold"))
            anchors = sum(stage.get("calls", 0) for name, stage in stages.items() if name.endswith("anchor_write"))
            if not hold_calls or hold <= 0 or anchors != 1 or any(name.endswith("gate_hold_boundary_uncertain") for name in stages):
                reasons.append("invalid_transaction_boundary_" + kind)
            if kind in ("comments", "feed", "image") and spec["prepared"] and not any(
                name.endswith("prepare_read") and stage.get("calls",0)>0 for name,stage in stages.items()
            ):
                reasons.append("prepared_path_not_observed_"+kind)
            holds.append(hold)
            conflicts += sum(stage.get("calls", 0) for name, stage in stages.items() if name.endswith("prepared_read_conflict"))
            rebuilds += sum(stage.get("calls", 0) for name, stage in stages.items() if name.endswith("prepared_read_rebuild"))
        metrics[kind] = {"elapsed": quantiles(elapsed), "gateHoldPerRequest": quantiles(holds), "readConflicts": conflicts, "readRebuilds": rebuilds,
                         "requestFailures": sum(sample["requestFailed"] for sample in selected)}
    return {"label": spec["label"], "spec": spec, "result": "FAIL" if reasons else "PASS", "reasons": sorted(set(reasons)),
            "actualConfiguration":public_configuration(data,spec),
            "metrics": metrics, "samples": samples, "warmupFailures": [public_sample(sample) for sample in data.get("warmupFailures", [])],
            "cohortClock": {key: value for key, value in clock.items() if type(value) is int},
            "sourceCommit": data.get("sourceCommit") if SHA.fullmatch(str(data.get("sourceCommit", ""))) else "unknown"}


def compare_pair(old, new, enforce):
    result = {"pair": old["spec"]["pair"], "category": old["spec"]["category"], "hardThresholdsEnforced": enforce}
    if old["result"] != "PASS" or new["result"] != "PASS":
        return dict(result, result="FAIL", reason="cohort_not_passed")
    try:
        hold_old, hold_new = (item["metrics"]["comments"]["gateHoldPerRequest"]["p95NS"] for item in (old, new))
        feed_old, feed_new = (item["metrics"]["feed"]["elapsed"]["p95NS"] for item in (old, new))
        if min(hold_old, feed_old) <= 0:
            raise ValueError
    except (KeyError, ValueError):
        return dict(result, result="FAIL", reason="missing_positive_reference")
    # Integer comparisons avoid rounding a 49.999% improvement into a pass.
    passed = hold_new * 100 <= hold_old * 50 and feed_new * 100 <= feed_old * 70
    return dict(result, result="PASS" if passed or not enforce else "FAIL", oldGateHoldP95NS=hold_old, newGateHoldP95NS=hold_new,
                oldFeedP95NS=feed_old, newFeedP95NS=feed_new, holdReductionPercent=(hold_old-hold_new)*100/hold_old,
                feedReductionPercent=(feed_old-feed_new)*100/feed_old, thresholdsMet=passed)


def stop_child(child):
    # The shell leader may already have exited while Redis/Go descendants
    # remain. Its session/group was created by us, so always address the group.
    try:
        os.killpg(child.pid,signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:child.wait(timeout=30)
    except subprocess.TimeoutExpired:pass
    deadline=time.monotonic()+1
    while time.monotonic()<deadline:
        try:os.killpg(child.pid,0)
        except ProcessLookupError:return
        time.sleep(.02)
    try:os.killpg(child.pid,signal.SIGKILL)
    except ProcessLookupError:pass
    child.wait(timeout=10)


def public_artifacts(directory):
    names = {"manifest.json", "comparison-summary.json"} | {"cohort-" + spec["label"] + ".json" for spec in profiles()}
    selected = []
    for name in sorted(names):
        path = directory / name
        if path.is_symlink():
            raise ComparisonError("publication_symlink")
        if path.is_file():
            selected.append(path)
    return selected


def child_environment(spec, head, output, media, python, redis):
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(("PG", "AUTHLAB_", "AUTHRUNTIME_", "REQ001_", "POST_MEDIA_", "COMMENT_SNAPSHOT_"))}
    env.update(GOMAXPROCS="4", GOTOOLCHAIN="local", GOWORK="off", GOFLAGS="-mod=readonly", PGPORT="55531",
               POST_MEDIA_PYTHON=str(python), POST_MEDIA_FIXTURES=str(media), COMMENT_SNAPSHOT_REDIS_SERVER=str(redis),
               REQ001_BASELINE_SOURCE=head if spec["prepared"] else BASELINE,
               REQ001_BASELINE_LABEL=spec["label"], REQ001_BASELINE_PREPARED=str(int(spec["prepared"])),
               REQ001_BASELINE_COMMENTS=str(spec["comments"]), REQ001_BASELINE_LIKES=str(min(spec["comments"],1000)),
               REQ001_BASELINE_SAMPLES=str(spec["samples"]), REQ001_BASELINE_WARMUP=str(spec["warmup"]),
               REQ001_BASELINE_FRESH=str(int(spec["fresh"])), REQ001_BASELINE_COMMENT_LIKE=str(int(spec["commentLike"])),
               REQ001_BASELINE_SORT=spec["sort"], REQ001_BASELINE_OUTPUT=str(output))
    return env


def run_cohort(tree, spec, head, private, media, python, redis):
    raw = private / (spec["label"] + ".json")
    env = child_environment(spec, head, raw, media, python, redis)
    with (private / (spec["label"] + ".private.log")).open("xb") as log:
        child = subprocess.Popen(["sh", "authlab/run-read-critical-section-isolated.sh"], cwd=tree/"services/api", env=env,
                                 stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        termination="completed"
        cleanup_failed=False
        try:
            code = child.wait(timeout=720)
        except subprocess.TimeoutExpired:
            termination="timeout";code=124
        except InterruptedComparison as error:
            termination="interrupted";code=128+error.signum
        finally:
            try:stop_child(child)
            except (OSError,subprocess.TimeoutExpired):cleanup_failed=True;code=code if termination!="completed" else 1
    try:
        data = json.loads(raw.read_text()) if raw.is_file() and not raw.is_symlink() else {}
    except (ValueError, OSError):
        data = {}
    report=evaluate_cohort(data,spec,code,head if spec["prepared"] else BASELINE)
    report["termination"]=termination
    report["processExitCode"]=code
    if cleanup_failed:
        report["result"]="FAIL"
        report["reasons"].append("owned_group_cleanup_failed")
    if termination!="completed":
        report["result"]="FAIL"
        report["reasons"].append("cohort_"+termination+"_no_retry")
    return report


def compile_trees(old, new, private):
    env = child_environment(profiles()[0], BASELINE, private/"unused.json", private/"unused-media", Path("/usr/bin/python3"), Path("/usr/bin/redis-server"))
    # No fixture or test is executed. Keep all DSNs absent even if the parent
    # happens to be a developer shell with database configuration.
    env = {key:value for key,value in env.items() if not key.startswith(("PG", "AUTHLAB_", "REQ001_", "POST_MEDIA_", "COMMENT_SNAPSHOT_"))}
    for label, tree in (("old",old),("new",new)):
        with (private/(label+"-compile.private.log")).open("xb") as log:
            child=subprocess.Popen(["go","test","-mod=readonly","-run","^$","./internal/authprivacy"],cwd=tree/"services/api",env=env,
                                   stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
            try:
                if child.wait(timeout=600):raise ComparisonError("compile_failed")
            finally:stop_child(child)


def executable_path(value):
    # Preserve a venv interpreter and Redis multicall symlink's argv[0].
    # resolve() would silently turn venv/bin/python into the system Python.
    if value is None or not value.is_absolute() or not value.is_file() or not os.access(value,os.X_OK):
        raise ComparisonError("absolute_executable_required")
    return value


def tool_versions(python,redis,prepare_only):
    result={"runnerOS":os.environ.get("RUNNER_OS",platform.system()),"runnerArch":os.environ.get("RUNNER_ARCH",platform.machine())}
    if result["runnerOS"]!="Linux" or result["runnerArch"] not in ("X64","x86_64","AMD64"):
        raise ComparisonError("unexpected_runner_platform")
    def read(command,pattern):
        raw=subprocess.run(command,capture_output=True,check=True,timeout=15).stdout.decode().strip()
        match=re.search(pattern,raw)
        if not match:raise ComparisonError("unexpected_tool_version")
        return match.group(1)
    result["goVersion"]=read(["go","version"],r"^go version (go1\.22\.2) linux/amd64$")
    if not prepare_only:
        result["postgresVersion"]=read(["pg_ctl","--version"],r"^pg_ctl \(PostgreSQL\) (16\.[0-9]+)")
        result["redisVersion"]=read([str(redis),"--version"],r"^Redis server v=(7\.[0-9]+\.[0-9]+)")
        result["pillowVersion"]=read([str(python),"-c","import PIL;print(PIL.__version__)"],r"^(12\.3\.0)$")
    return result


def run(args):
    head = commit_sha(args.head)
    source, output = validate_paths(args.source, args.output)
    verify_repository(source, head)
    if sys.platform != "linux" or os.getuid() == 0:
        raise ComparisonError("non_root_linux_runner_required")
    python = executable_path(args.python) if not args.prepare_only else None
    redis = executable_path(args.redis) if not args.prepare_only else None
    output.mkdir(mode=0o700)
    public, private = output/"public", output/"private"
    public.mkdir(mode=0o700); private.mkdir(mode=0o700)
    reports, pairs = [], []
    summary = {"result": "NOT_RUN", "scope": "M1 comment, M2 feed and M3 media prepared reads", "headCommit": head, "baselineProductCommit": BASELINE,
               "declaredOrder": [spec["label"] for spec in profiles()], "hardThresholds": {"mainGateHoldP95ReductionPercent": 50, "otherUserFeedP95ReductionPercent": 30},
               "freshAndScale": "reported separately; scale has eight samples and is not sufficient p95 acceptance", "cohorts": reports, "pairs": pairs,
               "performanceResult":"NOT_RUN","prepareOnly":args.prepare_only}
    exit_code = 1
    try:
        summary["phase"]="tool_versions"
        versions=tool_versions(python,redis,args.prepare_only)
        summary["toolVersions"]=versions
        old, new = private/"old", private/"new"
        summary["phase"] = "archive"
        archive_tree(source, BASELINE, old, private/"old.tar")
        archive_tree(source, head, new, private/"new.tar")
        summary["phase"] = "observation_overlay"
        overlay = overlay_observation(old, new)
        manifest = {"headCommit": head, "baselineProductCommit": BASELINE, "baselineOverlay": overlay, "overlayWhitelist": list(OVERLAY),
                    "oldTreeFiles": tree_hashes(old), "newTreeFiles": tree_hashes(new), "declaredProfiles": profiles(),
                    "configuration": {"poolMaxConns":12,"goMaxProcs":4,"posts":20,"commentPageSize":50,"feedPageSize":20,
                                      "mix":"comments:feed:image:like=1:1:1:1","clockMaximumDriftNS":MAX_DRIFT_NS},
                    "oldTreeLabel":"fixed product plus observation/harness whitelist; not an unmodified baseline commit"}
        manifest["comparisonRunnerSHA256"] = digest(Path(__file__))
        manifest["toolVersions"]=versions
        manifest["approvedInstrumentationCommit"]=INSTRUMENTATION_COMMIT
        manifest["approvedInstrumentationSHA256"]=INSTRUMENTATION_SHA256
        write_json(public/"manifest.json", manifest)
        summary["phase"] = "compile_both_trees"
        compile_trees(old,new,private)
        summary["compileResult"] = "PASS"
        if args.prepare_only:
            summary.update(result="PASS",reason="preparation_and_compile_only")
            exit_code=0
            return exit_code
        media = private/"media"
        summary["phase"] = "owned_media_fixture"
        with (private/"media-fixtures.private.log").open("xb") as log:
            subprocess.run([str(python), "media/test_validator.py", "--fixtures", str(media)], cwd=new/"services/api", stdout=log, stderr=subprocess.STDOUT, check=True)
        summary["phase"] = "cohorts"
        for spec in profiles():
            report = run_cohort(new if spec["prepared"] else old, spec, head, private, media, python, redis)
            reports.append({key:value for key,value in report.items() if key not in ("samples","warmupFailures")})
            write_json(public/("cohort-"+spec["label"]+".json"), report)
            print(spec["label"] + " " + report["result"], flush=True)
            if report["result"] != "PASS":
                summary.update(result="FAIL", reason="cohort_failed_no_retry")
                if report.get("termination")=="interrupted":summary["interruptedExitCode"]=report["processExitCode"]
                break
        for pair in dict.fromkeys(spec["pair"] for spec in profiles()):
            matched = [report for report in reports if report["spec"]["pair"] == pair]
            if len(matched) == 2:
                pairs.append(compare_pair(matched[0], matched[1], matched[0]["spec"]["category"] == "main"))
        if len(reports) == len(profiles()) and all(report["result"]=="PASS" for report in reports):
            summary["result"] = "PASS" if all(pair["result"]=="PASS" for pair in pairs) else "FAIL"
        summary["performanceResult"] = summary["result"]
        summary["phase"] = "complete" if len(reports)==len(profiles()) else "cohort_failed"
        summary["notRun"] = [spec["label"] for spec in profiles() if spec["label"] not in {report["label"] for report in reports}]
        exit_code = 0 if summary["result"] == "PASS" else summary.get("interruptedExitCode",1)
    except InterruptedComparison as error:
        summary.update(result="FAIL", reason="interrupted_no_retry")
        exit_code = 128 + error.signum
    except subprocess.TimeoutExpired:
        summary.update(result="FAIL", reason="cohort_timeout_no_retry")
    except Exception:
        summary.update(result="FAIL", reason="comparison_setup_or_data_invalid")
    finally:
        summary["notRun"] = [spec["label"] for spec in profiles() if spec["label"] not in {report["label"] for report in reports}]
        write_json(public/"comparison-summary.json", summary)
        selected = public_artifacts(public)
        if os.environ.get("GITHUB_OUTPUT"):
            with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as stream:
                stream.write("artifact_paths<<READ_SECTION_ARTIFACTS\n" + "\n".join(str(path) for path in selected) + "\nREAD_SECTION_ARTIFACTS\n")
    return exit_code


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--head", required=True)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--python", type=Path)
    parser.add_argument("--redis", type=Path)
    parser.add_argument("--prepare-only",action="store_true",help="archive, verify overlay and compile both trees; no SQL/performance")
    args = parser.parse_args()
    def interrupted(signum, _frame):
        raise InterruptedComparison(signum)
    for signum in (signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, interrupted)
    try:
        return run(args)
    except (ComparisonError, OSError):
        print("Comparison configuration rejected; no remote operations performed.", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
