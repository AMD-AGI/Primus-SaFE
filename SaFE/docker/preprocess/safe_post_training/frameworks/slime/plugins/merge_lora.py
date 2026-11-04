#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Export a LoRA checkpoint of slime as a merged HF model.

The torch_dist checkpoint is read in one process as slime's tools/convert_torch_dist_to_hf.py reads a full one, with
global tensors. Every adapted weight W becomes W + (alpha / rank) * B @ A in Megatron's layout, where the fused qkv
and gate/up rows of B follow W's, and the adapter entries are dropped; the converter then splits and renames the
weights as for a full checkpoint.

python3 merge_lora.py --slime-dir DIR --input-dir ITER_DIR --output-dir OUT --origin-hf-dir BASE --rank R --alpha A
"""
import argparse
import importlib.util
import os
import sys

ADAPTER_IN = ".adapter.linear_in.weight"
ADAPTER_OUT = ".adapter.linear_out.weight"


def merge_plan(keys):
    """Return the (weight, A, B) keys of every adapter and the other adapter keys to drop."""
    keys = set(keys)
    pairs, drop = [], []
    for key in sorted(keys):
        if key.endswith(ADAPTER_IN):
            base = key[:-len(ADAPTER_IN)]
            weight, out = base + ".weight", base + ADAPTER_OUT
            if out not in keys or weight not in keys:
                raise ValueError("the adapter of %s has no %s" % (base, "B matrix" if out not in keys else "weight"))
            pairs.append((weight, key, out))
        elif ".adapter." in key and not key.endswith(ADAPTER_OUT):
            drop.append(key)
    return pairs, drop


def merge(state_dict, scale):
    """Add scale * B @ A to every adapted weight, in float32, remove the adapter entries and return how many
    weights were merged. Weights stacked over the layers carry the layer axis first, and so do their adapters; routed
    experts' weights then carry the expert axis, which an adapter shared by the experts may lack."""
    import torch

    pairs, drop = merge_plan(state_dict)
    for weight, a_key, b_key in pairs:
        a, b, w = state_dict.pop(a_key), state_dict.pop(b_key), state_dict[weight]
        if a.dim() == b.dim() == w.dim() - 1 and w.dim() == 4:
            a, b = a.unsqueeze(-3), b.unsqueeze(-3)
        if not (a.dim() == b.dim() == w.dim() and w.dim() in (2, 3, 4)):
            raise ValueError("%s: an adapter of %d axes does not fit a weight of %d" % (weight, a.dim(), w.dim()))
        delta = b.float() @ a.float()
        try:
            fits = torch.broadcast_shapes(delta.shape, w.shape) == w.shape
        except RuntimeError:
            fits = False
        if not fits:
            raise ValueError("%s: the adapter gives %s for a weight of %s" % (weight, tuple(delta.shape),
                                                                            tuple(w.shape)))
        state_dict[weight] = (w.float() + scale * delta).to(w.dtype)
    for key in drop:
        state_dict.pop(key)
    return len(pairs)


def load_converter(slime_dir):
    """Import slime's tools/convert_torch_dist_to_hf.py, which is not part of a package."""
    path = os.path.join(slime_dir, "tools", "convert_torch_dist_to_hf.py")
    spec = importlib.util.spec_from_file_location("convert_torch_dist_to_hf", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main(argv=None):
    """Merge the checkpoint's adapters and write the HF model with the base model's assets."""
    p = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    p.add_argument("--slime-dir", required=True)
    p.add_argument("--input-dir", required=True)
    p.add_argument("--output-dir", required=True)
    p.add_argument("--origin-hf-dir", required=True)
    p.add_argument("--rank", type=int, required=True)
    p.add_argument("--alpha", type=float, required=True)
    p.add_argument("--chunk-size", type=int, default=5 * 1024 ** 3)
    p.add_argument("--vocab-size", type=int, default=None)
    args = p.parse_args(argv)

    import torch
    from transformers import AutoConfig

    conv = load_converter(args.slime_dir)
    model_name = type(AutoConfig.from_pretrained(args.origin_hf_dir, trust_remote_code=True)).__name__.lower()
    megatron_args = torch.load(os.path.join(args.input_dir, "common.pt"), weights_only=False)["args"]
    state_dict = {}
    reader = conv.WrappedStorageReader(args.input_dir)
    conv.dist_cp.state_dict_loader._load_state_dict(state_dict, storage_reader=reader,
                                                    planner=conv.EmptyStateDictLoadPlanner(), no_dist=True)
    merged = merge(state_dict, args.alpha / args.rank)
    if merged == 0:
        print("no LoRA adapters in %s" % args.input_dir, file=sys.stderr)
        return 1
    print("merged the adapters of %d weights" % merged)
    conv.save_tensors(megatron_args, model_name, state_dict, args.output_dir, args.chunk_size, args.vocab_size)
    conv.copy_assets(args.origin_hf_dir, args.output_dir)
    return 0


if __name__ == "__main__":
    sys.exit(main())
