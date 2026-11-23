#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import ast
import errno
import http.server
import io
import json
import os
import re
import signal
import subprocess
import sys
import threading

import pytest

from safe_post_training import launch, metrics, stages


class FakeAdapter:
    """Trains by printing slime-style metric lines and exports a tiny model directory."""

    def __init__(self, fail_train=False):
        self.fail_train = fail_train

    def convert(self, recipe, run_dir, run_command):
        os.makedirs(recipe["convert"]["dir"], exist_ok=True)

    def train_command(self, recipe, run_dir):
        code = ("import sys\n"
                "print(\"x - step 0: {'train/loss': 2.0, 'train/step': 0}\")\n"
                "print('[SUCCESS] {\"run\":\"pt-x\",\"output\":\"/elsewhere\"}')\n"
                "print(\"x - step 1: {'train/loss': 1.5}\")\n"
                "sys.exit(%d)\n" % (3 if self.fail_train else 0))
        return [sys.executable, "-c", code], {}, {"plugins.yaml": "a: 1"}

    def latest_checkpoint(self, run_dir, sub="checkpoints", strict=False):
        return 1 if os.path.exists(os.path.join(run_dir, ".train.done")) else None

    def export(self, recipe, run_dir, run_command, iteration=None, out=None):
        self.exported = iteration
        out = out or os.path.join(run_dir, "final_model")
        os.makedirs(out, exist_ok=True)
        for name in ("config.json", "model.safetensors", "tokenizer.json"):
            open(os.path.join(out, name), "w").close()
        return out

    def parse_metric_line(self, line):
        m = re.search(r"step (\d+): (\{.*\})$", line.strip())
        if not m:
            return None
        values = {k: v for k, v in ast.literal_eval(m.group(2)).items() if not k.endswith("/step")}
        return "step", "train", int(m.group(1)), values


def recipe_for(tmp_path):
    model = tmp_path / "model"
    model.mkdir()
    (model / "config.json").write_text("{}")
    data = tmp_path / "train.jsonl"
    data.write_text(json.dumps({"instruction": "a", "output": "b"}) + "\n")
    run_dir = tmp_path / "run"
    return {"runId": "pt-x", "framework": "fake", "trainType": "sft", "model": {"path": str(model)},
            "data": {"train": str(data), "fieldMap": {}}, "layout": {"nodes": 1, "gpusPerNode": 8},
            "metrics": {"include": ["train/*"]}, "convert": {"dir": str(run_dir / "convert" / "torch_dist")},
            "checkpoint": {"keepLast": 0}, "frameworkArgs": [], "output": {"base": str(run_dir)}}


def markers(out):
    result = []
    for line in out.getvalue().splitlines():
        if line.startswith("["):
            marker, body = line.split(" ", 1)
            result.append((marker, json.loads(body)))
    return result


def run(monkeypatch, recipe, adapter, gpus=8):
    monkeypatch.setattr(launch, "load_adapter", lambda name: adapter)
    out = io.StringIO()
    code = launch.run(recipe, out=out, gpus=lambda: gpus, sleep=lambda s: None)
    return code, out, markers(out)


def test_successful_run(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    code, out, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 0
    assert ms[0][0] == "[STAGE]" and ms[0][1]["stage"] == "start" and ms[0][1]["attempt"] == "a1"
    assert ms[-1][0] == "[SUCCESS]", "the result marker is the last marker"
    result = ms[-1][1]
    assert result["output"] == str(tmp_path / "run" / "final_model")
    assert result["lastMetrics"] == {"train/loss": [1, 1.5]}
    assert [m[1]["step"] for m in ms if m[0] == "[METRIC]"] == [0, 1]
    assert '  [SUCCESS] {"run":"pt-x","output":"/elsewhere"}' in out.getvalue(), "child markers are escaped"
    assert (tmp_path / "run" / "plugins.yaml").read_text() == "a: 1"
    assert json.loads((tmp_path / "run" / "result.json").read_text())["output"] == result["output"]

    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 0 and ms[0][1]["attempt"] == "a2"
    assert {m[1]["stage"] for m in ms if m[1].get("state") == "skipped"} == {"prepare", "convert", "train", "export"}


def test_an_hf_direct_run_has_no_convert_stage(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    del recipe["convert"]
    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 0
    assert "convert" not in {m[1].get("stage") for m in ms if m[0] == "[STAGE]"}
    assert not (tmp_path / "run" / "convert").exists()


def test_training_failure_is_retried(monkeypatch, tmp_path):
    code, _, ms = run(monkeypatch, recipe_for(tmp_path), FakeAdapter(fail_train=True))
    assert code == 1
    assert ms[-1] == ("[ERROR]", {"run": "pt-x", "attempt": "a1", "pv": 1, "stage": "train",
                                  "reason": "training exited with code 3", "retry": True})
    assert not (tmp_path / "run" / ".noretry").exists()


def test_dataset_errors_are_not_retried(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    (tmp_path / "train.jsonl").write_text(json.dumps({"instruction": "a"}) + "\n")
    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 1
    assert any(m[1].get("retry") is False and m[0] == "[STAGE]" for m in ms)
    assert ms[-1][0] == "[ERROR]" and ms[-1][1]["reason"] == "field 'output' missing in 1 of 1 rows"

    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 1 and len(ms) == 2, "the next attempt reports the stored error before doing any work"
    assert ms[-1][1]["retry"] is False


def test_sigterm_releases_the_conversion_lock(tmp_path):
    code = ("import sys, time\n"
            "from safe_post_training import launch, stages\n"
            "launch.exit_on_sigterm()\n"
            "with stages.cache_lock(sys.argv[1], owner={'run': 'pt-x', 'attempt': 'a1'}):\n"
            "    print('locked', flush=True)\n"
            "    time.sleep(60)\n")
    root = os.path.dirname(os.path.dirname(os.path.abspath(stages.__file__)))
    proc = subprocess.Popen([sys.executable, "-c", code, str(tmp_path)], stdout=subprocess.PIPE, text=True,
                            env=dict(os.environ, PYTHONPATH=root))
    assert proc.stdout.readline().strip() == "locked"
    proc.send_signal(signal.SIGTERM)
    assert proc.wait(timeout=30) == 128 + signal.SIGTERM
    assert not (tmp_path / stages.CONVERT_LOCK).exists(), "a stopped run does not leave its lock behind"


class NoSpaceAdapter(FakeAdapter):
    """Trains by printing one line, with RUN standing for the run directory, and failing."""

    def __init__(self, line):
        super().__init__()
        self.line = line

    def train_command(self, recipe, run_dir):
        return [sys.executable, "-c", "print(%r); raise SystemExit(1)" % self.line.replace("RUN", run_dir)], {}, {}


def test_only_running_out_of_space_in_the_run_directory_is_final(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    code, _, ms = run(monkeypatch, recipe, NoSpaceAdapter("NCCL WARN Call to posix_fallocate failed : "
                                                          "No space left on device"))
    assert code == 1 and ms[-1][1]["retry"] is True, "a full /dev/shm says nothing about the shared storage"
    assert ms[-1][1]["reason"] == "training exited with code 1"
    code, _, ms = run(monkeypatch, recipe, NoSpaceAdapter("OSError: [Errno 28] No space left on device: "
                                                          "'RUN/checkpoints/iter_0000001/0_0.distcp'"))
    assert code == 1 and ms[-1][1]["retry"] is False
    assert ms[-1][1]["reason"].startswith("shared storage is out of space")


def test_running_out_of_space_without_a_path_is_final_on_a_full_storage(monkeypatch, tmp_path):
    """A failed write to an open file names no path, so the free space of the run directory decides."""
    for name, full in (("room", False), ("full", True)):
        (tmp_path / name).mkdir()
        monkeypatch.setattr(stages, "storage_full", lambda run_dir, full=full: full)
        code, _, ms = run(monkeypatch, recipe_for(tmp_path / name),
                          NoSpaceAdapter("OSError: [Errno 28] No space left on device"))
        assert code == 1 and ms[-1][1]["retry"] is not full, name


class ExportFailsAdapter(FakeAdapter):
    """Fails the export with error."""

    def __init__(self, error):
        super().__init__()
        self.error = error

    def export(self, recipe, run_dir, run_command, iteration=None, out=None):
        raise self.error


def test_a_launcher_error_out_of_space_elsewhere_is_retried(monkeypatch, tmp_path):
    for name, path, retry in (("tmp", "/tmp/ray/spill/x", True), ("run", "RUN/final_model/x", False)):
        (tmp_path / name).mkdir()
        recipe = recipe_for(tmp_path / name)
        filename = path.replace("RUN", recipe["output"]["base"])
        err = OSError(errno.ENOSPC, os.strerror(errno.ENOSPC), filename)
        code, _, ms = run(monkeypatch, recipe, ExportFailsAdapter(err))
        assert code == 1 and ms[-1][1]["retry"] is retry, name


def test_errors_before_the_stages_still_end_with_an_error_marker(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    status = os.path.join(recipe["output"]["base"], launch.STATUS_FILE)

    def full(run_dir):
        raise OSError(errno.ENOSPC, os.strerror(errno.ENOSPC), status)

    monkeypatch.setattr(launch, "next_attempt", full)
    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 1 and ms[0][1]["stage"] == "start" and ms[0][1]["attempt"] == "a1"
    assert ms[-1][0] == "[ERROR]" and ms[-1][1]["retry"] is False
    assert ms[-1][1]["reason"].startswith("shared storage is out of space")


def test_an_unreadable_noretry_file_is_still_final(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    os.makedirs(recipe["output"]["base"])
    open(os.path.join(recipe["output"]["base"], launch.NORETRY_FILE), "w").close()
    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 1 and ms[-1][0] == "[ERROR]" and ms[-1][1]["retry"] is False
    assert ms[-1][1]["reason"] == "an earlier attempt failed for good and its reason was not recorded"


class RaisingEmitter(metrics.Emitter):
    """Fails to forward any output line."""

    def forward(self, line):
        raise OSError(errno.ENOSPC, os.strerror(errno.ENOSPC))


def test_a_crash_while_reading_the_output_ends_the_command(tmp_path):
    pid_file = tmp_path / "pid"
    code = ("import os, time\n"
            "open(%r, 'w').write(str(os.getpid()))\n"
            "print('line', flush=True)\n"
            "time.sleep(60)\n" % str(pid_file))
    runner = stages.Runner({}, str(tmp_path), FakeAdapter(), RaisingEmitter("pt-x", "a1", out=io.StringIO()))
    with pytest.raises(OSError):
        runner.run_command([sys.executable, "-c", code])
    with pytest.raises(ProcessLookupError):
        os.kill(int(pid_file.read_text()), 0)


def test_a_metric_line_that_breaks_the_parser_is_counted(tmp_path):
    runner = stages.Runner({}, str(tmp_path), FakeAdapter(), metrics.Emitter("pt-x", "a1", out=io.StringIO()))
    runner.parse_line("x - step 1: {[1]: 2}\n")
    assert runner.emitter.parse_failures == 1


def test_too_few_gpus(monkeypatch, tmp_path):
    code, _, ms = run(monkeypatch, recipe_for(tmp_path), FakeAdapter(), gpus=4)
    assert code == 1 and ms[-1][1]["reason"] == "the Ray cluster has 4 GPUs, the recipe needs 8"


class FakeWandbAdapter(FakeAdapter):
    """Prints the run link the way wandb does before the metric lines."""

    def train_command(self, recipe, run_dir):
        cmd, env, files = super().train_command(recipe, run_dir)
        cmd[-1] = "print('wandb: View run pt-x at: https://wandb.example/team/pt/runs/abc')\n" + cmd[-1]
        return cmd, env, files


def test_wandb_link_and_probe(monkeypatch, tmp_path):
    server = http.server.HTTPServer(("127.0.0.1", 0), http.server.BaseHTTPRequestHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    monkeypatch.setenv("WANDB_BASE_URL", "http://127.0.0.1:%d" % server.server_port)
    recipe = recipe_for(tmp_path)
    recipe["tracking"] = {"wandb": {"enabled": True, "secretId": "wb", "project": "pt"}}
    try:
        code, _, ms = run(monkeypatch, recipe, FakeWandbAdapter())
    finally:
        server.shutdown()
    assert code == 0, "a server that answers, even with an error status, is reachable"
    links = [m[1] for m in ms if m[0] == "[METRIC]" and m[1]["kind"] == "link"]
    assert len(links) == 1 and links[0]["v"] == {"wandb": "https://wandb.example/team/pt/runs/abc"}
    assert "seq" not in links[0], "a link is not a point and leaves the seq of the points alone"
    assert [m[1]["seq"] for m in ms if m[0] == "[METRIC]" and m[1]["kind"] != "link"] == [1, 2]


def test_wandb_unreachable(monkeypatch, tmp_path):
    monkeypatch.setenv("WANDB_BASE_URL", "http://127.0.0.1:9")
    recipe = recipe_for(tmp_path)
    recipe["tracking"] = {"wandb": {"enabled": True}}
    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 1
    assert ms[-1][1]["stage"] == "check"
    assert ms[-1][1]["reason"] == "the training nodes can't reach wandb at http://127.0.0.1:9"


def test_a_run_without_the_hf_export_exports_nothing(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    recipe["output"]["exportHf"] = False
    adapter = FakeAdapter()
    code, _, ms = run(monkeypatch, recipe, adapter)
    assert code == 0 and getattr(adapter, "exported", None) is None, "no checkpoint is converted"
    states = [m[1]["state"] for m in ms if m[0] == "[STAGE]" and m[1]["stage"] == "export"]
    assert states[0] == "skipped" and "running" not in states, states
    assert ms[-1][0] == "[SUCCESS]" and ms[-1][1]["output"] == ""
    assert json.loads((tmp_path / "run" / "result.json").read_text())["output"] == ""


class PPOAdapter(FakeAdapter):
    """A fake adapter whose actor and critic checkpoints end at the given steps."""

    def __init__(self, actor, critic):
        super().__init__()
        self.steps = {"checkpoints": actor, "critic_checkpoints": critic}

    def latest_checkpoint(self, run_dir, sub="checkpoints", strict=False):
        return self.steps[sub]

    def set_latest_checkpoint(self, run_dir, sub, iteration):
        self.steps[sub] = iteration


def test_ppo_resumes_only_in_step(monkeypatch, tmp_path):
    for name, actor, critic in (("fresh", None, None), ("resumed", 40, 40)):
        (tmp_path / name).mkdir()
        recipe = recipe_for(tmp_path / name)
        recipe["critic"] = {"megatronConfig": "megatron.yaml", "checkpoints": "critic_checkpoints"}
        code, _, ms = run(monkeypatch, recipe, PPOAdapter(actor, critic))
        assert code == 0, name
    assert any(m[1].get("detail", {}).get("resumeStep") == 40 for m in ms if m[0] == "[STAGE]")

    (tmp_path / "apart").mkdir()
    recipe = recipe_for(tmp_path / "apart")
    recipe["critic"] = {"megatronConfig": "megatron.yaml", "checkpoints": "critic_checkpoints"}
    adapter = PPOAdapter(40, 30)
    code, _, ms = run(monkeypatch, recipe, adapter)
    assert code == 0, "the actor and the critic go back to the iteration they share, here none"
    assert adapter.steps == {"checkpoints": None, "critic_checkpoints": None}
    assert not any("resumeStep" in (m[1].get("detail") or {}) for m in ms if m[0] == "[STAGE]")


def test_the_check_needs_the_spec_host_memory_on_every_node(tmp_path):
    recipe = dict(recipe_for(tmp_path), minHostMemoryTB=2.1)
    recipe["model"]["spec"] = "glm5.3-744B-A40B"
    run_dir = tmp_path / "run"
    (run_dir / "data").mkdir(parents=True)
    (run_dir / "data" / "train.jsonl").write_text("{}\n")
    emitter = metrics.Emitter("pt-x", "a1", None, out=io.StringIO())
    nodes = [("m2m-065", 2.2e12), ("m2m-085", 1.9e12)]
    runner = stages.Runner(recipe, str(run_dir), FakeAdapter(), emitter, gpus=lambda: 8,
                           nodes=lambda: [{"host": h, "memory": m, "rdma": []} for h, m in nodes])
    with pytest.raises(stages.StageError) as e:
        runner.stage_check()
    assert e.value.reason == "glm5.3-744B-A40B needs 2.1 TB of host memory per node: m2m-085 has 1.90 TB"
    assert not e.value.retry, "the same nodes come back on a retry"
    nodes[1] = ("m2m-085", 2.4e12)
    runner.stage_check()

    del recipe["minHostMemoryTB"]
    stages.Runner(recipe, str(run_dir), FakeAdapter(), emitter, gpus=lambda: 8,
                  nodes=lambda: pytest.fail("specs without a floor are not probed")).stage_check()


def test_the_check_refuses_a_pattern_python_cannot_compile(tmp_path):
    recipe = dict(recipe_for(tmp_path), choices={"freeze": {"trainOnly": [r"^(?!.*experts\.)", r"layers\.[0-"]}})
    run_dir = tmp_path / "run"
    (run_dir / "data").mkdir(parents=True)
    (run_dir / "data" / "train.jsonl").write_text("{}\n")
    emitter = metrics.Emitter("pt-x", "a1", None, out=io.StringIO())
    runner = stages.Runner(recipe, str(run_dir), FakeAdapter(), emitter, gpus=lambda: 8)
    with pytest.raises(stages.StageError) as e:
        runner.stage_check()
    assert e.value.reason.startswith("the pattern to train 'layers\\\\.[0-' is not a valid regex")
    assert not e.value.retry
    recipe["choices"]["freeze"]["trainOnly"].pop()
    runner.stage_check()


def test_mooncake_pd_needs_the_rdma_devices_on_every_node(tmp_path):
    recipe = dict(recipe_for(tmp_path), rollout={"pd": {"prefillGpus": 2, "rdmaDevices": ["rdma0", "rdma1"]}})
    run_dir = tmp_path / "run"
    (run_dir / "data").mkdir(parents=True)
    (run_dir / "data" / "train.jsonl").write_text("{}\n")
    emitter = metrics.Emitter("pt-x", "a1", None, out=io.StringIO())
    nodes = [{"host": "m2m-001", "memory": 3e12, "rdma": ["rdma0", "rdma1"]},
             {"host": "m2m-002", "memory": 3e12, "rdma": ["rdma0"]}]
    runner = stages.Runner(recipe, str(run_dir), FakeAdapter(), emitter, gpus=lambda: 8, nodes=lambda: nodes)
    with pytest.raises(stages.StageError) as e:
        runner.stage_check()
    assert e.value.reason == "Mooncake PD needs the RDMA devices rdma0, rdma1 on every node: m2m-002 lacks rdma1"
    assert not e.value.retry
    nodes[1]["rdma"].append("rdma1")
    runner.stage_check()
