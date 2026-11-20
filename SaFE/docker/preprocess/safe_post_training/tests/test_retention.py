#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import collections
import errno
import io
import json
import os
import shutil
import sys

import pytest

from safe_post_training import launch, metrics, stages
from safe_post_training.frameworks.slime.adapter import Adapter as SlimeAdapter
from safe_post_training.tests.test_launch import FakeAdapter, markers, recipe_for, run


def runner(tmp_path, recipe):
    out = io.StringIO()
    emitter = metrics.Emitter("pt-x", "a1", str(tmp_path / "metrics.jsonl"), include=["eval/*/*"], out=out)
    return stages.Runner(recipe, str(tmp_path), FakeAdapter(), emitter), out


def save(tmp_path, *steps):
    for step in steps:
        for sub in ("checkpoints", "critic_checkpoints"):
            os.makedirs(tmp_path / sub / ("iter_%07d" % step), exist_ok=True)


def kept(tmp_path, sub="checkpoints"):
    return stages.saved_iterations(str(tmp_path / sub))


def test_keep_last_two_and_the_best(tmp_path):
    r, out = runner(tmp_path, {"trainType": "sft", "checkpoint": {"keepLast": 2, "keepBest": True}})
    save(tmp_path, 3, 7, 11, 15, 19, 23)
    # The validation after rollout i, saved as iter_i, is recorded at step i + 1.
    r.val_losses = {4: 1.2, 8: 1.1, 12: 1.15, 16: 1.3, 20: 1.25, 24: 1.0}
    r.apply_retention(19)
    assert kept(tmp_path) == [7, 15, 19, 23], "23 is still being written past the latest saved iteration"
    assert kept(tmp_path, "critic_checkpoints") == [7, 15, 19, 23], "critic checkpoints go at the same steps"
    notice = [m[1] for m in markers(out) if m[0] == "[STAGE]"][-1]
    assert notice["state"] == "retention" and notice["detail"] == {"deleted": [3, 11], "kept": [7, 15, 19]}


class PPOAdapter(SlimeAdapter):
    """Reads slime's checkpoint trackers and trains with a command that does nothing."""

    def train_command(self, recipe, run_dir):
        return [sys.executable, "-c", "pass"], {}, {}


PPO = {"trainType": "rl", "critic": {"checkpoints": "critic_checkpoints"}, "checkpoint": {"keepLast": 1}}


def track(tmp_path, sub, step):
    (tmp_path / sub / "latest_checkpointed_iteration.txt").write_text(str(step))


def test_ppo_retention_keeps_the_iteration_actor_and_critic_share(tmp_path):
    r, _ = runner(tmp_path, PPO)
    r.adapter = PPOAdapter()
    save(tmp_path, 9, 19)
    track(tmp_path, "checkpoints", 19)
    track(tmp_path, "critic_checkpoints", 9)
    r.apply_retention(19)
    assert kept(tmp_path) == [9, 19] and kept(tmp_path, "critic_checkpoints") == [9, 19], \
        "the critic is still writing iter_19, so iter_9 is the only one both can resume from"
    track(tmp_path, "critic_checkpoints", 19)
    r.apply_retention(19)
    assert kept(tmp_path) == [19] and kept(tmp_path, "critic_checkpoints") == [19]


def test_a_ppo_resume_falls_back_to_the_iteration_actor_and_critic_share(tmp_path):
    r, out = runner(tmp_path, PPO)
    r.adapter = PPOAdapter()
    save(tmp_path, 9, 19)
    track(tmp_path, "checkpoints", 19)
    track(tmp_path, "critic_checkpoints", 9)
    r.stage_train()
    assert r.adapter.latest_checkpoint(str(tmp_path)) == 9
    assert r.adapter.latest_checkpoint(str(tmp_path), "critic_checkpoints") == 9
    assert kept(tmp_path) == [9] and kept(tmp_path, "critic_checkpoints") == [9], "the unmatched iter_19 goes"
    stages_seen = [m[1] for m in markers(out) if m[0] == "[STAGE]"]
    assert [s["detail"] for s in stages_seen if s.get("state") == "checkpoint"] == \
        [{"actor": 19, "critic": 9, "resume": 9}]
    assert any(s.get("detail") == {"resumeStep": 9} for s in stages_seen)

    shutil.rmtree(tmp_path / "critic_checkpoints")
    r.stage_train()
    assert r.adapter.latest_checkpoint(str(tmp_path)) is None and kept(tmp_path) == [], \
        "without a shared iteration both start over"


def test_a_ppo_resume_keeps_the_checkpoints_when_a_tracker_gives_no_iteration(tmp_path):
    """A tracker that is there and names no iteration fails the resume and removes no checkpoint."""
    r, _ = runner(tmp_path, PPO)
    r.adapter = PPOAdapter()
    save(tmp_path, 9, 19)
    track(tmp_path, "checkpoints", 19)
    tracker = tmp_path / "critic_checkpoints" / "latest_checkpointed_iteration.txt"
    tracker.write_text("")
    with pytest.raises(stages.StageError) as e:
        r.stage_train()
    assert not e.value.retry, "a crash while the tracker was rewritten leaves it empty for every attempt"
    assert kept(tmp_path) == [9, 19] and kept(tmp_path, "critic_checkpoints") == [9, 19]

    tracker.unlink()
    tracker.mkdir()
    with pytest.raises(stages.StageError) as e:
        r.stage_train()
    assert e.value.retry, "a tracker that cannot be read now may be readable on the next attempt"
    assert kept(tmp_path) == [9, 19] and kept(tmp_path, "critic_checkpoints") == [9, 19]


def test_a_resume_does_not_start_over_when_its_tracker_names_no_iteration(tmp_path):
    """A run without a critic reads its tracker the same way: one that is there and empty fails the run, and the
    training does not quietly begin again from the base weights."""
    r, _ = runner(tmp_path, {"trainType": "sft"})
    r.adapter = PPOAdapter()
    os.makedirs(tmp_path / "checkpoints" / "iter_0000009")
    (tmp_path / "checkpoints" / "latest_checkpointed_iteration.txt").write_text("")
    with pytest.raises(stages.StageError) as e:
        r.stage_train()
    assert not e.value.retry
    assert kept(tmp_path) == [9]


def test_a_ppo_resume_keeps_the_checkpoints_when_a_directory_cannot_be_listed(monkeypatch, tmp_path):
    """A checkpoint directory that cannot be listed is not one without checkpoints: the attempt fails, to be tried
    again, and removes nothing."""
    r, _ = runner(tmp_path, PPO)
    r.adapter = PPOAdapter()
    save(tmp_path, 9, 19)
    track(tmp_path, "checkpoints", 19)
    track(tmp_path, "critic_checkpoints", 9)
    listdir = os.listdir

    def failing(path):
        if str(path).endswith("critic_checkpoints"):
            raise OSError(errno.EIO, "Input/output error", path)
        return listdir(path)

    with monkeypatch.context() as m:
        m.setattr(stages.os, "listdir", failing)
        with pytest.raises(stages.StageError) as e:
            r.stage_train()
    assert e.value.retry
    assert kept(tmp_path) == [9, 19] and kept(tmp_path, "critic_checkpoints") == [9, 19]


def test_a_resumed_run_does_not_count_its_own_checkpoints_twice(monkeypatch, tmp_path):
    recipe = recipe_for(tmp_path)
    shard = tmp_path / "run" / "checkpoints" / "iter_0000001" / "shard.distcp"
    shard.parent.mkdir(parents=True)
    shard.write_bytes(b"x" * 1000)
    monkeypatch.setattr(stages, "weights_size", lambda path: 100)
    usage = collections.namedtuple("usage", "total used free")
    monkeypatch.setattr(shutil, "disk_usage", lambda path: usage(1 << 40, 0, 1500))
    code, _, ms = run(monkeypatch, recipe, FakeAdapter())
    assert code == 0, "the run needs about 2200 bytes, 1000 of which it already wrote: %s" % ms[-1:]


def test_rl_and_keep_all(tmp_path):
    r, _ = runner(tmp_path, {"trainType": "rl", "checkpoint": {"keepLast": 1, "keepBest": True}})
    save(tmp_path, 1, 2, 3)
    r.val_losses = {1: 0.1}
    r.apply_retention(3)
    assert kept(tmp_path) == [3], "RL has no validation loss to keep a best checkpoint by"

    r, _ = runner(tmp_path, {"trainType": "sft", "checkpoint": {"keepLast": 0}})
    save(tmp_path, 1, 2)
    r.apply_retention(3)
    assert kept(tmp_path) == [1, 2, 3]


SAVES = """
import os, sys
for step in (4, 9, 11):
    os.makedirs(os.path.join(sys.argv[1], "checkpoints", "iter_%07d" % step))
    with open(os.path.join(sys.argv[1], "checkpoints", "latest_checkpointed_iteration.txt"), "w") as f:
        f.write(str(step))
"""


class SavingAdapter(SlimeAdapter):
    """Reads slime's checkpoint trackers and trains with a command that saves three checkpoints and ends."""

    def train_command(self, recipe, run_dir):
        return [sys.executable, "-c", SAVES, run_dir], {}, {}


def test_retention_runs_for_a_save_the_training_ended_right_after(tmp_path):
    """A training that ends within a tick of its last save still has the retention run for that save, with a
    watchdog that never got to tick and with none."""
    for name in ("watched", "unwatched"):
        (tmp_path / name).mkdir()
        r, out = runner(tmp_path / name, {"trainType": "sft", "checkpoint": {"keepLast": 1}})
        r.adapter = SavingAdapter()
        if name == "watched":
            r.watchdog = launch.Watchdog(r.emitter, interval=3600)
        r.stage_train()
        assert kept(tmp_path / name) == [11], name
        notices = [m[1] for m in markers(out) if m[0] == "[STAGE]"]
        assert [n["detail"] for n in notices if n.get("state") == "retention"] == \
            [{"deleted": [4, 9], "kept": [11]}], name


def test_validation_losses_survive_a_restart(tmp_path):
    path = tmp_path / "metrics.jsonl"
    path.write_text("\n".join([json.dumps({"step": 8, "v": {"eval/val/loss": 1.1, "train/loss": 2}}),
                               "not json", json.dumps({"step": 9, "v": {"eval/val/loss": "NaN"}})]) + "\n")
    assert stages.load_val_losses(str(path)) == {8: 1.1}
    assert stages.load_val_losses(str(tmp_path / "missing.jsonl")) == {}

    r, _ = runner(tmp_path, {"trainType": "sft"})
    r.parse_line("x - step 12: {'eval/val/loss': 0.9, 'train/step': 12}\n")
    assert r.val_losses == {12: 0.9}


def test_space_needed_counts_the_checkpoints_kept_together(monkeypatch, tmp_path):
    monkeypatch.setattr(stages, "weights_size", lambda path: 10)
    model = {"path": str(tmp_path)}
    every_save = {"model": model, "choices": {"numRollout": 1200, "saveInterval": 200}, "checkpoint": {"keepLast": 0}}
    assert stages.Runner(every_save, str(tmp_path), None, None).space_needed() == 10 * 7 * 6 + 10
    policy = dict(every_save, checkpoint={"keepLast": 2, "keepBest": True})
    assert stages.Runner(policy, str(tmp_path), None, None).space_needed() == 10 * 7 * 4 + 10
    epochs = {"model": model, "choices": {"numEpoch": 3}}
    assert stages.Runner(epochs, str(tmp_path), None, None).space_needed() == 10 * 7 * 3 + 10
    frozen = dict(policy, checkpointTB=1.34)
    assert stages.Runner(frozen, str(tmp_path), None, None).space_needed() == int(1.34e12) * 4 + 10, \
        "the spec measured a trainable-only checkpoint"
