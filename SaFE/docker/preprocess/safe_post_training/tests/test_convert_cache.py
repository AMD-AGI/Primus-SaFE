#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import io
import json
import os
import time

import pytest

from safe_post_training import metrics, stages


class CountingAdapter:
    """Converts by creating the output directory and counts its conversions."""

    def __init__(self):
        self.conversions = 0

    def convert(self, recipe, run_dir, run_command):
        self.conversions += 1
        os.makedirs(recipe["convert"]["dir"], exist_ok=True)
        with open(os.path.join(recipe["convert"]["dir"], "model.pt"), "w") as f:
            f.write("x" * 10)


def setup(tmp_path, run):
    """Return a runner of run over a shared cache, with the emitter output."""
    model = tmp_path / "model"
    model.mkdir(exist_ok=True)
    (model / "config.json").write_text("{}")
    (model / "model.safetensors").write_text("weights")
    cache = tmp_path / "custom" / "models" / "post-training-cache" / "slime-qwen3-4B-m1-abc"
    recipe = {"model": {"path": str(model)},
              "convert": {"cacheKey": "slime-qwen3-4B-m1-abc", "dir": str(cache / "torch_dist")}}
    out = io.StringIO()
    emitter = metrics.Emitter("pt-" + run, "a1", out=out)
    return stages.Runner(recipe, str(tmp_path / run), None, emitter), out


def convert_markers(out):
    """Return the (state, detail) of the convert [STAGE] markers printed to out."""
    found = []
    for line in out.getvalue().splitlines():
        body = json.loads(line[len("[STAGE] "):])
        if body.get("stage") == "convert":
            found.append((body.get("state"), body.get("detail")))
    return found


def test_runs_of_a_model_share_one_conversion(tmp_path):
    first, out = setup(tmp_path, "r1")
    first.adapter = adapter = CountingAdapter()
    assert not first.reuse_conversion()
    first.stage_convert()
    state, detail = convert_markers(out)[-1]
    assert state == "done" and detail["cacheKey"] == "slime-qwen3-4B-m1-abc" and detail["sizeBytes"] == 10
    assert os.path.exists(os.path.join(detail["dir"], stages.SOURCE_FILE))

    second, out = setup(tmp_path, "r2")
    second.adapter = adapter
    assert second.reuse_conversion()
    assert convert_markers(out) == [("skipped", detail)]
    assert adapter.conversions == 1


def test_changed_weights_do_not_reuse_the_cache(tmp_path):
    runner, _ = setup(tmp_path, "r1")
    runner.adapter = CountingAdapter()
    runner.stage_convert()
    with open(os.path.join(runner.recipe["model"]["path"], "model.safetensors"), "w") as f:
        f.write("retrained weights")
    with pytest.raises(stages.StageError) as err:
        runner.reuse_conversion()
    assert err.value.reason == "the conversion cache does not match the model weights" and not err.value.retry


def test_a_waiting_run_reuses_what_the_lock_holder_converted(tmp_path):
    runner, out = setup(tmp_path, "r2")
    runner.adapter = adapter = CountingAdapter()
    _, root, complete = runner.convert_paths()
    os.makedirs(os.path.join(root, stages.CONVERT_LOCK))
    holder, _ = setup(tmp_path, "r1")

    def other_run_finishes(_):
        holder.adapter = CountingAdapter()
        os.rmdir(os.path.join(root, stages.CONVERT_LOCK))
        holder.stage_convert()

    runner.sleep = other_run_finishes
    runner.stage_convert()
    assert adapter.conversions == 0, "the run converts nothing once the lock holder is done"
    assert os.path.exists(complete) and convert_markers(out)[-1][0] == "done"


def test_a_stale_lock_is_taken_over(tmp_path):
    runner, _ = setup(tmp_path, "r1")
    runner.adapter = adapter = CountingAdapter()
    _, root, _ = runner.convert_paths()
    lock = os.path.join(root, stages.CONVERT_LOCK)
    os.makedirs(lock)
    old = time.time() - stages.LOCK_STALE_SECONDS - 60
    os.utime(lock, (old, old))
    runner.sleep = lambda _: pytest.fail("a stale lock is not waited for")
    runner.stage_convert()
    assert adapter.conversions == 1 and not os.path.exists(lock)
