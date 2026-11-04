#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import io
import json

from safe_post_training import backfill


def write_metrics(run_dir, records):
    """Write metrics.jsonl the way the launcher does."""
    (run_dir / "metrics").mkdir(parents=True)
    with open(run_dir / "metrics" / "metrics.jsonl", "w") as f:
        for r in records:
            f.write(json.dumps(r) + "\n")


def markers(out):
    """Return (marker, body) of every printed line."""
    return [(line.split(" ", 1)[0], json.loads(line.split(" ", 1)[1])) for line in out.getvalue().splitlines()]


def test_prints_only_the_missing_points_then_the_usage(tmp_path):
    records = [{"seq": s, "kind": "step", "axis": "train", "step": s, "t": 100.0 + s, "v": {"train/loss": 1.0 / s},
                "run": "pt-x", "attempt": a} for a in ("a1", "a2") for s in range(1, 6)]
    write_metrics(tmp_path, records)
    (tmp_path / "checkpoints").mkdir()
    (tmp_path / "checkpoints" / "model.pt").write_bytes(b"x" * 64)
    out = io.StringIO()
    spec = {"run": "pt-x", "dir": str(tmp_path), "attempt": "a2", "stage": "train",
            "ranges": {"a1": [[2, 3]], "a2": [[5, 5]]}}
    assert backfill.run(spec, out=out) == 3
    ms = markers(out)
    assert [(b["attempt"], b["seq"]) for m, b in ms if m == "[METRIC]"] == [("a1", 2), ("a1", 3), ("a2", 5)]
    assert ms[0][1] == {"run": "pt-x", "attempt": "a1", "pv": 1, "seq": 2, "kind": "step", "axis": "train", "step": 2,
                        "t": 102.0, "v": {"train/loss": 0.5}}
    assert ms[-1][0] == "[STAGE]" and ms[-1][1]["state"] == "disk" and ms[-1][1]["attempt"] == "a2"
    assert ms[-1][1]["detail"]["checkpoints"] == 64


def test_output_is_paced(tmp_path, monkeypatch):
    monkeypatch.setattr(backfill, "RATE_BYTES", 200)
    write_metrics(tmp_path, [{"seq": s, "kind": "step", "axis": "train", "step": s, "t": 1.0, "v": {"x": s},
                              "attempt": "a1"} for s in range(1, 11)])
    waits = []
    out = io.StringIO()
    backfill.run({"run": "pt-x", "dir": str(tmp_path), "attempt": "a1", "stage": "done", "ranges": {"a1": [[1, 10]]}},
                 out=out, sleep=waits.append, clock=lambda: 0.0)
    assert len(waits) >= 4 and all(w == backfill.RATE_SECONDS for w in waits)


def test_usage_only_without_gaps(tmp_path):
    out = io.StringIO()
    assert backfill.run({"run": "pt-x", "dir": str(tmp_path), "attempt": "a1", "stage": "train", "ranges": {}},
                        out=out) == 0
    assert [m for m, _ in markers(out)] == ["[STAGE]"]
