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


class FP8Adapter(CountingAdapter):
    """Also quantizes to FP8 by writing a weights file, and counts those conversions."""

    def __init__(self):
        super().__init__()
        self.fp8 = 0

    def convert_fp8(self, recipe, run_dir, run_command, out):
        self.fp8 += 1
        os.makedirs(out, exist_ok=True)
        with open(os.path.join(out, "model.safetensors"), "w") as f:
            f.write("y" * 4)


def test_an_fp8_rollout_adds_its_weights_to_the_shared_conversion(tmp_path):
    adapter = FP8Adapter()
    bf16, out = setup(tmp_path, "r1")
    bf16.adapter = adapter
    bf16.stage_convert()
    assert (adapter.conversions, adapter.fp8) == (1, 0)
    assert convert_markers(out)[-1][1]["fp8"] is False

    fp8, out = setup(tmp_path, "r2")
    fp8.recipe["convert"]["fp8"], fp8.adapter = True, adapter
    assert not fp8.reuse_conversion(), "the conversion is there, its FP8 weights are not"
    fp8.stage_convert()
    assert (adapter.conversions, adapter.fp8) == (1, 1), "only the FP8 weights are made"
    state, detail = convert_markers(out)[-1]
    assert state == "done" and detail["fp8"] is True and detail["sizeBytes"] == 14

    again, _ = setup(tmp_path, "r3")
    again.recipe["convert"]["fp8"], again.adapter = True, adapter
    assert again.reuse_conversion() and adapter.fp8 == 1
    plain, out = setup(tmp_path, "r4")
    assert plain.reuse_conversion()
    assert convert_markers(out)[-1][1]["fp8"] is True, "a BF16 run does not clear the record of the FP8 weights"


def test_an_hf_direct_spec_only_makes_the_fp8_weights(tmp_path):
    adapter = FP8Adapter()
    first, out = setup(tmp_path, "r1")
    first.recipe["convert"].update(hfDirect=True, fp8=True)
    first.adapter = adapter
    assert not first.reuse_conversion()
    first.stage_convert()
    assert (adapter.conversions, adapter.fp8) == (0, 1), "training loads the HF weights; only FP8 is made"
    state, detail = convert_markers(out)[-1]
    assert state == "done" and detail["fp8"] is True and detail["sizeBytes"] == 4
    assert os.path.exists(os.path.join(detail["dir"], stages.SOURCE_FILE))

    second, out = setup(tmp_path, "r2")
    second.recipe["convert"].update(hfDirect=True, fp8=True)
    second.adapter = adapter
    assert second.reuse_conversion() and adapter.fp8 == 1
    assert convert_markers(out) == [("skipped", detail)]
