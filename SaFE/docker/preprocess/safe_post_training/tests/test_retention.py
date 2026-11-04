#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import io
import json
import os

from safe_post_training import metrics, stages
from safe_post_training.tests.test_launch import FakeAdapter, markers


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
    save(tmp_path, 4, 8, 12, 16, 20, 24)
    r.val_losses = {4: 1.2, 8: 1.1, 12: 1.15, 16: 1.3, 20: 1.25, 24: 1.0}
    r.apply_retention(20)
    assert kept(tmp_path) == [8, 16, 20, 24], "24 is still being written past the latest saved iteration"
    assert kept(tmp_path, "critic_checkpoints") == [8, 16, 20, 24], "critic checkpoints go at the same steps"
    notice = [m[1] for m in markers(out) if m[0] == "[STAGE]"][-1]
    assert notice["state"] == "retention" and notice["detail"] == {"deleted": [4, 12], "kept": [8, 16, 20]}


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
