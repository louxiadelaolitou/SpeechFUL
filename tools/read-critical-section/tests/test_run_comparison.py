import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tarfile
import tempfile
import time
import unittest
from unittest import mock


SCRIPT = Path(__file__).resolve().parents[1] / "run_comparison.py"
SPEC = importlib.util.spec_from_file_location("read_comparison", SCRIPT)
comparison = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(comparison)


def good_data(spec):
    samples = []
    for index in range(spec["samples"]):
        for kind in comparison.KINDS:
            stages={kind+".gate_hold":dict(nanoseconds=80,calls=2),kind+".anchor_write":dict(nanoseconds=10,calls=1)}
            if spec["prepared"] and kind in ("comments","feed","image"):
                stages[kind+".prepare_read"]=dict(nanoseconds=20,calls=1)
            samples.append(dict(kind=kind,index=index,elapsedNS=100,wallStartNS=1000,wallEndNS=1100,wallElapsedNS=100,rawElapsedNS=100,
                                wallGoDriftNS=0,rawGoDriftNS=0,clockStable=True,
                                observation=dict(clockValid=True,truncated=False,stages=stages)))
    value=dict(result="PASS",failures=0,environmentValid=True,environmentResult="PASS",preparedReads=spec["prepared"],
                comments=spec["comments"],sampleWaves=spec["samples"],warmupWaves=spec["warmup"],poolMaxConns=12,concurrency=4,
                freshNonce=spec["fresh"],commentSort=spec["sort"],warmupFailures=[],samples=samples,sourceCommit=comparison.BASELINE,
                cohortClock=dict(wallElapsedNS=1000,goElapsedNS=1000,rawElapsedNS=1000))
    value.update(comparison.frozen_configuration(spec))
    value["transactionSettings"]=comparison.TRANSACTION_SETTINGS.copy()
    return value


class ComparisonTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def test_commit_and_output_paths_are_strict(self):
        self.assertEqual(comparison.BASELINE, comparison.commit_sha(comparison.BASELINE))
        for value in ("HEAD", "-h", "a"*39, "A"*40, "f"*40+"\n", "f"*40+";echo"):
            with self.subTest(value=value), self.assertRaises(comparison.ComparisonError):
                comparison.commit_sha(value)
        source=self.root/"source";source.mkdir()
        self.assertEqual((source.resolve(),self.root/"out"),comparison.validate_paths(source,self.root/"out"))
        for output in (Path("relative"),source/"inside",source,self.root):
            with self.subTest(output=output),self.assertRaises(comparison.ComparisonError):
                comparison.validate_paths(source,output)

    def test_overlay_copies_only_whitelisted_observation_not_product(self):
        old,new=self.root/"old",self.root/"new";old.mkdir();new.mkdir()
        for name in comparison.OVERLAY:
            path=new/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_text("observation")
        product=new/"services/api/internal/authprivacy/community_comments_prepared.go";product.write_text("new product")
        approved={path:comparison.digest(new/path) for path in comparison.INSTRUMENTATION_SHA256}
        with mock.patch.dict(comparison.INSTRUMENTATION_SHA256,approved,clear=True):
            records=comparison.overlay_observation(old,new)
        self.assertEqual(len(comparison.OVERLAY),len(records))
        self.assertFalse((old/product.relative_to(new)).exists())
        self.assertTrue(all(row["headSHA256"]==row["appliedSHA256"] for row in records))
        for paths in (("services/api/internal/authprivacy/community_comments_prepared.go",),(comparison.OVERLAY[0],)*2):
            with self.assertRaises(comparison.ComparisonError):comparison.overlay_observation(old,new,paths)
        with self.assertRaises(comparison.ComparisonError):comparison.overlay_observation(old,new)

    def test_archive_rejects_parent_absolute_symlink_and_special_entries(self):
        for name,kind,link in (("../outside",tarfile.REGTYPE,""),("/outside",tarfile.REGTYPE,""),
                               ("link",tarfile.SYMTYPE,"../../outside"),("device",tarfile.CHRTYPE,""),
                               ("hardlink",tarfile.LNKTYPE,"file")):
            archive=self.root/"bad.tar"
            with tarfile.open(archive,"w") as stream:
                entry=tarfile.TarInfo(name);entry.type=kind;entry.linkname=link;stream.addfile(entry)
            destination=self.root/"extract";destination.mkdir(exist_ok=True)
            with self.subTest(name=name),self.assertRaises(comparison.ComparisonError):comparison.extract_archive(archive,destination)

    @unittest.skipUnless(os.name=="posix","POSIX archive symlinks")
    def test_archive_resolves_complete_link_graph_and_cycles(self):
        for links in ((('a','.'),('b','a/../outside')),(('a','b'),('b','a'))):
            with tempfile.TemporaryDirectory(dir=self.root) as temporary:
                root=Path(temporary);archive=root/"links.tar";destination=root/"tree";destination.mkdir()
                with tarfile.open(archive,"w") as stream:
                    for name,target in links:
                        entry=tarfile.TarInfo(name);entry.type=tarfile.SYMTYPE;entry.linkname=target;stream.addfile(entry)
                with self.assertRaises(comparison.ComparisonError):comparison.extract_archive(archive,destination)
                self.assertFalse((root/"outside").exists())

    def test_clock_errors_missing_samples_and_wrong_commit_cannot_pass(self):
        spec=dict(comparison.profiles()[0],samples=2)
        data=good_data(spec)
        self.assertEqual("PASS",comparison.evaluate_cohort(data,spec,0,comparison.BASELINE)["result"])
        variants=[]
        value=copy.deepcopy(data);value["environmentValid"]=False;variants.append(value)
        value=copy.deepcopy(data);value["cohortClock"]["rawElapsedNS"]+=300_000_000;variants.append(value)
        value=copy.deepcopy(data);value["samples"][0]["error"]="private error material";variants.append(value)
        value=copy.deepcopy(data);value["warmupFailures"]=[value["samples"][0]];variants.append(value)
        value=copy.deepcopy(data);value["samples"].pop();variants.append(value)
        value=copy.deepcopy(data);value["sourceCommit"]="f"*40;variants.append(value)
        value=copy.deepcopy(data);value["samples"][0]["observation"]["stages"]["comments.gate_hold"]["nanoseconds"]=0;variants.append(value)
        for value in variants:
            with self.subTest(value=value):
                self.assertEqual("FAIL",comparison.evaluate_cohort(value,spec,0,comparison.BASELINE)["result"])
        self.assertEqual("FAIL",comparison.evaluate_cohort(data,spec,1)["result"])

    def test_unknown_kind_is_judged_before_grouping_and_cannot_hide_error(self):
        spec=dict(comparison.profiles()[0],samples=2);data=good_data(spec)
        extra=copy.deepcopy(data["samples"][0]);extra.update(kind="unknown",error="private failure",clockStable=False)
        data["samples"].append(extra)
        report=comparison.evaluate_cohort(data,spec,0)
        self.assertEqual("FAIL",report["result"])
        self.assertIn("unknown_sample_kind",report["reasons"])
        self.assertIn("request_failed",report["reasons"])
        self.assertIn("invalid_sample_clock_or_trace",report["reasons"])
        self.assertEqual(9,len(report["samples"]))

    def test_complete_configuration_and_timeouts_are_frozen_and_public(self):
        spec=dict(comparison.profiles()[0],samples=2);data=good_data(spec)
        for field,value in dict(goMaxProcs=8,posts=21,pageSize=20,feedPageSize=50,commentLikeRows=0,mix="other",snapshotBackend="postgres",likeTarget="comment-like-same-post",goVersion="go1.23.0").items():
            changed=copy.deepcopy(data);changed[field]=value
            with self.subTest(field=field):
                report=comparison.evaluate_cohort(changed,spec,0)
                self.assertEqual("FAIL",report["result"])
                self.assertIn(field,report["actualConfiguration"])
        changed=copy.deepcopy(data);changed["transactionSettings"]["statement_timeout"]="20s"
        report=comparison.evaluate_cohort(changed,spec,0)
        self.assertIn("wrong_transaction_settings",report["reasons"])
        self.assertEqual("20s",report["actualConfiguration"]["transactionSettings"]["statement_timeout"])

    def test_all_three_prepared_switches_are_required_and_public_per_side(self):
        for side in (0,1):
            spec=dict(comparison.profiles()[side],samples=2);data=good_data(spec)
            report=comparison.evaluate_cohort(data,spec,0)
            self.assertEqual("PASS",report["result"])
            for flag in comparison.PREPARED_FLAGS:
                self.assertIs(spec["prepared"],report["actualConfiguration"][flag])
                for bad in (not spec["prepared"],None,1,"true"):
                    changed=copy.deepcopy(data);changed[flag]=bad
                    with self.subTest(side=side,flag=flag,bad=bad):
                        report=comparison.evaluate_cohort(changed,spec,0)
                        self.assertEqual("FAIL",report["result"])
                        self.assertIn("wrong_"+flag,report["reasons"])
                changed=copy.deepcopy(data);changed.pop(flag)
                self.assertEqual("FAIL",comparison.evaluate_cohort(changed,spec,0)["result"])

    def test_new_comments_feed_and_image_each_need_an_observed_prepare(self):
        spec=dict(comparison.profiles()[1],samples=2);data=good_data(spec)
        for kind in ("comments","feed","image"):
            for calls in (None,0):
                changed=copy.deepcopy(data)
                stages=next(sample for sample in changed["samples"] if sample["kind"]==kind)["observation"]["stages"]
                if calls is None:stages.pop(kind+".prepare_read")
                else:stages[kind+".prepare_read"]["calls"]=calls
                with self.subTest(kind=kind,calls=calls):
                    report=comparison.evaluate_cohort(changed,spec,0)
                    self.assertEqual("FAIL",report["result"])
                    self.assertIn("prepared_path_not_observed_"+kind,report["reasons"])
        # The generic helper can emit an unprefixed phase. Judge the request's
        # own kind, rather than requiring a product-specific naming change.
        for sample in data["samples"]:
            stages=sample["observation"]["stages"]
            if sample["kind"] in ("feed","image"):
                stages["prepare_read"]=stages.pop(sample["kind"]+".prepare_read")
        self.assertEqual("PASS",comparison.evaluate_cohort(data,spec,0)["result"])

    def test_exact_thresholds_and_non_main_reporting(self):
        spec=dict(comparison.profiles()[0],samples=2)
        old=comparison.evaluate_cohort(good_data(spec),spec,0)
        new=copy.deepcopy(old)
        old["metrics"]["comments"]["gateHoldPerRequest"]["p95NS"]=10000
        old["metrics"]["feed"]["elapsed"]["p95NS"]=10000
        new["metrics"]["comments"]["gateHoldPerRequest"]["p95NS"]=5000
        new["metrics"]["feed"]["elapsed"]["p95NS"]=7000
        self.assertEqual("PASS",comparison.compare_pair(old,new,True)["result"])
        new["metrics"]["comments"]["gateHoldPerRequest"]["p95NS"]=5001
        self.assertEqual("FAIL",comparison.compare_pair(old,new,True)["result"])
        self.assertEqual("PASS",comparison.compare_pair(old,new,False)["result"])
        new["result"]="FAIL"
        self.assertEqual("FAIL",comparison.compare_pair(old,new,False)["result"])

    def test_publication_drops_messages_and_selects_exact_artifacts(self):
        spec=dict(comparison.profiles()[0],samples=2)
        data=good_data(spec);data["samples"][0].update(error="do-not-publish-payload",errorCode="INVALID TOKEN SECRET",errorStatus=503)
        public=comparison.evaluate_cohort(data,spec,0)
        self.assertNotIn("do-not-publish-payload",json.dumps(public))
        self.assertNotIn("INVALID TOKEN SECRET",json.dumps(public))
        directory=self.root/"public";directory.mkdir()
        for name in ("manifest.json","comparison-summary.json","cohort-main-r1-old.json","private.log","credentials.private.json"):
            (directory/name).write_text("{}")
        selected=[path.name for path in comparison.public_artifacts(directory)]
        self.assertEqual(["cohort-main-r1-old.json","comparison-summary.json","manifest.json"],selected)

    def test_frozen_profiles_are_fourteen_and_not_retries(self):
        profiles=comparison.profiles()
        self.assertEqual(14,len(profiles));self.assertEqual(14,len({spec["label"] for spec in profiles}))
        self.assertEqual(["old","new"]*7,[spec["side"] for spec in profiles])
        self.assertEqual([80]*6+[40]*4+[8]*4,[spec["samples"] for spec in profiles])

    @unittest.skipUnless(hasattr(os,"killpg"),"POSIX process groups")
    def test_interrupt_cleanup_terminates_owned_group_and_escalates(self):
        child=mock.Mock(pid=12345);child.poll.return_value=None
        child.wait.side_effect=[subprocess.TimeoutExpired("owned",30),0]
        with mock.patch.object(comparison.os,"killpg") as kill:
            comparison.stop_child(child)
        self.assertEqual([mock.call(12345,signal.SIGTERM),mock.call(12345,signal.SIGKILL)],[call for call in kill.call_args_list if call.args[1]!=0])

    def test_timeout_and_interrupt_preserve_existing_raw_samples(self):
        spec=dict(comparison.profiles()[0],samples=2)
        for problem,termination in ((subprocess.TimeoutExpired("owned",720),"timeout"),(comparison.InterruptedComparison(signal.SIGTERM),"interrupted")):
            with tempfile.TemporaryDirectory(dir=self.root) as temporary:
                private=Path(temporary);(private/(spec["label"]+".json")).write_text(json.dumps(good_data(spec)))
                child=mock.Mock();child.wait.side_effect=problem
                with mock.patch.object(comparison.subprocess,"Popen",return_value=child),mock.patch.object(comparison,"stop_child") as stop:
                    report=comparison.run_cohort(self.root,spec,comparison.BASELINE,private,self.root,Path(sys.executable),Path(sys.executable))
                self.assertEqual("FAIL",report["result"]);self.assertEqual(termination,report["termination"])
                self.assertEqual(8,len(report["samples"]));self.assertEqual(spec["label"],report["label"]);stop.assert_called_once_with(child)

    @unittest.skipUnless(hasattr(os,"killpg"),"POSIX process groups")
    def test_cleanup_stops_descendant_after_shell_leader_already_exited(self):
        ready,stopped=self.root/"ready",self.root/"stopped"
        child_code="import signal,sys,time;from pathlib import Path;signal.signal(signal.SIGTERM,lambda *_:(Path(sys.argv[2]).write_text('stopped'),sys.exit(0)));Path(sys.argv[1]).write_text('ready');time.sleep(120)"
        leader_code="import subprocess,sys;p=subprocess.Popen([sys.executable,'-c',sys.argv[1],sys.argv[2],sys.argv[3]],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL);print(p.pid,flush=True)"
        leader=subprocess.Popen([sys.executable,"-c",leader_code,child_code,str(ready),str(stopped)],stdout=subprocess.PIPE,text=True,start_new_session=True)
        try:
            leader.stdout.readline();leader.wait(timeout=5)
            deadline=time.monotonic()+5
            while not ready.exists() and time.monotonic()<deadline:time.sleep(.01)
            self.assertTrue(ready.exists());self.assertIsNotNone(leader.poll())
            comparison.stop_child(leader)
            self.assertTrue(stopped.exists())
        finally:
            try:os.killpg(leader.pid,signal.SIGKILL)
            except ProcessLookupError:pass
            leader.stdout.close()


if __name__ == "__main__":unittest.main()
