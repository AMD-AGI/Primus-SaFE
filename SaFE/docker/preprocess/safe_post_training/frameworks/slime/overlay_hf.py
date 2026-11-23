#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Overlay the HF tensors converted from a trainable-only checkpoint onto the base model's HF directory.

Shards without a trained tensor are hard-linked from the base (copied when the file system refuses the link), the
others are rewritten with the trained tensors in place, and the base index is kept, so every untrained tensor stays
bit-identical to the base. The result is written aside and moved into place.

    python overlay_hf.py --base <base HF dir> --trained <converted trainable tensors> --out <export dir>
"""

import argparse
import json
import os
import shutil
import sys

INDEX = "model.safetensors.index.json"


def weight_map(hf_dir):
    """Return the tensor-to-shard map of an HF directory, from its index or from its only shard."""
    index = os.path.join(hf_dir, INDEX)
    if os.path.exists(index):
        with open(index) as f:
            return json.load(f)["weight_map"]
    shards = sorted(n for n in os.listdir(hf_dir) if n.endswith(".safetensors"))
    if len(shards) != 1:
        raise ValueError("%s has no %s and %d shards" % (hf_dir, INDEX, len(shards)))
    from safetensors import safe_open

    with safe_open(os.path.join(hf_dir, shards[0]), framework="pt") as f:
        return {name: shards[0] for name in f.keys()}


def link_or_copy(src, dst):
    """Hard-link src to dst, or copy it when the file system does not allow the link."""
    try:
        os.link(src, dst)
    except OSError:
        shutil.copyfile(src, dst)


def overlay(base, trained, out):
    """Write the base model with the trained tensors in place into out and return how many tensors were replaced."""
    from safetensors import safe_open
    from safetensors.torch import save_file

    base_map, trained_map = weight_map(base), weight_map(trained)
    unknown = sorted(set(trained_map) - set(base_map))
    if unknown:
        raise ValueError("the checkpoint has %d tensors the base model lacks, e.g. %s" % (
            len(unknown), ", ".join(unknown[:3])))
    if not trained_map:
        raise ValueError("the checkpoint has no tensors")
    by_shard = {}
    for name in trained_map:
        by_shard.setdefault(base_map[name], []).append(name)

    tmp = out.rstrip("/") + ".partial"
    shutil.rmtree(tmp, ignore_errors=True)
    os.makedirs(tmp)
    for name in os.listdir(base):
        src = os.path.join(base, name)
        if os.path.isfile(src) and not name.endswith(".safetensors"):
            shutil.copyfile(src, os.path.join(tmp, name))
    for shard in sorted(set(base_map.values())):
        src, dst = os.path.join(base, shard), os.path.join(tmp, shard)
        names = by_shard.get(shard)
        if not names:
            link_or_copy(src, dst)
            continue
        with safe_open(src, framework="pt") as f:
            metadata = f.metadata()
            tensors = {k: f.get_tensor(k) for k in f.keys()}
        for name in names:
            with safe_open(os.path.join(trained, trained_map[name]), framework="pt") as f:
                value = f.get_tensor(name)
            if tuple(value.shape) != tuple(tensors[name].shape):
                raise ValueError("%s has shape %s in the checkpoint and %s in the base model" % (
                    name, tuple(value.shape), tuple(tensors[name].shape)))
            tensors[name] = value.to(tensors[name].dtype).contiguous()
        save_file(tensors, dst, metadata=metadata)
    shutil.rmtree(out, ignore_errors=True)
    os.replace(tmp, out)
    return len(trained_map)


def main(argv=None):
    """Parse the arguments, overlay, and print how many tensors were replaced."""
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    parser.add_argument("--base", required=True, help="the base model's HF directory")
    parser.add_argument("--trained", required=True, help="the HF tensors converted from the checkpoint")
    parser.add_argument("--out", required=True, help="the export directory")
    args = parser.parse_args(argv)
    try:
        replaced = overlay(args.base, args.trained, args.out)
    except ValueError as e:
        print("overlay failed: %s" % e, file=sys.stderr)
        return 1
    print("replaced %d tensors of %s in %s" % (replaced, args.base, args.out))
    return 0


if __name__ == "__main__":
    sys.exit(main())
