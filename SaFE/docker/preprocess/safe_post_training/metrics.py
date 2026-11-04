#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Marker protocol between the launcher and the Post Training controller.

A marker is one line: the marker, a space and single-line JSON that carries run, attempt and pv; [METRIC] also
carries seq. Child output that starts like a marker is forwarded with two leading spaces, so the controller,
which matches markers at the start of the line, ignores it. Forwarded lines over 32 KB are truncated.
"""

import collections
import json
import math
import os
import re
import sys
import threading
import time

PROTOCOL_VERSION = 1
MARKERS = ("[STAGE]", "[METRIC]", "[SAMPLE]", "[ERROR]", "[SUCCESS]")
MAX_FORWARD_BYTES = 32 * 1024
REPLAY_INTERVAL = 60
REPLAY_WINDOW = 120
LAST_METRIC_GRACE = 60

_ANSI = re.compile(r"\x1b\[[0-9;?]*[A-Za-z]")
_RAY_PREFIX = re.compile(r"^\([^()\s]+ pid=\d+(?:, ip=[0-9A-Fa-f.:]+)?\)\s?")


def strip_prefixes(line):
    """Remove ANSI color codes and a leading Ray forwarding prefix such as "(Actor pid=12) "."""
    return _RAY_PREFIX.sub("", _ANSI.sub("", line), count=1)


def looks_like_marker(line):
    """Tell whether a child line would read as a marker once its prefixes are removed."""
    return strip_prefixes(line).lstrip().startswith(MARKERS)


def forward_text(line):
    """Return a child output line as the launcher forwards it: escaped when it looks like a marker and truncated
    past 32 KB."""
    line = line.rstrip("\n")
    if looks_like_marker(line):
        line = "  " + line
    data = line.encode("utf-8", "replace")
    if len(data) > MAX_FORWARD_BYTES:
        omitted = len(data) - MAX_FORWARD_BYTES
        line = data[:MAX_FORWARD_BYTES].decode("utf-8", "ignore") + " ... [%d bytes omitted]" % omitted
    return line


def _finite(value):
    """Encode a metric value for JSON: NaN and infinities become strings."""
    if isinstance(value, float):
        if math.isnan(value):
            return "NaN"
        if math.isinf(value):
            return "Inf" if value > 0 else "-Inf"
    return value


def match_name(pattern, name):
    """Match a metric name against a pattern in which * matches any run of characters other than "/"."""
    regex = "^" + "[^/]*".join(re.escape(part) for part in pattern.split("*")) + "$"
    return re.match(regex, name) is not None


class Emitter:
    """Prints markers for one launcher attempt, records metrics in metrics.jsonl and replays recent ones."""

    def __init__(self, run_id, attempt, metrics_path=None, include=None, out=None, clock=time.time):
        self.run_id = run_id
        self.attempt = attempt
        self.include = list(include or [])
        self.out = out or sys.stdout
        self.clock = clock
        self.metrics_path = metrics_path
        self.seq = 0
        self.stage_name = ""
        self.stage_detail = None
        self.parse_failures = 0
        # notes are added to the heartbeat detail, such as why GPU sampling stopped.
        self.notes = {}
        self.last_metric_time = None
        self.last_values = {}
        self._recent = collections.deque()
        self._lock = threading.RLock()
        self._stop = threading.Event()
        self._thread = None
        if metrics_path:
            os.makedirs(os.path.dirname(metrics_path), exist_ok=True)

    def _write(self, marker, body):
        """Print one marker line."""
        payload = {"run": self.run_id, "attempt": self.attempt, "pv": PROTOCOL_VERSION}
        payload.update(body)
        line = marker + " " + json.dumps(payload, separators=(",", ":"), allow_nan=False)
        with self._lock:
            self.out.write(line + "\n")
            self.out.flush()
        return line

    def forward(self, line):
        """Forward one line of child output."""
        with self._lock:
            self.out.write(forward_text(line) + "\n")
            self.out.flush()

    def stage(self, stage, state=None, detail=None, **extra):
        """Print a [STAGE] marker and remember it as the current stage for heartbeats."""
        body = {"stage": stage}
        if state:
            body["state"] = state
        if detail is not None:
            body["detail"] = detail
        body.update(extra)
        with self._lock:
            if state != "heartbeat":
                self.stage_name, self.stage_detail = stage, detail
            return self._write("[STAGE]", body)

    def notice(self, state, detail):
        """Print a [STAGE] marker about the current stage, such as a stall or the disk usage, without changing what
        the heartbeat repeats."""
        with self._lock:
            return self._write("[STAGE]", {"stage": self.stage_name, "state": state, "detail": detail})

    def included(self, name):
        """Tell whether a metric name is forwarded: system/* always, others when they match the recipe include."""
        if name.startswith("system/"):
            return True
        return any(match_name(p, name) for p in self.include)

    def metric(self, kind, axis, step, values, t=None):
        """Print a [METRIC] marker for the included values, append it to metrics.jsonl and keep it for replay.
        Returns the line, or None when nothing is included."""
        kept = {k: _finite(v) for k, v in values.items() if self.included(k)}
        if not kept or step is None or step < 0:
            return None
        with self._lock:
            self.seq += 1
            now = self.clock() if t is None else t
            body = {"seq": self.seq, "kind": kind, "axis": axis, "step": int(step), "t": round(now, 3), "v": kept}
            line = self._write("[METRIC]", body)
            self.last_metric_time = self.clock()
            for name, value in kept.items():
                self.last_values[name] = [int(step), value]
            self._recent.append((self.last_metric_time, line))
            if self.metrics_path:
                record = dict(body, run=self.run_id, attempt=self.attempt)
                with open(self.metrics_path, "a", encoding="utf-8") as f:
                    f.write(json.dumps(record, separators=(",", ":")) + "\n")
        return line

    def link(self, name, url):
        """Print a [METRIC] marker of kind link for a URL, such as the W&B run; it has no seq and is not a point."""
        return self._write("[METRIC]", {"kind": "link", "t": round(self.clock(), 3), "v": {name: url}})

    def last_metrics(self, limit=64):
        """Return the last point of each metric, at most limit of them, for the result marker."""
        with self._lock:
            names = sorted(self.last_values)[:limit]
            return {name: self.last_values[name] for name in names}

    def sample(self, **fields):
        """Print a [SAMPLE] marker."""
        return self._write("[SAMPLE]", fields)

    def error(self, stage, reason, **extra):
        """Print the final [ERROR] marker."""
        body = {"stage": stage, "reason": reason}
        body.update(extra)
        return self._write("[ERROR]", body)

    def success(self, **fields):
        """Print the final [SUCCESS] marker."""
        return self._write("[SUCCESS]", fields)

    def replay(self):
        """Print again the [METRIC] lines of the last two minutes, with their original seq, and a heartbeat of the
        current stage."""
        with self._lock:
            cutoff = self.clock() - REPLAY_WINDOW
            while self._recent and self._recent[0][0] < cutoff:
                self._recent.popleft()
            for _, line in list(self._recent):
                self.out.write(line + "\n")
            self.out.flush()
            if self.stage_name:
                detail = dict(self.notes, parseFailures=self.parse_failures)
                if isinstance(self.stage_detail, dict):
                    detail = dict(self.stage_detail, **detail)
                self.stage(self.stage_name, state="heartbeat", detail=detail)

    def start_heartbeat(self, interval=REPLAY_INTERVAL):
        """Replay recent metrics and repeat the current stage every interval seconds until stop_heartbeat."""

        def loop():
            while not self._stop.wait(interval):
                self.replay()

        self._thread = threading.Thread(target=loop, name="pt-heartbeat", daemon=True)
        self._thread.start()

    def stop_heartbeat(self):
        """Stop the heartbeat thread, so the result marker is the last marker printed."""
        self._stop.set()
        if self._thread:
            self._thread.join(timeout=5)

    def wait_after_last_metric(self, grace=LAST_METRIC_GRACE, sleep=time.sleep):
        """Wait until grace seconds have passed since the last [METRIC], so the controller reads it at least once
        more before the workload ends."""
        if self.last_metric_time is None:
            return
        remaining = self.last_metric_time + grace - self.clock()
        if remaining > 0:
            sleep(remaining)
