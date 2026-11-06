#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import base64
import gzip
import io
import json
import os
import subprocess

import pytest

from safe_post_training import spur_import

RAY = "\x1b[36m(MegatronTrainRayActor pid=12, ip=10.0.0.1)\x1b[0m "
KEY = "0123456789abcdef0123456789abcdef01234567"


def write(path, text):
    """Write a file, creating its directory."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        f.write(text)


def gate_lines(host, rank, gates):
    """Return the node log lines of (gate, begin, end, result) tuples."""
    out = []
    for gate, begin, end, result in gates:
        out.append("[2026-10-21T10:%s:00Z] [%s r%d] BEGIN %s" % (begin, host, rank, gate))
        if end:
            out.append("[2026-10-21T10:%s:00Z] [%s r%d] END %s %s" % (end, host, rank, gate, result))
    return out


def make_run(root, verdict="FAIL", rl=False):
    """Write a slime-rocm run directory with two nodes."""
    train = "b6_grpo" if rl else "b2_sft"
    head_gates = [("a1_host", "10", "11", "pass"), ("a1_ray", "12", "14", "pass")]
    if rl:
        head_gates.append(("b6_dryrun", "14", "16", "pass"))
    head_gates += [(train, "16", "50", "fail(1)" if verdict == "FAIL" else "pass"),
                   ("c9_custom", "50", "51", "pass")]
    for host, rank, gates in (("h1", 0, head_gates), ("h2", 1, [("a1_host", "10", "11", "pass"),
                                                               (train, "16", "49", "pass")])):
        first = "[2026-10-21T10:10:00Z] [%s] rank=%d/2 ip=10.0.0.%d head=h1(10.0.0.1) image=sglang:v0.5" % (
            host, rank, rank + 1)
        write(os.path.join(root, "node-%s.log" % host), "\n".join([first] + gate_lines(host, rank, gates)) + "\n")
    rows = ["a1_host\th1\tpass\t60", "a1_ray\th1\tpass\t120", "%s\th1\t%s\t2040" % (
        train, "fail(1)" if verdict == "FAIL" else "pass"), "a1_host\th2\tpass\t60", "%s\th2\tpass\t1980" % train]
    write(os.path.join(root, "results", "results.tsv"), "\n".join(rows) + "\n")
    if verdict == "FAIL":
        write(os.path.join(root, "FAIL"), "failed items:\n%s\th1\tfail(1)\t2040\n" % train)
    elif verdict == "PASS":
        write(os.path.join(root, "PASS"), "\n".join(rows) + "\n")
    for i, name in enumerate(("ray_head_up", "done_0")):
        write(os.path.join(root, "markers", name), "")
        os.utime(os.path.join(root, "markers", name), (1792578000 + i * 60, 1792578000 + i * 60))
    write(os.path.join(root, "mon", "h1.tsv"), "ts\tmem_total_kb\tmem_avail_kb\tvram_used_max_b\tvram_used_sum_b\n"
          "2026-10-21T10:20:00Z\t1000\t400\t5000\t9000\n2026-10-21T10:20:30Z\t1000\t100\t7000\t9000\n")
    args = ["  loss_type ...... %s" % ("policy_loss" if rl else "sft_loss"), "  lr ...... 1e-06",
            "  load ...... /models/GLM-5.3-BF16/", "  input_key ...... messages", "  wandb_key ...... %s" % KEY,
            "  max_tokens_per_gpu ...... 16384", "  rollout_batch_size ...... 32",
            "  n_samples_per_prompt ...... 8", "  global_batch_size ...... 256", "  advantage_estimator ...... grpo",
            "  prompt_data ...... /d/train.jsonl"]
    log = ["[2026-10-21 10:16:05] arguments.py:12 - starting",
           "------------------------ arguments ------------------------"]
    log += args + ["-------------------- end of arguments ---------------------"]
    log += [RAY + "[2026-10-21 10:20:00] model.py:953 - step 0: {'train/loss': 2.0, 'train/step': 0}",
            RAY + "[2026-10-21 10:21:00] model.py:953 - step 1: {'train/loss': 1.5}",
            RAY + "[2026-10-21 10:21:30] model.py:953 - eval 0: {'eval/val/loss': 2.1}",
            RAY + "[2026-10-21 10:22:00] model.py:953 - eval 1: {'eval/val/loss': 1.7}",
            RAY + "NCCL INFO noise", "trainable: dense 18.60B, total 95.91B",
            "FAIL: VRAM still rising over steps 10-20", json.dumps({"gate": "t2_export", "dev_nll": 1.23})]
    write(os.path.join(root, "logs", "%s-h1.log" % train), "\n".join(log) + "\n")
    write(os.path.join(root, "ckpt", "latest_checkpointed_iteration.txt"), "1\n")
    write(os.path.join(root, "ckpt", "iter_0000001", "model.pt"), "x" * 10)
    write(os.path.join(root, "rollout", "0.pt"), "y" * 4)
    return root


def test_a_failed_sft_run(tmp_path):
    root = make_run(str(tmp_path / "j2r5-229357"))
    s = spur_import.parse_slime_rocm(root)
    assert (s["schema"], s["verdict"], s["status"], s["failedStage"]) == ("slime-rocm-run/1", "FAIL", "Failed", "train")
    assert s["message"] == "FAIL: VRAM still rising over steps 10-20", "the check output's first FAIL line"
    assert s["slurmJobId"] == "229357" and s["head"] == "h1" and [n["host"] for n in s["nodes"]] == ["h1", "h2"]
    assert s["nodes"][0]["image"] == "sglang:v0.5" and s["nodes"][0]["head"] is True
    assert [st["label"] for st in s["stages"]] == ["Preflight", "Ray", "Train", "c9_custom", "Verdict"]
    assert s["stages"][2]["result"] == "fail" and s["stages"][0]["start"] == "2026-10-21T10:10:00Z"
    assert any("c9_custom" in w for w in s["warnings"]), "an unknown gate is its own stage, with a note"
    kept = s["arguments"]["kept"]
    assert kept["wandb_key"] == "***" and KEY not in json.dumps(s)
    assert kept["input_key"] == "messages" and kept["max_tokens_per_gpu"] == 16384 and kept["lr"] == 1e-06
    assert s["arguments"]["total"] == 11 and s["baseModel"] == "GLM-5.3-BF16" and s["dataset"] == "/d/train.jsonl"
    loss = s["metrics"]["train/loss"]
    assert loss["axis"] == "train" and [p[:2] for p in loss["points"]] == [[0, 2.0], [1, 1.5]]
    assert loss["points"][0][2] == spur_import.utc("2026-10-21T10:20:00Z"), "the wall time is the line's UTC time"
    assert [p[0] for p in s["metrics"]["eval/val/loss"]["points"]] == [0, 2], "eval N of SFT comes after N + 1 steps"
    assert "train/step" not in s["metrics"]
    assert s["checkpoints"]["iterations"][0]["step"] == 2 and s["checkpoints"]["latest"] == 1
    assert s["checkpoints"]["rollout"] == {"files": 1, "bytes": 4}
    assert s["resources"]["h1"]["hostMemPeakKb"] == 900 and s["resources"]["h1"]["vramPeakBytes"] == 7000
    assert s["trainable"] == "dense 18.60B, total 95.91B" and s["finalEval"]["data"]["dev_nll"] == 1.23
    assert "NCCL INFO noise" not in s["check"], "the check output is the plain lines at the end"
    assert s["mainLog"]["name"] == "b2_sft-h1.log" and s["mainLog"]["metricLines"] == 2 and s["saveSet"] is False
    out = io.StringIO()
    spur_import.report(s, out)
    assert "  node-*.log   2 nodes · 6 gate results · FAIL (b2_sft fail(1) on h1)" in out.getvalue()
    assert "b2_sft-h1.log" in out.getvalue() and "2 step lines" in out.getvalue()


def test_a_run_without_verdict_is_stopped(tmp_path):
    s = spur_import.parse_slime_rocm(make_run(str(tmp_path / "j3-1"), verdict=""))
    assert s["status"] == "Stopped" and s["verdict"] == ""
    assert "the last marker is done_0" in s["message"]

    root = make_run(str(tmp_path / "j3-2"), verdict="")
    log = os.path.join(root, "node-h1.log")
    write(log, "".join(line for line in open(log) if "END b2_sft" not in line))
    train = next(st for st in spur_import.parse_slime_rocm(root)["stages"] if st["stage"] == "train")
    assert train["result"] == "running" and train["end"] is None, "a gate without its END line has not ended"


def test_a_passed_rl_run(tmp_path):
    s = spur_import.parse_slime_rocm(make_run(str(tmp_path / "j3k-238752"), verdict="PASS", rl=True))
    assert (s["status"], s["trainType"], s["algorithm"], s["stepsPerRollout"]) == ("Succeeded", "rl", "grpo", 1)
    assert [st["stage"] for st in s["stages"]][:4] == ["preflight", "ray", "dryrun", "train"]
    assert s["metrics"]["eval/val/loss"]["axis"] == "rollout"
    with pytest.raises(spur_import.RunDirError, match="no node"):
        spur_import.parse_slime_rocm(str(tmp_path))


def test_a_large_summary_is_downsampled_keeping_the_extremes():
    points = [[i, float(i % 7), 0] for i in range(5000)]
    summary = {"metrics": {"train/loss": {"axis": "train", "count": 5000, "downsampled": False, "points": points}}}
    data = spur_import.encode(summary, limit=60000)
    s = json.loads(data)["metrics"]["train/loss"]
    assert s["downsampled"] is True and s["count"] == 5000 and len(s["points"]) < 5000
    assert min(p[1] for p in s["points"]) == 0.0 and max(p[1] for p in s["points"]) == 6.0
    with pytest.raises(spur_import.RunDirError, match="above the limit"):
        spur_import.encode({"metrics": {}, "blob": "x" * 100}, limit=10)


def token(address="https://safe.example.com"):
    """Return an import token naming address."""
    return "sst_%s.%s" % (base64.urlsafe_b64encode(address.encode()).decode().rstrip("="), "r" * 43)


class Answer(io.BytesIO):
    """An HTTP answer the fake opener returns."""

    status = 201

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


def test_upload_sends_the_gzip_summary_with_the_token():
    sent = {}

    def opener(req, timeout):
        sent.update(url=req.full_url, auth=req.get_header("Authorization"), body=json.loads(gzip.decompress(req.data)))
        return Answer(b'{"runId": "spur-j2"}')

    t = token()
    status, answer = spur_import.upload(b'{"name": "j2"}', t, opener=opener)
    assert (status, answer) == (201, {"runId": "spur-j2"})
    assert sent == {"url": "https://safe.example.com/api/v1/post-training/imports", "auth": "Bearer " + t,
                    "body": {"name": "j2"}}
    assert spur_import.token_address(t) == "https://safe.example.com"
    with pytest.raises(spur_import.RunDirError):
        spur_import.token_address("sst_garbage")


def test_a_token_must_name_an_http_address(capsys):
    for address in ("file:///etc/passwd", "ftp://safe.example.com", "https://", "safe.example.com", "http://[::1"):
        with pytest.raises(spur_import.RunDirError, match="not an http or https address"):
            spur_import.token_address(token(address))

    def answer(req, timeout):
        return Answer(b"{}")

    assert spur_import.upload(b"{}", token("http://safe.example.com"), opener=answer)[0] == 201
    assert "plain http, so the token and the summary travel unencrypted" in capsys.readouterr().err
    spur_import.upload(b"{}", token(), opener=answer)
    assert capsys.readouterr().err == "", "https needs no warning"


def test_main_reads_the_token_from_the_environment(monkeypatch, tmp_path, capsys):
    root = make_run(str(tmp_path / "j2-1"))
    calls = []
    monkeypatch.setattr(spur_import, "upload", lambda data, t, stage=False: calls.append((t, stage)) or
                        (409, {"errorMessage": "the directory is already imported", "runId": "spur-j2-1"}))
    monkeypatch.delenv(spur_import.TOKEN_ENV, raising=False)
    monkeypatch.setattr("sys.stdin", io.StringIO())
    assert spur_import.main([root]) == 1, "no token, and no terminal to ask on"
    monkeypatch.setenv(spur_import.TOKEN_ENV, token())
    assert spur_import.main([root]) == 2
    assert calls == [(token(), False)]
    assert "This directory is already imported: https://safe.example.com/post-training/detail?runId=spur-j2-1" in \
        capsys.readouterr().err

    assert spur_import.main([root, "--out", os.path.join(root, "summary.json")]) == 1, "the run directory is read-only"
    out = str(tmp_path / "summary.json.gz")
    assert spur_import.main([root, "--out", out]) == 0
    assert json.loads(gzip.decompress(open(out, "rb").read()))["name"] == "j2-1"
    assert spur_import.main(["--upload", out]) == 2 and len(calls) == 2
    assert spur_import.main([root, "--dry-run"]) == 0 and len(calls) == 2, "a dry run uploads nothing"


def test_results_that_disagree_with_the_node_logs(tmp_path):
    root = make_run(str(tmp_path / "j2-2"))
    rows = open(os.path.join(root, "results", "results.tsv")).read().replace("b2_sft\th1\tfail(1)", "b2_sft\th1\tpass")
    write(os.path.join(root, "results", "results.tsv"), rows)
    s = spur_import.parse_slime_rocm(root)
    assert any("results.tsv says pass but the node log says fail(1)" in w for w in s["warnings"])
    assert next(g for g in s["gates"] if g["gate"] == "b2_sft" and g["node"] == "h1")["result"] == "pass"


def test_the_command_runs_from_a_copy_of_the_package(tmp_path):
    root = make_run(str(tmp_path / "j2-3"))
    script = os.path.join(os.path.dirname(spur_import.__file__), "bin", "safe-post-training-import")
    out = subprocess.run(["sh", script, "--dry-run", root], capture_output=True, text=True, timeout=60)
    assert out.returncode == 0, out.stderr
    assert json.loads(out.stdout)["name"] == "j2-3" and "points" in out.stdout


def test_a_scan_reads_only_under_the_mount_root(monkeypatch, tmp_path, capsys):
    mount = tmp_path / "shared_nfs"
    inside = make_run(str(mount / "runs" / "j2-4"))
    outside = make_run(str(tmp_path / "elsewhere" / "j2-5"))
    os.symlink(outside, str(mount / "runs" / "link"))
    log = tmp_path / "termination-log"
    monkeypatch.setattr(spur_import, "TERMINATION_LOG", str(log))
    monkeypatch.setenv(spur_import.TOKEN_ENV, token())
    calls = []
    monkeypatch.setattr(spur_import, "upload", lambda data, t, stage=False: calls.append(stage) or
                        (422, {"errorMessage": "unknown parserVersion"}))
    for refused in (outside, str(mount / "runs" / "link"), str(mount / "runs" / "j2-4" / ".." / ".." / ".." /
                                                              "elsewhere" / "j2-5")):
        assert spur_import.main([refused, "--stage", "--root", str(mount)]) == 1
        assert log.read_text() == "The directory must be under %s." % mount
    assert calls == [], "nothing outside the mount is read or uploaded"
    assert spur_import.main([inside, "--stage", "--root", str(mount)]) == 2 and calls == [True]
    assert log.read_text() == "The upload was refused: unknown parserVersion", "the dialog shows why the scan failed"
    log.unlink()
    assert spur_import.main([outside, "--dry-run", "--root", str(mount)]) == 1 and not log.exists(), \
        "only a scan job leaves a termination message"
    assert spur_import.main([inside, "--stage", "--root", inside + "/"]) == 2 and calls == [True, True], \
        "the root itself may be the run directory"
    capsys.readouterr()
