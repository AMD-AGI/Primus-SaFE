#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import io
import json
import math

from safe_post_training import metrics

ACTOR = "\x1b[36m(MegatronTrainRayActor pid=2414, ip=10.245.153.145)\x1b[0m "


def lines(out):
    return [json.loads(l.split(" ", 1)[1]) for l in out.getvalue().splitlines() if l.startswith("[")]


def test_forward_escapes_lines_that_look_like_markers():
    assert metrics.forward_text('[SUCCESS] {"run":"pt-x"}\n') == '  [SUCCESS] {"run":"pt-x"}'
    assert metrics.forward_text(ACTOR + '[ERROR] {"run":"pt-x"}').startswith("  \x1b[36m(")
    assert metrics.forward_text("  [METRIC] {}").startswith("    ")
    assert metrics.forward_text("step 3: {'train/loss': 1.0}") == "step 3: {'train/loss': 1.0}"


def test_forward_truncates_long_lines():
    out = metrics.forward_text("x" * 200_000)
    assert out.endswith("[%d bytes omitted]" % (200_000 - metrics.MAX_FORWARD_BYTES))
    assert len(out.encode()) < metrics.MAX_FORWARD_BYTES + 64


def test_strip_prefixes():
    assert metrics.strip_prefixes(ACTOR + "[2026-10-26 06:25:06] model.py:955 - step 0: {}") \
        == "[2026-10-26 06:25:06] model.py:955 - step 0: {}"


def test_match_name():
    assert metrics.match_name("eval/*", "eval/aime")
    assert not metrics.match_name("eval/*", "eval/dev/loss")
    assert metrics.match_name("eval/*/loss", "eval/dev/loss")
    assert metrics.match_name("train/lr-pg_*", "train/lr-pg_2")


def test_metric_markers(tmp_path):
    out, now = io.StringIO(), [100.0]
    em = metrics.Emitter("pt-x", "a1", str(tmp_path / "m" / "metrics.jsonl"), include=["train/*"], out=out,
                         clock=lambda: now[0])
    assert em.metric("step", "train", 0, {"perf/x": 1.0}) is None, "names outside include are dropped"
    em.metric("step", "train", 0, {"train/loss": 2.0, "perf/x": 1.0, "train/grad_norm": math.nan})
    em.metric("system", "time", 30, {"system/gpu_util_mean": 81.0})
    first, second = lines(out)
    assert first == {"run": "pt-x", "attempt": "a1", "pv": 1, "seq": 1, "kind": "step", "axis": "train", "step": 0,
                     "t": 100.0, "v": {"train/loss": 2.0, "train/grad_norm": "NaN"}}
    assert second["seq"] == 2 and second["v"] == {"system/gpu_util_mean": 81.0}
    recorded = [json.loads(l) for l in (tmp_path / "m" / "metrics.jsonl").read_text().splitlines()]
    assert [r["seq"] for r in recorded] == [1, 2]
    assert em.last_metrics() == {"system/gpu_util_mean": [30, 81.0], "train/grad_norm": [0, "NaN"],
                                 "train/loss": [0, 2.0]}


def test_replay_resends_recent_metrics_and_a_heartbeat():
    out, now = io.StringIO(), [0.0]
    em = metrics.Emitter("pt-x", "a1", include=["train/*"], out=out, clock=lambda: now[0])
    em.stage("train", state="running")
    em.metric("step", "train", 1, {"train/loss": 1.0})
    now[0] = 100.0
    em.metric("step", "train", 2, {"train/loss": 0.9})
    now[0] = 190.0
    out.truncate(0), out.seek(0)
    em.replay()
    replayed = lines(out)
    assert [r.get("seq") for r in replayed[:-1]] == [2], "only the last two minutes, with the original seq"
    assert replayed[-1]["stage"] == "train" and replayed[-1]["state"] == "heartbeat"


def test_wait_after_last_metric():
    now, slept = [10.0], []
    em = metrics.Emitter("pt-x", "a1", include=["train/*"], out=io.StringIO(), clock=lambda: now[0])
    em.wait_after_last_metric(sleep=slept.append)
    assert slept == []
    em.metric("step", "train", 1, {"train/loss": 1.0})
    now[0] = 25.0
    em.wait_after_last_metric(sleep=slept.append)
    assert slept == [45.0]
