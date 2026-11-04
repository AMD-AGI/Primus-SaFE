#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import re
import types

import pytest

from safe_post_training.frameworks.slime.adapter import Adapter
from safe_post_training.frameworks.slime.plugins import export_peft, lora, lora_sync, merge_lora

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


def test_peft_export_names():
    args = types.SimpleNamespace(hidden_size=4096, num_attention_heads=32, kv_channels=None, num_query_groups=8)
    b_args = export_peft.split_args(args, 16)
    assert (b_args.hidden_size, b_args.kv_channels, args.hidden_size) == (16, 128, 4096), \
        "B splits like a weight of rank columns with the model's head size"
    layer = "module.module.decoder.layers.0.self_attention.linear_qkv"
    assert export_peft.adapter_layers([layer + ".adapter.linear_in.weight", layer + ".adapter.linear_out.weight"]) == \
        [(layer + ".weight", layer + ".adapter.linear_in.weight", layer + ".adapter.linear_out.weight")]
    with pytest.raises(ValueError, match="no B matrix"):
        export_peft.adapter_layers([layer + ".adapter.linear_in.weight"])
    module, a, b = export_peft.peft_keys("model.layers.0.self_attn.q_proj.weight")
    assert (module, a, b) == ("model.layers.0.self_attn.q_proj",
                              "base_model.model.model.layers.0.self_attn.q_proj.lora_A.weight",
                              "base_model.model.model.layers.0.self_attn.q_proj.lora_B.weight")
    cfg = export_peft.adapter_config(16, 32.0, 0.0, ["model.layers.0.self_attn.q_proj",
                                                     "model.layers.1.self_attn.q_proj", "model.layers.0.mlp.up_proj"],
                                     "/models/base")
    assert cfg["peft_type"] == "LORA" and cfg["r"] == 16 and cfg["lora_alpha"] == 32.0
    assert cfg["target_modules"] == ["q_proj", "up_proj"] and cfg["base_model_name_or_path"] == "/models/base"


def test_adapter_only_checkpoints_merge_on_the_hf_side(tmp_path):
    root = tmp_path / "slime"
    (root / "tools").mkdir(parents=True)
    (root / "train.py").write_text("")
    (tmp_path / "base").mkdir()
    src = tmp_path / "checkpoints" / "iter_0000008"
    src.mkdir(parents=True)
    (src / "trainable_only").write_text("/models/base\n")
    commands = []

    def run_command(cmd, env=None):
        commands.append(cmd)
        (tmp_path / "final_model.adapter-tmp").mkdir(exist_ok=True)
        return 0

    a = Adapter(env={"SLIME_DIR": str(root)})
    recipe = {"model": {"path": str(tmp_path / "base")}, "lora": {"rank": 16, "alpha": 32}}
    a.export(recipe, str(tmp_path), run_command, iteration=8)
    cmd = commands[0]
    assert cmd[1].endswith("plugins/export_peft.py")
    assert cmd[cmd.index("--merge-into") + 1] == str(tmp_path / "final_model")
    assert not (tmp_path / "final_model.adapter-tmp").exists(), "the scratch adapter is removed"

    a.export_adapter(recipe, str(tmp_path), run_command, 8, str(tmp_path / "final_adapter"), str(tmp_path / "m"))
    cmd = commands[1]
    assert cmd[cmd.index("--output-dir") + 1] == str(tmp_path / "final_adapter")
    assert cmd[cmd.index("--verify-merged") + 1] == str(tmp_path / "m")


def test_lora_targets_on_a_moe_model():
    def hits(patterns, full_name):
        """Bridge's ModuleMatcher: the module's own name equals a pattern, or its full name matches the wildcard."""
        return any(p == full_name.rsplit(".", 1)[-1] or re.match("^" + p.replace("*", "(.*)") + "$", full_name)
                   for p in patterns)

    names = {"decoder.layers.0.mlp.linear_fc1", "decoder.layers.5.mlp.shared_experts.linear_fc1",
             "decoder.layers.5.mlp.experts.linear_fc1", "decoder.layers.5.mlp.experts.linear_fc2",
             "decoder.layers.5.self_attention.linear_proj", "decoder.layers.5.mlp.router"}
    assert lora.target_patterns({"targets": ["linear_qkv", "linear_fc1"]}, moe=False) == ["linear_qkv", "linear_fc1"]
    experts = lora.target_patterns({"targets": ["linear_proj", "linear_fc1"], "experts": True,
                                    "expertTargets": ["linear_fc1", "linear_fc2"]}, moe=True)
    assert {n for n in names if hits(experts, n)} == names - {"decoder.layers.5.mlp.router"}
    plain = lora.target_patterns({"targets": ["linear_proj", "linear_fc1"]}, moe=True)
    assert {n for n in names if hits(plain, n)} == {"decoder.layers.0.mlp.linear_fc1",
                                                     "decoder.layers.5.mlp.shared_experts.linear_fc1",
                                                     "decoder.layers.5.self_attention.linear_proj"}, \
        "without experts the routed experts keep their weights"
