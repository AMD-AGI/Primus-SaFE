#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import math

import pytest

from safe_post_training.frameworks.slime.adapter import Adapter
from safe_post_training.stages import StageError

ACTOR = "\x1b[36m(MegatronTrainRayActor pid=2414, ip=10.245.153.145)\x1b[0m "
ROLLOUT = "\x1b[36m(RolloutManager pid=30706)\x1b[0m "
# Lines from the J2 (GLM-5.3 SFT) main log on Spur, cut to a few keys.
STEP = ACTOR + ("[2026-10-26 06:25:06] model.py:955 - step 0: {'train/loss': 0.7292641418646737, "
                "'train/grad_norm': 3.995555877685547, 'train/lr-pg_0': 9.98951413118856e-06, "
                "'train/global_batch_size': 8, 'train/step': 0}")
EVAL = ACTOR + "[2026-10-26 06:21:00] actor.py:493 - eval 0: {'eval/dev/loss': 0.5517232246126252, 'eval/step': 0}"
PERF = ROLLOUT + ("[2026-10-26 06:21:01] rollout_metrics.py:286 - perf 0: {'rollout/response_len/mean': 764.125, "
                  "'rollout/response_len/max': 2156, 'rollout/zero_std/count_0': 8}")


def test_parse_real_lines():
    a = Adapter(env={})
    kind, axis, step, values = a.parse_metric_line(STEP)
    assert (kind, axis, step) == ("step", "train", 0)
    assert values == {"train/loss": 0.7292641418646737, "train/grad_norm": 3.995555877685547,
                      "train/lr-pg_0": 9.98951413118856e-06, "train/global_batch_size": 8.0}
    assert a.parse_metric_line(EVAL) == ("eval", "rollout", 0, {"eval/dev/loss": 0.5517232246126252})
    kind, axis, step, values = a.parse_metric_line(PERF)
    assert (kind, axis, step) == ("perf", "rollout", 0) and values["rollout/response_len/max"] == 2156.0


def test_sft_eval_steps_follow_training_steps():
    a = Adapter(env={})
    a.train_type = "sft"
    eval_line = "x - eval %d: {'eval/val/loss': 0.5, 'eval/step': %d}"
    assert [a.parse_metric_line(eval_line % (n, n))[1:3] for n in (0, 9, 19)] == \
        [("train", 0), ("train", 10), ("train", 20)], "J2's eval 0, 9 and 19 sit at steps 0, 10 and 20"


def test_parse_non_finite_and_other_lines():
    a = Adapter(env={})
    _, _, _, values = a.parse_metric_line("x - step 3: {'train/loss': nan, 'train/grad_norm': inf, 'train/x': -inf}")
    assert math.isnan(values["train/loss"]) and values["train/grad_norm"] == math.inf and values["train/x"] == -math.inf
    _, _, step, _ = a.parse_metric_line("critic-step 7: {'train/critic-value_loss': 0.1}")
    assert step == 7
    assert a.parse_metric_line("INFO loading weights") is None
    with pytest.raises((ValueError, SyntaxError)):
        a.parse_metric_line("x - step 3: {'train/loss': }")


def test_latest_checkpoint(tmp_path):
    a = Adapter(env={})
    assert a.latest_checkpoint(str(tmp_path)) is None
    (tmp_path / "checkpoints").mkdir()
    (tmp_path / "checkpoints" / "latest_checkpointed_iteration.txt").write_text("19\n")
    assert a.latest_checkpoint(str(tmp_path)) == 19


def test_train_command_uses_the_model_script(tmp_path):
    root = tmp_path / "slime"
    (root / "scripts" / "models").mkdir(parents=True)
    (root / "train.py").write_text("")
    (root / "scripts" / "models" / "qwen3-4B.sh").write_text('MODEL_ARGS=(\n --num-layers 36\n --hidden-size 2560\n)\n')
    a = Adapter(env={"SLIME_DIR": str(root), "MEGATRON_DIR": "/m"})
    recipe = {"model": {"script": "scripts/models/qwen3-4B.sh"}, "frameworkArgs": ["--lr", "1e-05"],
              "files": {"plugins.yaml": "x: 1"}}
    cmd, env, files = a.train_command(recipe, str(tmp_path))
    assert cmd[1:] == [str(root / "train.py"), "--num-layers", "36", "--hidden-size", "2560", "--lr", "1e-05"]
    assert env["PYTHONPATH"].split(":")[:2] == [str(root), "/m"]
    assert files == {"plugins.yaml": "x: 1"}
    with pytest.raises(StageError, match="does not exist in the image"):
        a.train_command({"model": {"script": "scripts/models/none.sh"}, "frameworkArgs": []}, str(tmp_path))


def test_export_names_a_checkpoint(tmp_path):
    root = tmp_path / "slime"
    (root / "tools").mkdir(parents=True)
    (root / "train.py").write_text("")
    (tmp_path / "base").mkdir()
    (tmp_path / "base" / "tokenizer.json").write_text("{}")
    (tmp_path / "checkpoints" / "iter_0000008").mkdir(parents=True)
    commands = []

    def run_command(cmd, env=None):
        commands.append(cmd)
        return 0

    a = Adapter(env={"SLIME_DIR": str(root)})
    recipe = {"model": {"path": str(tmp_path / "base")}}
    out = a.export(recipe, str(tmp_path), run_command, iteration=8, out=str(tmp_path / "exports" / "step-8"))
    assert out == str(tmp_path / "exports" / "step-8")
    assert commands[0][commands[0].index("--input-dir") + 1] == str(tmp_path / "checkpoints" / "iter_0000008")
    assert (tmp_path / "exports" / "step-8" / "tokenizer.json").exists()
    with pytest.raises(StageError, match="iteration 4 no longer exists") as e:
        a.export(recipe, str(tmp_path), run_command, iteration=4)
    assert not e.value.retry
