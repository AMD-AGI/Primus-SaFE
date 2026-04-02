#!/usr/bin/env python3
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Parse Primus/Megatron training logs and emit a benchmark results JSON.
# Recognises the standard Megatron log line format:
#   iteration  5/ 50 | ... | elapsed time per iteration (ms): 1234.56 |
#   throughput per GPU (TFLOP/s/GPU): 123.45 |
#   tokens per GPU (tokens/s/GPU): 4567 | global batch size: 128
#
# Also handles comma-formatted numbers (e.g. 1,234.56).

import argparse
import json
import os
import re
import sys
from statistics import mean

NUM = r"[\d,]+(?:\.\d+)?"

ITERATION_RE = re.compile(
    rf"iteration\s+(\d+)/\s*\d+.*?"
    rf"elapsed time per iteration \(ms\):\s*({NUM}).*?"
    rf"throughput per GPU \(TFLOP/s/GPU\):\s*({NUM}).*?"
    rf"tokens per GPU \(tokens/s/GPU\):\s*({NUM}).*?"
    rf"global batch size:\s*(\d+)",
    re.IGNORECASE,
)


def _float(s: str) -> float:
    return float(s.replace(",", ""))


def parse_log(path: str) -> list[dict]:
    records = []
    with open(path, errors="ignore") as f:
        for line in f:
            m = ITERATION_RE.search(line)
            if m:
                records.append(
                    {
                        "iter": int(m.group(1)),
                        "elapsed_ms": _float(m.group(2)),
                        "tflops_gpu": _float(m.group(3)),
                        "tokens_gpu": _float(m.group(4)),
                        "gbs": int(m.group(5)),
                    }
                )
    return records


def compute(records: list[dict], warmup: int) -> dict | None:
    if not records:
        return None

    records = sorted(records, key=lambda r: r["iter"])
    if len(records) <= warmup:
        return None

    records = records[warmup:]
    return {
        "iterations_used": len(records),
        "avg_elapsed_ms": round(mean(r["elapsed_ms"] for r in records), 2),
        "avg_tflops_gpu": round(mean(r["tflops_gpu"] for r in records), 2),
        "avg_tokens_gpu": round(mean(r["tokens_gpu"] for r in records), 2),
        "global_batch_size": records[0]["gbs"],
    }


def resolve_config_name(config_path: str) -> str:
    basename = os.path.basename(config_path)
    name, _ = os.path.splitext(basename)
    return name


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--log-file", required=True)
    ap.add_argument("--config", default="")
    ap.add_argument("--nodes", type=int, default=1)
    ap.add_argument("--gpus-per-node", type=int, default=8)
    ap.add_argument("--warmup-iters", type=int, default=2)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    records = parse_log(args.log_file)
    stats = compute(records, args.warmup_iters)

    result = {
        "benchmark": "model_training",
        "config": resolve_config_name(args.config) if args.config else "unknown",
        "config_path": args.config,
        "nodes": args.nodes,
        "gpus_per_node": args.gpus_per_node,
        "total_gpus": args.nodes * args.gpus_per_node,
        "warmup_iters_discarded": args.warmup_iters,
    }

    if stats:
        result["metrics"] = stats
        result["status"] = "success"
    else:
        result["status"] = "no_data"
        result["metrics"] = {}
        print(
            f"WARNING: Could not extract metrics from {args.log_file} "
            f"(found {len(records)} iteration records, need > {args.warmup_iters})",
            file=sys.stderr,
        )

    with open(args.output, "w") as f:
        json.dump(result, f, indent=2)

    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
