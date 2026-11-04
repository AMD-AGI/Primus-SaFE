#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import pytest

from safe_post_training.frameworks.slime.adapter import Adapter
from safe_post_training.frameworks.slime.plugins import lora, lora_sync, merge_lora

QKV = "decoder.layers.self_attention.linear_qkv."


def test_base_keys_move_under_the_wrapper():
    state = {QKV + "weight": "W", QKV + "layer_norm_weight": "N", "decoder.final_layernorm.weight": "F"}
    lora.remap_keys(state, QKV, {"linear_in.weight": "A0", "linear_out.weight": "B0"})
    assert state == {QKV + "to_wrap.weight": "W", QKV + "to_wrap.layer_norm_weight": "N",
                     QKV + "adapter.linear_in.weight": "A0", QKV + "adapter.linear_out.weight": "B0",
                     "decoder.final_layernorm.weight": "F"}, "a base checkpoint keeps the initial adapters"

    resumed = {QKV + "weight": "W", QKV + "adapter.linear_in.weight": "A1", QKV + "adapter.linear_out.weight": "B1"}
    lora.remap_keys(resumed, QKV, {"linear_in.weight": "A0", "linear_out.weight": "B0"})
    assert resumed[QKV + "adapter.linear_in.weight"] == "A1", "a LoRA checkpoint's adapters are loaded"
    assert QKV + "weight" not in resumed


def test_checkpoint_path(tmp_path):
    assert lora.checkpoint_path(str(tmp_path)) == str(tmp_path)
    (tmp_path / "latest_checkpointed_iteration.txt").write_text("40\n")
    assert lora.checkpoint_path(str(tmp_path)) == str(tmp_path / "iter_0000040")
    (tmp_path / "latest_checkpointed_iteration.txt").write_text("release")
    assert lora.checkpoint_path(str(tmp_path)) == str(tmp_path / "release")
    assert lora.has_adapters("") is False


def test_hf_loading_sees_only_base_parameters():
    named = [("module.module.decoder.layers.0.self_attention.linear_qkv.to_wrap.weight", 1),
             ("module.module.decoder.layers.0.self_attention.linear_qkv.adapter.linear_in.weight", 2),
             ("module.module.embedding.word_embeddings.weight", 3)]
    assert list(lora.base_names(named)) == [("module.module.decoder.layers.0.self_attention.linear_qkv.weight", 1),
                                            ("module.module.embedding.word_embeddings.weight", 3)]


def test_merge_plan_pairs_every_adapter_with_its_weight():
    keys = [QKV + "weight", QKV + "adapter.linear_in.weight", QKV + "adapter.linear_out.weight",
            QKV + "adapter.linear_in.bias", "decoder.final_layernorm.weight"]
    pairs, drop = merge_lora.merge_plan(keys)
    assert pairs == [(QKV + "weight", QKV + "adapter.linear_in.weight", QKV + "adapter.linear_out.weight")]
    assert drop == [QKV + "adapter.linear_in.bias"]
    with pytest.raises(ValueError, match="no B matrix"):
        merge_lora.merge_plan([QKV + "weight", QKV + "adapter.linear_in.weight"])
    with pytest.raises(ValueError, match="no weight"):
        merge_lora.merge_plan([QKV + "adapter.linear_in.weight", QKV + "adapter.linear_out.weight"])


def test_lora_checkpoints_export_merged(tmp_path):
    root = tmp_path / "slime"
    (root / "tools").mkdir(parents=True)
    (root / "train.py").write_text("")
    (tmp_path / "base").mkdir()
    (tmp_path / "checkpoints" / "iter_0000008").mkdir(parents=True)
    commands = []

    def run_command(cmd, env=None):
        commands.append(cmd)
        return 0

    recipe = {"model": {"path": str(tmp_path / "base")}, "lora": {"rank": 16, "alpha": 32}}
    Adapter(env={"SLIME_DIR": str(root)}).export(recipe, str(tmp_path), run_command, iteration=8)
    cmd = commands[0]
    assert cmd[1].endswith("plugins/merge_lora.py")
    assert cmd[cmd.index("--rank") + 1] == "16" and cmd[cmd.index("--alpha") + 1] == "32"
    assert cmd[cmd.index("--input-dir") + 1] == str(tmp_path / "checkpoints" / "iter_0000008")


def test_a_base_load_starts_the_adapters_at_zero():
    state = {"linear_in.weight": "A", "linear_out.weight": "B"}
    assert lora.fill_adapters(state, True, lambda v: "0") == {"linear_in.weight": "A", "linear_out.weight": "0"}
    assert lora.fill_adapters(state, False, lambda v: "0") == state, "a LoRA checkpoint brings its own adapters"


def test_sync_sends_merged_weights():
    layer = "module.module.decoder.layers.3.self_attention.linear_qkv."
    named = [(layer + "to_wrap.weight", "W"), (layer + "to_wrap.layer_norm_weight", "N"),
             (layer + "adapter.linear_in.weight", "A"), (layer + "adapter.linear_out.weight", "B"),
             ("module.module.embedding.word_embeddings.weight", "E")]
    out = list(lora_sync.merged_named(named, lambda w, a, b: "%s+%s%s" % (w, b, a)))
    assert out == [(layer + "weight", "W+BA"), (layer + "layer_norm_weight", "N"),
                   ("module.module.embedding.word_embeddings.weight", "E")]
