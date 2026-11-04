#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Export the LoRA adapters of a slime checkpoint as a Hugging Face PEFT adapter.

The adapters are read in one process as slime's tools/convert_torch_dist_to_hf.py reads a checkpoint, layer by
layer. The B matrix of a fused Megatron weight is split into the HF modules by the converter's own function, given B
as a weight with rank columns: q, k and v of linear_qkv and gate and up of linear_fc1 each get their rows of B and the
whole A, so every HF adapter keeps the rank and alpha. --merge-into adds (alpha / rank) * B @ A to the base model's HF
weights, the export of a checkpoint that holds the adapters only. --verify-merged loads the base with the adapter
through peft and fails unless its logits on a fixed prompt match the merged model's.

python3 export_peft.py --slime-dir DIR --input-dir ITER_DIR --origin-hf-dir BASE --rank R --alpha A --output-dir OUT
"""
import argparse
import copy
import importlib.util
import json
import os
import shutil
import sys

ADAPTER_IN = ".adapter.linear_in.weight"
ADAPTER_OUT = ".adapter.linear_out.weight"
PEFT_PREFIX = "base_model.model."
WEIGHTS = "adapter_model.safetensors"
CONFIG = "adapter_config.json"
PROMPT = "The capital of France is Paris. The capital of Japan is"
MAX_RELATIVE_DIFF = 0.02


def split_args(args, rank):
    """Return the converter arguments that split a B matrix of rank columns like a weight of the model."""
    out = copy.copy(args)
    if getattr(args, "kv_channels", None) is None:
        out.kv_channels = args.hidden_size // args.num_attention_heads
    out.hidden_size = rank
    return out


def adapter_layers(names):
    """Return the (weight, A, B) names of every adapter among per-layer names."""
    names = set(names)
    out = []
    for name in sorted(names):
        if name.endswith(ADAPTER_IN):
            base = name[:-len(ADAPTER_IN)]
            if base + ADAPTER_OUT not in names:
                raise ValueError("the adapter of %s has no B matrix" % base)
            out.append((base + ".weight", name, base + ADAPTER_OUT))
    return out


def peft_keys(hf_weight):
    """Return the HF module of a weight name and its lora_A and lora_B keys in a PEFT adapter."""
    if not hf_weight.endswith(".weight"):
        raise ValueError("%s is not a weight" % hf_weight)
    module = hf_weight[:-len(".weight")]
    return module, PEFT_PREFIX + module + ".lora_A.weight", PEFT_PREFIX + module + ".lora_B.weight"


def adapter_config(rank, alpha, dropout, modules, base):
    """Return the adapter_config.json of a LoRA adapter on the HF modules."""
    return {"peft_type": "LORA", "task_type": "CAUSAL_LM", "r": rank, "lora_alpha": alpha, "lora_dropout": dropout,
            "target_modules": sorted({m.rsplit(".", 1)[-1] for m in modules}), "bias": "none",
            "fan_in_fan_out": False, "inference_mode": True, "base_model_name_or_path": base}


def load_converter(slime_dir):
    """Import slime's tools/convert_torch_dist_to_hf.py, which is not part of a package."""
    path = os.path.join(slime_dir, "tools", "convert_torch_dist_to_hf.py")
    spec = importlib.util.spec_from_file_location("convert_torch_dist_to_hf", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def load_adapters(conv, input_dir):
    """Read only the adapter tensors of a torch_dist checkpoint and return them per layer, with the checkpoint's
    Megatron arguments."""
    import torch
    from torch.distributed.checkpoint.default_planner import DefaultLoadPlanner
    from torch.distributed.checkpoint.metadata import TensorStorageMetadata

    class AdapterPlanner(DefaultLoadPlanner):
        """Plan the read of the adapter tensors only."""

        def set_up_planner(self, state_dict, metadata=None, is_coordinator=False):
            """Fill state_dict with empty adapter tensors of the checkpoint's shapes."""
            for key, value in metadata.state_dict_metadata.items():
                if ".adapter." in key and isinstance(value, TensorStorageMetadata) and "_state" not in key:
                    state_dict[key] = torch.empty(value.size, dtype=value.properties.dtype)
            super().set_up_planner(state_dict, metadata, is_coordinator)

    megatron_args = torch.load(os.path.join(input_dir, "common.pt"), weights_only=False)["args"]
    state = {}
    conv.dist_cp.state_dict_loader._load_state_dict(state, storage_reader=conv.WrappedStorageReader(input_dir),
                                                    planner=AdapterPlanner(), no_dist=True)
    return dict(conv.get_named_params(megatron_args, state)), megatron_args


def adapter_tensors(conv, model_name, megatron_args, layers, rank):
    """Return the PEFT tensors of the per-layer adapters and the HF modules they adapt."""
    b_args = split_args(megatron_args, rank)
    tensors, modules = {}, []
    for weight, a_name, b_name in adapter_layers(layers):
        a = layers[a_name]
        if a.dim() != 2:
            raise ValueError("%s: expert adapters cannot be exported yet" % weight)
        for hf_weight, b in conv.convert_to_hf(b_args, model_name, weight, layers[b_name]):
            module, a_key, b_key = peft_keys(hf_weight)
            tensors[a_key], tensors[b_key] = a.clone().contiguous(), b.clone().contiguous()
            modules.append(module)
    if not tensors:
        raise ValueError("the checkpoint holds no LoRA adapters")
    return tensors, modules


def merge_into(base, tensors, modules, scale, out):
    """Write the base HF model with the weight W of every adapted module as W + scale * B @ A, one shard at a
    time."""
    import safetensors.torch
    from safetensors import safe_open

    deltas = {}
    for module in modules:
        _, a_key, b_key = peft_keys(module + ".weight")
        deltas[module + ".weight"] = (tensors[a_key], tensors[b_key])
    os.makedirs(out, exist_ok=True)
    merged = 0
    for name in sorted(os.listdir(base)):
        src = os.path.join(base, name)
        if not name.endswith(".safetensors"):
            if os.path.isfile(src):
                shutil.copy(src, os.path.join(out, name))
            continue
        shard = {}
        with safe_open(src, framework="pt", device="cpu") as f:
            for key in f.keys():
                w = f.get_tensor(key)
                if key in deltas:
                    a, b = deltas[key]
                    w = (w.float() + scale * (b.float() @ a.float())).to(w.dtype)
                    merged += 1
                shard[key] = w
        safetensors.torch.save_file(shard, os.path.join(out, name), metadata={"format": "pt"})
    if merged != len(deltas):
        raise ValueError("%d of %d adapted weights are not in %s" % (len(deltas) - merged, len(deltas), base))


def verify(base, adapter, merged):
    """Return the largest logit difference between the base model with the adapter and the merged model, relative
    to the largest logit."""
    import torch
    from peft import PeftModel
    from transformers import AutoModelForCausalLM, AutoTokenizer

    device = "cuda" if torch.cuda.is_available() else "cpu"
    ids = AutoTokenizer.from_pretrained(merged)(PROMPT, return_tensors="pt").input_ids.to(device)

    def logits(model):
        """Return the model's logits on the prompt and free the model."""
        with torch.no_grad():
            out = model.to(device).eval()(ids).logits.float().cpu()
        del model
        if device == "cuda":
            torch.cuda.empty_cache()
        return out

    want = logits(AutoModelForCausalLM.from_pretrained(merged, torch_dtype=torch.bfloat16))
    got = logits(PeftModel.from_pretrained(AutoModelForCausalLM.from_pretrained(base, torch_dtype=torch.bfloat16),
                                           adapter))
    return ((want - got).abs().max() / want.abs().max().clamp(min=1e-6)).item()


def main(argv=None):
    """Write the PEFT adapter of a checkpoint and, as asked, the merged model and the peft check."""
    p = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    p.add_argument("--slime-dir", required=True)
    p.add_argument("--input-dir", required=True)
    p.add_argument("--origin-hf-dir", required=True)
    p.add_argument("--rank", type=int, required=True)
    p.add_argument("--alpha", type=float, required=True)
    p.add_argument("--dropout", type=float, default=0.0)
    p.add_argument("--output-dir", required=True)
    p.add_argument("--merge-into", default=None)
    p.add_argument("--verify-merged", default=None)
    args = p.parse_args(argv)

    import safetensors.torch
    from transformers import AutoConfig

    conv = load_converter(args.slime_dir)
    model_name = type(AutoConfig.from_pretrained(args.origin_hf_dir, trust_remote_code=True)).__name__.lower()
    layers, megatron_args = load_adapters(conv, args.input_dir)
    tensors, modules = adapter_tensors(conv, model_name, megatron_args, layers, args.rank)
    os.makedirs(args.output_dir, exist_ok=True)
    safetensors.torch.save_file(tensors, os.path.join(args.output_dir, WEIGHTS), metadata={"format": "pt"})
    with open(os.path.join(args.output_dir, CONFIG), "w") as f:
        json.dump(adapter_config(args.rank, args.alpha, args.dropout, modules, args.origin_hf_dir), f, indent=2)
    print("wrote the adapters of %d HF modules to %s" % (len(modules), args.output_dir))
    if args.merge_into:
        merge_into(args.origin_hf_dir, tensors, modules, args.alpha / args.rank, args.merge_into)
    if args.verify_merged:
        diff = verify(args.origin_hf_dir, args.output_dir, args.verify_merged)
        print("peft logits differ from the merged model's by %.4f of the largest logit" % diff)
        if diff > MAX_RELATIVE_DIFF:
            print("the adapter does not reproduce the merged model", file=sys.stderr)
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
