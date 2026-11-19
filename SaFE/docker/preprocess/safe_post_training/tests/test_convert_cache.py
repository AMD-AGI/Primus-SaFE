#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import errno
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
    with open(os.path.join(lock, stages.LOCK_OWNER), "w") as f:
        json.dump({"run": "pt-r9", "attempt": "a1", "token": "gone"}, f)
    old = time.time() - stages.LOCK_STALE_SECONDS - 60
    os.utime(lock, (old, old))
    runner.sleep = lambda _: pytest.fail("a stale lock is not waited for")
    runner.stage_convert()
    assert adapter.conversions == 1 and not os.path.exists(lock)


def test_a_lock_left_by_an_earlier_attempt_of_the_run_is_taken_over(tmp_path):
    runner, _ = setup(tmp_path, "r1")
    runner.emitter.attempt = "a2"
    runner.adapter = adapter = CountingAdapter()
    _, root, _ = runner.convert_paths()
    lock = os.path.join(root, stages.CONVERT_LOCK)
    os.makedirs(lock)
    with open(os.path.join(lock, stages.LOCK_OWNER), "w") as f:
        json.dump({"run": "pt-r1", "attempt": "a1", "token": "old"}, f)
    runner.sleep = lambda _: pytest.fail("the run does not wait for its own earlier attempt")
    runner.stage_convert()
    assert adapter.conversions == 1 and not os.path.exists(lock)


def test_the_holder_keeps_its_lock_fresh_and_removes_only_its_own(monkeypatch, tmp_path):
    monkeypatch.setattr(stages, "LOCK_HEARTBEAT_SECONDS", 0.05)
    root = str(tmp_path / "cache")
    lock = os.path.join(root, stages.CONVERT_LOCK)
    with stages.cache_lock(root, owner={"run": "pt-r1", "attempt": "a1"}):
        owner = json.load(open(os.path.join(lock, stages.LOCK_OWNER)))
        assert owner["run"] == "pt-r1" and owner["attempt"] == "a1" and owner["token"]
        old = time.time() - stages.LOCK_STALE_SECONDS - 60
        os.utime(lock, (old, old))
        time.sleep(0.5)
        assert time.time() - os.path.getmtime(lock) < stages.LOCK_STALE_SECONDS, "a live holder refreshes its lock"
        with open(os.path.join(lock, stages.LOCK_OWNER), "w") as f:
            json.dump({"run": "pt-r2", "attempt": "a1", "token": "other"}, f)
    assert os.path.exists(lock), "a lock that another run took over is left to it"


def test_the_holder_keeps_beating_while_its_owner_record_cannot_be_read(monkeypatch, tmp_path):
    """One failed read of the owner record does not end the heartbeat, which would hand the lock to a waiting run."""
    monkeypatch.setattr(stages, "LOCK_HEARTBEAT_SECONDS", 0.05)
    root = str(tmp_path / "cache")
    lock = os.path.join(root, stages.CONVERT_LOCK)
    with stages.cache_lock(root, owner={"run": "pt-r1", "attempt": "a1"}) as held:
        record = os.path.join(lock, stages.LOCK_OWNER)
        saved = open(record).read()
        os.remove(record)
        time.sleep(0.3)
        assert held(), "a record that cannot be read is not another run's"
        with open(record, "w") as f:
            f.write(saved)
        old = time.time() - stages.LOCK_STALE_SECONDS - 60
        os.utime(lock, (old, old))
        time.sleep(0.5)
        assert time.time() - os.path.getmtime(lock) < stages.LOCK_STALE_SECONDS


def test_a_run_that_found_a_lock_stale_leaves_the_lock_another_run_took_meanwhile(tmp_path):
    """Of the runs that find a lock stale only one takes it, and one that comes late leaves the new holder's."""
    lock = tmp_path / stages.CONVERT_LOCK
    lock.mkdir()
    fresh = {"run": "pt-a", "attempt": "a1", "token": "fresh"}
    (lock / stages.LOCK_OWNER).write_text(json.dumps(fresh))
    late = {"run": "pt-b", "attempt": "a1", "token": "late"}
    assert not stages.take_over(str(lock), {"run": "pt-gone", "token": "stale"}, late)
    assert json.loads((lock / stages.LOCK_OWNER).read_text()) == fresh, "the holder keeps its record"
    assert stages.take_over(str(lock), fresh, late), "the record it found stale is the one it takes"
    assert json.loads((lock / stages.LOCK_OWNER).read_text()) == late
    assert sorted(os.listdir(lock)) == [stages.LOCK_OWNER], "nothing of the takeover is left in the lock"


def test_a_lock_whose_owner_record_cannot_be_written_is_not_left_behind(monkeypatch, tmp_path):
    """A lock without an owner record would be waited for six hours."""
    root = str(tmp_path / "cache")

    def full(lock, owner):
        raise OSError(errno.ENOSPC, "No space left on device")

    monkeypatch.setattr(stages, "write_owner", full)
    with pytest.raises(OSError):
        with stages.cache_lock(root, owner={"run": "pt-r1", "attempt": "a1"}):
            pytest.fail("the lock is not held")
    assert not os.path.exists(os.path.join(root, stages.CONVERT_LOCK))


class Waited(Exception):
    """Raised by a runner's sleep, to end a wait for a lock."""


def wait(_):
    """Stand in for a runner's sleep."""
    raise Waited()


def test_a_lock_without_an_owner_record_is_waited_for_longer(tmp_path):
    """A launcher from before the heartbeat holds a lock without a record, which is not refreshed while it converts."""
    runner, _ = setup(tmp_path, "r1")
    runner.adapter = adapter = CountingAdapter()
    _, root, _ = runner.convert_paths()
    lock = os.path.join(root, stages.CONVERT_LOCK)
    os.makedirs(lock)
    old = time.time() - stages.LOCK_STALE_SECONDS - 60
    os.utime(lock, (old, old))
    runner.sleep = wait
    with pytest.raises(Waited):
        runner.stage_convert()
    assert adapter.conversions == 0 and os.path.exists(lock)
    old = time.time() - stages.LOCK_UNOWNED_STALE_SECONDS - 60
    os.utime(lock, (old, old))
    runner.stage_convert()
    assert adapter.conversions == 1 and not os.path.exists(lock)


class TakenOverAdapter(CountingAdapter):
    """Converts while another run takes the lock over."""

    def convert(self, recipe, run_dir, run_command):
        super().convert(recipe, run_dir, run_command)
        with open(os.path.join(self.lock, stages.LOCK_OWNER), "w") as f:
            json.dump({"run": "pt-r2", "attempt": "a1", "token": "other"}, f)


def test_a_conversion_whose_lock_was_taken_over_is_not_marked_complete(tmp_path):
    """Two runs then wrote the same cache directory, so neither result is recorded as usable."""
    runner, _ = setup(tmp_path, "r1")
    runner.adapter = adapter = TakenOverAdapter()
    _, root, complete = runner.convert_paths()
    adapter.lock = os.path.join(root, stages.CONVERT_LOCK)
    with pytest.raises(stages.StageError) as e:
        runner.stage_convert()
    assert e.value.retry and not os.path.exists(complete)
    assert os.path.exists(adapter.lock), "the lock stays with the run that took it over"


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
