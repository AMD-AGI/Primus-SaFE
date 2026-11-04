#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import enum
import io
import json
import types

from safe_post_training import metrics, stages
from safe_post_training.frameworks.slime.plugins import sample_log
from safe_post_training.tests.test_launch import FakeAdapter, markers, recipe_for, run


class Status(enum.Enum):
    COMPLETED = "completed"
    TRUNCATED = "truncated"


def sample(group, index, reward=1.0):
    return types.SimpleNamespace(group_index=group, index=index, prompt=[{"role": "user", "content": "2+2?"}],
                                 response="4", label=4, reward=reward, status=Status.COMPLETED, response_length=1)


def run_dir_with(tmp_path, samples):
    tmp_path.mkdir(parents=True, exist_ok=True)
    (tmp_path / "recipe.json").write_text(json.dumps({"metrics": {"samples": samples}}))
    return types.SimpleNamespace(save=str(tmp_path / "checkpoints"), rollout_seed=7)


def lines(path):
    return [json.loads(line) for line in path.read_text().splitlines()]


def test_rollout_groups_are_sampled_whole(tmp_path):
    args = run_dir_with(tmp_path, {"rolloutGroups": 2, "evalPerSet": 3})
    samples = [sample(g, g * 4 + i, reward=float(i % 2)) for g in range(6) for i in range(4)]
    assert sample_log.log_rollout(3, args, samples, {}, 1.0) is False, "slime still logs its own metrics"
    out = lines(tmp_path / "samples" / "rollout-3.jsonl")
    assert len(out) == 8 and len({r["group"] for r in out}) == 2, "two groups, every response of each"
    assert out[0]["prompt"] == "user: 2+2?" and out[0]["status"] == "completed" and out[0]["label"] == "4"
    sample_log.log_rollout(3, args, samples, {}, 1.0)
    assert lines(tmp_path / "samples" / "rollout-3.jsonl") == out, "the rollout seed and id fix the groups"

    data = {"aime": {"samples": [sample(0, i) for i in (9, 2, 5, 1, 7)]}}
    assert sample_log.log_eval(3, args, data, {}) is False
    assert [r["index"] for r in lines(tmp_path / "samples" / "eval-3.jsonl")] == [1, 2, 5]

    off = run_dir_with(tmp_path / "off", {"rolloutGroups": 0, "evalPerSet": 0})
    sample_log.log_rollout(1, off, samples)
    assert not (tmp_path / "off" / "samples").exists()


def test_launcher_reports_truncated_samples(tmp_path):
    out = io.StringIO()
    emitter = metrics.Emitter("pt-x", "a1", None, out=out)
    limits = {"promptChars": 10, "responseHeadChars": 5, "responseTailChars": 5}
    r = stages.Runner({"metrics": {"samples": limits}}, str(tmp_path), FakeAdapter(), emitter)
    (tmp_path / "samples").mkdir()
    record = {"dataset": "", "group": 1, "index": 0, "prompt": "p" * 30, "response": "head-" + "x" * 50 + "-tail",
              "label": "4", "reward": float("nan"), "status": "truncated", "tokens": 4096}
    (tmp_path / "samples" / "rollout-12.jsonl").write_text(json.dumps(record) + "\n")
    (tmp_path / "samples" / "rollout-13.jsonl.tmp").write_text(json.dumps(record) + "\n")
    r.sample_tick()
    r.sample_tick()
    got = [m[1] for m in markers(out) if m[0] == "[SAMPLE]"]
    assert len(got) == 1, "a file is reported once, and a file still being written not at all"
    s = got[0]
    assert s["kind"] == "rollout" and s["step"] == 12 and "seq" not in s
    assert s["prompt"] == "p" * 10 + " … [20 characters omitted] … "
    assert s["response"] == "head- … [50 characters omitted] … -tail"
    assert s["promptChars"] == 30 and s["responseChars"] == 60 and s["reward"] is None


def test_a_file_reports_at_most_one_megabyte(tmp_path):
    out = io.StringIO()
    emitter = metrics.Emitter("pt-x", "a1", None, out=out)
    r = stages.Runner({"metrics": {"samples": {}}}, str(tmp_path), FakeAdapter(), emitter)
    (tmp_path / "samples").mkdir()
    big = {"group": 0, "index": 0, "prompt": "q", "response": "r" * 3900, "status": "completed"}
    (tmp_path / "samples" / "eval-1.jsonl").write_text("\n".join(json.dumps(dict(big, index=i)) for i in range(400)))
    r.sample_tick()
    reported = [line for line in out.getvalue().splitlines() if line.startswith("[SAMPLE]")]
    assert 200 < len(reported) < 400
    assert sum(len(line) for line in reported) < (1 << 20) + 400 * 64


def test_launcher_writes_the_recipe_for_the_plugin(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    code, _, _ = run(monkeypatch, recipe, FakeAdapter())
    assert code == 0
    assert json.loads((tmp_path / "run" / "recipe.json").read_text())["runId"] == "pt-x"
