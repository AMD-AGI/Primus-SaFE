#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import ast
import http.server
import io
import json
import os
import re
import sys
import threading

from safe_post_training import launch


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

    def latest_checkpoint(self, run_dir):
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
    code = launch.run(recipe, out=out, gpus=lambda: gpus, sleep=lambda s: None, sampler=None)
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


def test_checkpoint_export_runs_only_the_export(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    code, _, _ = run(monkeypatch, recipe, FakeAdapter())
    assert code == 0
    status = (tmp_path / "run" / "status.json").read_text()

    export_dir = tmp_path / "run" / "exports" / "step-1"
    recipe["exportOnly"] = {"step": 1, "dir": str(export_dir)}
    adapter = FakeAdapter()
    code, _, ms = run(monkeypatch, recipe, adapter)
    assert code == 0 and adapter.exported == 1
    assert {m[1]["stage"] for m in ms if m[0] == "[STAGE]"} == {"start", "export"}, "no other stage runs"
    assert ms[-1][0] == "[SUCCESS]" and ms[-1][1]["output"] == str(export_dir) and ms[-1][1]["attempt"] == "x1"
    assert (export_dir / ".done").exists() and json.loads((export_dir / "result.json").read_text())["iteration"] == 1
    assert (tmp_path / "run" / "status.json").read_text() == status, "the run's attempts stay as they were"
    assert json.loads((tmp_path / "run" / "result.json").read_text())["output"] == str(tmp_path / "run" / "final_model")

    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 0 and ms[-1][1]["output"] == str(export_dir), "a finished export is reported again"


class PPOAdapter(FakeAdapter):
    """A fake adapter whose actor and critic checkpoints end at the given steps."""

    def __init__(self, actor, critic):
        super().__init__()
        self.steps = {"checkpoints": actor, "critic_checkpoints": critic}

    def latest_checkpoint(self, run_dir, sub="checkpoints"):
        return self.steps[sub]


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
    code, _, ms = run(monkeypatch, recipe, PPOAdapter(40, 30))
    assert code != 0
    assert ms[-1][0] == "[ERROR]" and ms[-1][1]["stage"] == "train"
    assert ms[-1][1]["reason"] == "the actor checkpoints end at step 40 but the critic checkpoints at step 30"
    assert ms[-1][1]["retry"] is False, "a rerun would not bring them back in step"
