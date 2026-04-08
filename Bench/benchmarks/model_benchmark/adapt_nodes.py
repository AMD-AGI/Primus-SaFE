#!/usr/bin/env python3
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Given a Primus experiment YAML and the number of healthy nodes, calculate
# how many nodes can actually participate in model training.
#
# Megatron distributes world_size GPUs as:
#     world_size = TP * PP * DP    (EP folds into DP or is separate)
#
# Constraint: world_size (= nodes * gpus_per_node) must be divisible by
# TP * PP * EP.  We find the largest node count <= healthy_nodes that
# satisfies this, and also ensure global_batch_size is feasible.
#
# Output (stdout, one line): JSON with usable_nodes, adjusted_gbs, etc.

import argparse
import json
import math
import sys

import yaml


def _read_parallel_params(config_path: str) -> dict:
    """Extract TP, PP, EP, micro_batch_size, global_batch_size from config."""
    with open(config_path) as f:
        cfg = yaml.safe_load(f)

    overrides = {}
    modules = cfg.get("modules", {})
    for mod in modules.values():
        if isinstance(mod, dict) and "overrides" in mod:
            overrides = mod["overrides"]
            break

    return {
        "tp": int(overrides.get("tensor_model_parallel_size", 1)),
        "pp": int(overrides.get("pipeline_model_parallel_size", 1)),
        "ep": int(overrides.get("expert_model_parallel_size", 1)),
        "mbs": int(overrides.get("micro_batch_size", 1)),
        "gbs": int(overrides.get("global_batch_size", 128)),
    }


def compute_usable_nodes(
    healthy_nodes: int,
    gpus_per_node: int,
    tp: int,
    pp: int,
    ep: int,
) -> int:
    """Return the largest usable node count <= healthy_nodes.

    Constraint: usable_nodes * gpus_per_node must be divisible by (TP * PP * EP).
    """
    parallel_unit = tp * pp * ep
    if parallel_unit <= 0:
        return healthy_nodes

    for n in range(healthy_nodes, 0, -1):
        total_gpus = n * gpus_per_node
        if total_gpus % parallel_unit == 0:
            return n

    return 0


def adapt_gbs(
    original_gbs: int,
    mbs: int,
    usable_nodes: int,
    gpus_per_node: int,
    tp: int,
    pp: int,
    ep: int,
) -> int:
    """Ensure global_batch_size is divisible by mbs * DP.

    DP = total_gpus / (TP * PP * EP).
    GBS must be divisible by mbs * DP.
    If not, round down to nearest valid value; if that's 0, round up.
    """
    total_gpus = usable_nodes * gpus_per_node
    parallel_unit = tp * pp * ep
    dp = total_gpus // parallel_unit
    step = mbs * dp

    if step <= 0:
        return original_gbs

    if original_gbs % step == 0:
        return original_gbs

    adjusted = (original_gbs // step) * step
    if adjusted <= 0:
        adjusted = step
    return adjusted


def main():
    ap = argparse.ArgumentParser(description="Adapt node count for model benchmark")
    ap.add_argument("--config", required=True, help="Primus experiment YAML path")
    ap.add_argument("--healthy-nodes", type=int, required=True)
    ap.add_argument("--gpus-per-node", type=int, default=8)
    ap.add_argument("--json", action="store_true", help="Output full JSON")
    args = ap.parse_args()

    params = _read_parallel_params(args.config)
    tp, pp, ep = params["tp"], params["pp"], params["ep"]
    mbs, gbs = params["mbs"], params["gbs"]

    usable = compute_usable_nodes(args.healthy_nodes, args.gpus_per_node, tp, pp, ep)
    adjusted_gbs = adapt_gbs(gbs, mbs, usable, args.gpus_per_node, tp, pp, ep) if usable > 0 else 0
    total_gpus = usable * args.gpus_per_node
    dp = total_gpus // (tp * pp * ep) if usable > 0 else 0

    result = {
        "healthy_nodes": args.healthy_nodes,
        "usable_nodes": usable,
        "idle_nodes": args.healthy_nodes - usable,
        "gpus_per_node": args.gpus_per_node,
        "total_gpus": total_gpus,
        "tp": tp,
        "pp": pp,
        "ep": ep,
        "dp": dp,
        "micro_batch_size": mbs,
        "original_gbs": gbs,
        "adjusted_gbs": adjusted_gbs,
    }

    if args.json:
        print(json.dumps(result))
    else:
        print(usable)


if __name__ == "__main__":
    main()
