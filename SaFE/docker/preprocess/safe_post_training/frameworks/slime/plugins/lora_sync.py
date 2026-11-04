#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Weight sync of RL with LoRA over slime's separate rollout.

SGLang serves the base model's parameter names, so every sync sends merged weights: W + (alpha / rank) * B @ A under
the base name, with W's tensor-parallel attributes, and no adapter parameters. Only the iterator slime's separate
rollout sync uses is replaced; it still applies its PP and EP renaming, all-gather, HF conversion and sending.

A routed expert's weight is weight<global expert index> of its grouped linear. A shared adapter merges into every
expert of the layer; an adapter per expert stacks the local experts on its first axis.
"""
import re

ADAPTER_IN = ".adapter.linear_in.weight"
ADAPTER_OUT = ".adapter.linear_out.weight"
WRAPPED = ".to_wrap."
TP_ATTRS = ("tensor_model_parallel", "partition_dim", "partition_stride", "partition_sections")
EXPERT_WEIGHT = re.compile(r"^weight(\d+)$")


def adapter_pairs(named):
    """Map the name prefix of every LoRA wrapper to its [A, B] tensors."""
    pairs = {}
    for name, tensor in named:
        for suffix, slot in ((ADAPTER_IN, 0), (ADAPTER_OUT, 1)):
            if name.endswith(suffix):
                pairs.setdefault(name[:-len(suffix)], [None, None])[slot] = tensor
    return pairs


def merged_named(named, merge, merge_expert=None, expert_offset=0):
    """Yield (name, tensor) pairs with the adapters merged: a wrapped weight W as merge(W, A, B) under its base name,
    a routed expert's weight as merge_expert with its adapter, the wrapped module's other tensors under their unwrapped
    names, and the rest unchanged. expert_offset is the global index of this rank's first local expert."""
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
        expert = EXPERT_WEIGHT.match(rest)
        if a is None or b is None:
            yield prefix + "." + rest, tensor
        elif rest == "weight":
            yield prefix + "." + rest, merge(tensor, a, b)
        elif expert and merge_expert is not None:
            if a.dim() == 3:
                local = int(expert.group(1)) - expert_offset
                if not 0 <= local < a.shape[0]:
                    raise ValueError("%s: expert %s is not one of this rank's %d experts" % (
                        prefix, expert.group(1), a.shape[0]))
                a, b = a[local], b[local]
            yield prefix + "." + rest, merge_expert(tensor, a, b)
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


def local_merge(alpha, rank):
    """Return the merge of a routed expert's weight with its adapter, in float32 on this rank: runs keep expert tensor
    parallelism at 1, so neither is split; it copies the weight's tensor-parallel attributes."""
    import torch

    scale = alpha / rank

    def merge(w, a, b):
        """Return W + (alpha / rank) * B @ A."""
        with torch.no_grad():
            out = (w.detach().float() + scale * (b.detach().float() @ a.detach().float())).to(w.dtype)
        for attr in TP_ATTRS:
            if hasattr(w, attr):
                setattr(out, attr, getattr(w, attr))
        return out

    return merge


def install(alpha, rank):
    """Make slime's separate-rollout weight sync send merged weights."""
    from slime.backends.megatron_utils.update_weight import update_weight_from_distributed as sync

    named = sync.named_params_and_buffers
    merge, merge_expert = bridge_merge(alpha, rank), local_merge(alpha, rank)

    def merged_params(args, model):
        """Yield slime's named parameters with the LoRA adapters merged."""
        offset = 0
        if getattr(args, "num_experts", None):
            from megatron.core import mpu

            offset = (mpu.get_expert_model_parallel_rank() * args.num_experts //
                      mpu.get_expert_model_parallel_world_size())
        return merged_named(named(args, model), merge, merge_expert, offset)

    sync.named_params_and_buffers = merged_params
