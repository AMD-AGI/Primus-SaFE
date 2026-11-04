#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Weight sync of RL with LoRA over slime's separate rollout.

SGLang serves the base model's parameter names, so every sync sends merged weights: W + (alpha / rank) * B @ A under
the base name, with W's tensor-parallel attributes, and no adapter parameters. Only the iterator slime's separate
rollout sync uses is replaced; it still applies its PP and EP renaming, all-gather, HF conversion and sending.
"""

ADAPTER_IN = ".adapter.linear_in.weight"
ADAPTER_OUT = ".adapter.linear_out.weight"
WRAPPED = ".to_wrap."
TP_ATTRS = ("tensor_model_parallel", "partition_dim", "partition_stride", "partition_sections")


def adapter_pairs(named):
    """Map the name prefix of every LoRA wrapper to its [A, B] tensors."""
    pairs = {}
    for name, tensor in named:
        for suffix, slot in ((ADAPTER_IN, 0), (ADAPTER_OUT, 1)):
            if name.endswith(suffix):
                pairs.setdefault(name[:-len(suffix)], [None, None])[slot] = tensor
    return pairs


def merged_named(named, merge):
    """Yield (name, tensor) pairs with the adapters merged: a wrapped weight W as merge(W, A, B) under its base name,
    the wrapped module's other tensors under their unwrapped names, and the rest unchanged."""
    named = list(named)
    pairs = adapter_pairs(named)
    for name, tensor in named:
        if ".adapter." in name:
            continue
        if WRAPPED not in name:
            yield name, tensor
            continue
        prefix, rest = name.split(WRAPPED, 1)
        a, b = pairs.get(prefix, (None, None))
        if rest == "weight" and a is not None and b is not None:
            yield prefix + "." + rest, merge(tensor, a, b)
        else:
            yield prefix + "." + rest, tensor


def bridge_merge(alpha, rank):
    """Return the merge of one local weight shard with Bridge's LoRAMerge, which gathers the adapter matrix tensor
    parallelism splits; it computes in float32 and copies the weight's tensor-parallel attributes."""
    import torch
    from megatron.bridge.peft.lora_merge import LoRAMerge
    from megatron.core import mpu

    merger, group = LoRAMerge(), mpu.get_tensor_model_parallel_group()

    def merge(w, a, b):
        """Return W + (alpha / rank) * B @ A for the local shard of W."""
        with torch.no_grad():
            out = merger.merge(w.detach().float(), b.detach().float(), a.detach().float(), alpha, rank,
                               tp_group=group).to(w.dtype)
        for attr in TP_ATTRS:
            if hasattr(w, attr):
                setattr(out, attr, getattr(w, attr))
        return out

    return merge


def install(alpha, rank):
    """Make slime's separate-rollout weight sync send merged weights."""
    from slime.backends.megatron_utils.update_weight import update_weight_from_distributed as sync

    named = sync.named_params_and_buffers
    merge = bridge_merge(alpha, rank)

    def merged_params(args, model):
        """Yield slime's named parameters with the LoRA adapters merged."""
        return merged_named(named(args, model), merge)

    sync.named_params_and_buffers = merged_params
