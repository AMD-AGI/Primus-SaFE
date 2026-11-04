#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import re
import sys
import types

import pytest

from safe_post_training.frameworks.slime.plugins import export_peft, lora, lora_sync, merge_lora

torch = pytest.importorskip("torch")
QKV = "decoder.layers.self_attention.linear_qkv."
LAYER = "module.module.decoder.layers.0.self_attention.linear_qkv"


def qwen_qkv(args, name, param):
    """Split linear_qkv like slime's convert_qwen2_to_hf: query groups of q heads, then k and v, per row."""
    head_dim = args.kv_channels if args.kv_channels is not None else args.hidden_size // args.num_attention_heads
    per_group = args.num_attention_heads // args.num_query_groups
    q, k, v = torch.split(param.view(args.num_query_groups, -1, head_dim, args.hidden_size), [per_group, 1, 1], dim=1)
    layer = re.search(r"layers\.(\d+)\.", name).group(1)
    return [("model.layers.%s.self_attn.%s_proj.weight" % (layer, n), t.reshape(-1, args.hidden_size))
            for n, t in zip("qkv", (q, k, v))]


def test_merge_adds_the_scaled_low_rank_update():
    g = torch.Generator().manual_seed(0)
    w, a, b = torch.randn(6, 4, generator=g).to(torch.bfloat16), torch.randn(2, 4, generator=g), torch.randn(6, 2)
    state = {QKV + "weight": w.clone(), QKV + "adapter.linear_in.weight": a, QKV + "adapter.linear_out.weight": b}
    assert merge_lora.merge(state, 2.0) == 1
    assert list(state) == [QKV + "weight"]
    assert state[QKV + "weight"].dtype == torch.bfloat16
    assert torch.equal(state[QKV + "weight"], (w.float() + 2.0 * b @ a).to(torch.bfloat16))

    ws, a_s, bs = torch.randn(3, 6, 4), torch.randn(3, 2, 4), torch.randn(3, 6, 2)
    state = {QKV + "weight": ws.clone(), QKV + "adapter.linear_in.weight": a_s, QKV + "adapter.linear_out.weight": bs}
    merge_lora.merge(state, 0.5)
    assert torch.allclose(state[QKV + "weight"], ws + 0.5 * bs @ a_s), "layers stacked on the first axis"

    state = {QKV + "weight": torch.randn(6, 4), QKV + "adapter.linear_in.weight": torch.randn(2, 5),
             QKV + "adapter.linear_out.weight": torch.randn(6, 2)}
    with pytest.raises(ValueError, match="the adapter gives"):
        merge_lora.merge(state, 1.0)


class Wrapper(torch.nn.Module):
    """An AdapterWrapper with Bridge's parameter layout: to_wrap, and adapter.linear_in / linear_out."""

    def __init__(self):
        super().__init__()
        self.to_wrap = torch.nn.Linear(4, 6, bias=False)
        self.adapter = torch.nn.Module()
        self.adapter.linear_in = torch.nn.Linear(4, 2, bias=False)
        self.adapter.linear_out = torch.nn.Linear(2, 6, bias=False)


def test_the_pre_hook_loads_a_base_and_a_lora_checkpoint_strictly():
    model = torch.nn.Module()
    model.qkv = Wrapper()
    model.qkv._register_load_state_dict_pre_hook(lora.remap_hook, with_module=True)
    with torch.no_grad():
        model.qkv.adapter.linear_out.weight.fill_(3.0)
    a = model.qkv.adapter.linear_in.weight.detach().clone()
    base = {"qkv.weight": torch.randn(6, 4)}
    lora._state["base_load"] = True
    try:
        model.load_state_dict(base, strict=True)
    finally:
        lora._state["base_load"] = False
    assert torch.equal(model.qkv.to_wrap.weight, base["qkv.weight"])
    assert torch.equal(model.qkv.adapter.linear_in.weight, a)
    assert not model.qkv.adapter.linear_out.weight.any(), "B starts at zero, so the model equals the base"

    saved = {"qkv.weight": torch.randn(6, 4), "qkv.adapter.linear_in.weight": torch.randn(2, 4),
             "qkv.adapter.linear_out.weight": torch.randn(6, 2)}
    model.load_state_dict(saved, strict=True)
    assert torch.equal(model.qkv.adapter.linear_out.weight, saved["qkv.adapter.linear_out.weight"])


def test_split_adapters_reproduce_the_merged_weights():
    args = types.SimpleNamespace(hidden_size=8, num_attention_heads=4, num_query_groups=2, kv_channels=None)
    conv = types.SimpleNamespace(convert_to_hf=lambda a, model_name, name, param: qwen_qkv(a, name, param))
    w, a, b = torch.randn(16, 8), torch.randn(3, 8), torch.randn(16, 3)
    layers = {LAYER + ".adapter.linear_in.weight": a, LAYER + ".adapter.linear_out.weight": b}
    tensors, modules = export_peft.adapter_tensors(conv, "qwen3", args, layers, 3)
    assert len(modules) == 3
    scale = 32 / 3
    merged = dict(qwen_qkv(args, LAYER + ".weight", w + scale * b @ a))
    plain = dict(qwen_qkv(args, LAYER + ".weight", w))
    for module in modules:
        _, a_key, b_key = export_peft.peft_keys(module + ".weight")
        assert tensors[b_key].shape[1] == 3, "q, k and v keep the rank"
        delta = scale * tensors[b_key] @ tensors[a_key]
        assert torch.allclose(merged[module + ".weight"], plain[module + ".weight"] + delta, atol=1e-5)


def test_merge_into_the_hf_base(tmp_path):
    import safetensors.torch

    base, out = tmp_path / "base", tmp_path / "out"
    base.mkdir()
    q, norm = torch.randn(4, 8), torch.ones(8)
    safetensors.torch.save_file({"model.layers.0.self_attn.q_proj.weight": q, "model.norm.weight": norm},
                                str(base / "model.safetensors"))
    (base / "config.json").write_text("{}")
    a, b = torch.randn(2, 8), torch.randn(4, 2)
    module = "model.layers.0.self_attn.q_proj"
    _, a_key, b_key = export_peft.peft_keys(module + ".weight")
    export_peft.merge_into(str(base), {a_key: a, b_key: b}, [module], 0.5, str(out))
    got = safetensors.torch.load_file(str(out / "model.safetensors"))
    assert torch.allclose(got[module + ".weight"], q + 0.5 * b @ a)
    assert torch.equal(got["model.norm.weight"], norm)
    assert (out / "config.json").exists()
    missing = "model.layers.1.self_attn.q_proj"
    _, a_key, b_key = export_peft.peft_keys(missing + ".weight")
    with pytest.raises(ValueError, match="1 of 1 adapted weights"):
        export_peft.merge_into(str(base), {a_key: a, b_key: b}, [missing], 0.5, str(tmp_path / "x"))


def test_sync_merge_keeps_the_tensor_parallel_attributes(monkeypatch):
    class Merge:
        """Bridge's LoRAMerge on one tensor-parallel rank."""

        def merge(self, base, linear_out, linear_in, alpha, dim, *, tp_group):
            assert base.dtype == torch.float32, "the merge runs in float32"
            return base + alpha / dim * (linear_out @ linear_in)

    lora_merge = types.ModuleType("megatron.bridge.peft.lora_merge")
    lora_merge.LoRAMerge = Merge
    core = types.ModuleType("megatron.core")
    core.mpu = types.SimpleNamespace(get_tensor_model_parallel_group=lambda: None)
    for name in ("megatron", "megatron.bridge", "megatron.bridge.peft"):
        monkeypatch.setitem(sys.modules, name, types.ModuleType(name))
    monkeypatch.setitem(sys.modules, "megatron.bridge.peft.lora_merge", lora_merge)
    monkeypatch.setitem(sys.modules, "megatron.core", core)

    w = torch.nn.Parameter(torch.randn(6, 4).to(torch.bfloat16), requires_grad=False)
    w.tensor_model_parallel, w.partition_dim, w.partition_stride = True, 0, 1
    a, b = torch.randn(2, 4), torch.randn(6, 2)
    out = lora_sync.bridge_merge(4, 2)(w, a, b)
    assert out.dtype == torch.bfloat16
    assert torch.equal(out, (w.float() + 2 * b @ a).to(torch.bfloat16))
    assert (out.tensor_model_parallel, out.partition_dim, out.partition_stride) == (True, 0, 1), \
        "slime's all-gather reads them"
