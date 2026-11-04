#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Re-print the [METRIC] markers missing from a finished run's log, and the run directory's usage.

Run as python3 /shared-data/safe_post_training/backfill.py in a read-only workload of the run's workspace. It
reads PT_BACKFILL, base64 JSON {"run", "dir", "attempt", "stage", "ranges": {attempt: [[from, to], ...]}}, and
prints the points of metrics/metrics.jsonl whose (attempt, seq) falls in the ranges, in the launcher's format.
The output is paced, so the job's own log is not rotated before the controller reads it. A disk [STAGE] of
the given attempt and stage comes last.
"""

import base64
import json
import os
import sys
import time

if __package__ in (None, ""):
    sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from safe_post_training import metrics, stages  # noqa: E402

RATE_BYTES = 1 << 20
RATE_SECONDS = 30


def in_ranges(ranges, seq):
    """Tell whether seq lies in one of the [from, to] ranges."""
    return any(lo <= seq <= hi for lo, hi in ranges or [])


def run(spec, out=sys.stdout, sleep=time.sleep, clock=time.monotonic):
    """Print the missing points and the usage; returns the number of points printed."""
    run_id, ranges = spec["run"], spec.get("ranges") or {}
    printed, budget, window = 0, RATE_BYTES, clock()
    path = os.path.join(spec["dir"], "metrics", "metrics.jsonl")
    lines = open(path, encoding="utf-8") if ranges and os.path.exists(path) else []
    for raw in lines:
        try:
            record = json.loads(raw)
        except ValueError:
            continue
        attempt, seq = record.get("attempt"), record.get("seq")
        if not isinstance(seq, int) or not in_ranges(ranges.get(attempt), seq):
            continue
        body = {"run": run_id, "attempt": attempt, "pv": metrics.PROTOCOL_VERSION}
        body.update({k: record[k] for k in ("seq", "kind", "axis", "step", "t", "v") if k in record})
        line = "[METRIC] " + json.dumps(body, separators=(",", ":")) + "\n"
        if len(line) > budget:
            sleep(max(0.0, RATE_SECONDS - (clock() - window)))
            budget, window = RATE_BYTES, clock()
        out.write(line)
        budget -= len(line)
        printed += 1
    if lines:
        lines.close()
    usage = {"run": run_id, "attempt": spec["attempt"], "pv": metrics.PROTOCOL_VERSION, "stage": spec["stage"],
             "state": "disk", "detail": stages.disk_usage(spec["dir"])}
    out.write("[STAGE] " + json.dumps(usage, separators=(",", ":")) + "\n")
    out.flush()
    return printed


def main(env=os.environ):
    """Decode PT_BACKFILL and run the backfill."""
    raw = env.get("PT_BACKFILL")
    if not raw:
        raise SystemExit("PT_BACKFILL is not set")
    run(json.loads(base64.b64decode(raw)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
