#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import ast
import io
import json
import os
import re
import sys

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

    def export(self, recipe, run_dir, run_command):
        out = os.path.join(run_dir, "final_model")
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
