#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Post Training launcher entry point: python3 /shared-data/safe_post_training/launch.py.

Reads the recipe from PT_RECIPE (base64 JSON), registers a new attempt, runs the stages and prints the result
marker last. Exits 0 only when every stage succeeded.
"""

import argparse
import base64
import collections
import errno
import importlib
import json
import math
import os
import re
import signal
import subprocess
import sys
import threading
import time

if __package__ in (None, ""):
    sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from safe_post_training import gpu_sampler, metrics, stages  # noqa: E402

LAUNCHER_VERSION = "1.0.0"
NORETRY_FILE = ".noretry"
NORETRY_WAIT = 90
STATUS_FILE = "status.json"
WATCH_INTERVAL = 10
KILL_GRACE = 30
SAVING_PATTERN = re.compile(r"saving checkpoint", re.IGNORECASE)


class Watchdog:
    """Progress watchdog of one child process. Progress is a new training step, rollout or evaluation, or any
    output before the first step and in stages without steps. Past the warning threshold it prints a stalled
    [STAGE]; past the stop threshold it ends the child's process group."""

    def __init__(self, emitter, config=None, clock=time.time, interval=WATCH_INTERVAL, grace=KILL_GRACE):
        cfg = config or {}
        self.emitter, self.clock, self.interval, self.grace = emitter, clock, interval, grace
        self.startup = cfg.get("startupMinutes") or 30
        self.convert = cfg.get("convertMinutes") or self.startup
        self.min_stall = cfg.get("minStallMinutes") or 30
        self.factor = cfg.get("stallFactor") or 3
        self.save = cfg.get("saveMinutes") or 20
        self._lock = threading.Lock()
        self._done = threading.Event()
        self._thread = None
        self.reset(None)

    def reset(self, stage):
        """Start watching stage from now."""
        self.stage, self.last, self.step, self.saving, self.warned, self.reason = \
            stage, self.clock(), None, False, False, None
        self.times = collections.deque(maxlen=11)

    def line(self, text):
        """Note one line of child output: the start of a checkpoint save, or progress before the first step."""
        with self._lock:
            if SAVING_PATTERN.search(text):
                self.saving, self.last = True, self.clock()
            elif self.step is None:
                self._progress()

    def progress(self, step):
        """Note a new training step, rollout or evaluation."""
        with self._lock:
            self.step, self.saving = step, False
            self.times.append(self.clock())
            self._progress()

    def saved(self):
        """Note that a checkpoint finished saving."""
        with self._lock:
            self.saving = False
            self._progress()

    def _progress(self):
        """Restart the idle time and report a recovery after a stall warning."""
        self.last = self.clock()
        if self.warned:
            self.warned = False
            self.emitter.notice("recovered", {"lastStep": self.step})

    def thresholds(self):
        """Return the warning and stop thresholds in minutes."""
        if self.saving:
            return self.save, 2 * self.save
        if self.step is None:
            stop = self.convert if self.stage == "convert" else self.startup
            return stop / 2.0, stop
        times = list(self.times)
        gaps = sorted(b - a for a, b in zip(times, times[1:]))
        median = gaps[len(gaps) // 2] / 60.0 if gaps else 0
        warn = max(self.min_stall, self.factor * median)
        return warn, 2 * warn

    def check(self):
        """Warn once past the warning threshold; past the stop threshold return the reason to stop."""
        with self._lock:
            idle = (self.clock() - self.last) / 60.0
            warn, stop = self.thresholds()
            last = "none" if self.step is None else self.step
            if idle >= stop:
                self.reason = "%d minutes without training progress (last step %s)" % (idle, last)
                return self.reason
            if idle >= warn and not self.warned:
                self.warned = True
                self.emitter.notice("stalled", {"since": round(self.last, 3), "lastStep": self.step,
                                                "minutes": int(idle), "stopMinutes": int(math.ceil(stop))})
        return None

    def watch(self, proc, stage, tick=None):
        """Watch proc from a background thread until unwatch; tick runs on every pass."""
        self.reset(stage)
        self._done.clear()

        def loop():
            while not self._done.wait(self.interval):
                if tick:
                    tick()
                if self.check():
                    stop_group(proc, self.grace)
                    return

        self._thread = threading.Thread(target=loop, name="watchdog", daemon=True)
        self._thread.start()

    def unwatch(self):
        """Stop watching."""
        self._done.set()
        if self._thread:
            self._thread.join()
            self._thread = None


def stop_group(proc, grace):
    """End a child's whole process group: SIGTERM first, SIGKILL after grace seconds."""
    try:
        os.killpg(proc.pid, signal.SIGTERM)
        proc.wait(timeout=grace)
    except ProcessLookupError:
        return
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def load_recipe(env=os.environ):
    """Decode the recipe from the PT_RECIPE environment variable."""
    raw = env.get("PT_RECIPE")
    if not raw:
        raise SystemExit("PT_RECIPE is not set")
    return json.loads(base64.b64decode(raw))


def next_attempt(run_dir):
    """Count this start in status.json and return its attempt id, a1 for the first start."""
    path = os.path.join(run_dir, STATUS_FILE)
    status = {}
    if os.path.exists(path):
        with open(path) as f:
            status = json.load(f)
    status["attempts"] = status.get("attempts", 0) + 1
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(status, f)
    os.replace(tmp, path)
    return "a%d" % status["attempts"]


def train_start(run_dir, clock=time.time):
    """Return when the run first entered the train stage, recording it in status.json on the first entry; System
    metrics count seconds from it, so every attempt continues the same time axis."""
    path = os.path.join(run_dir, STATUS_FILE)
    status = {}
    if os.path.exists(path):
        with open(path) as f:
            status = json.load(f)
    if "trainStart" not in status:
        status["trainStart"] = round(clock(), 3)
        tmp = path + ".tmp"
        with open(tmp, "w") as f:
            json.dump(status, f)
        os.replace(tmp, path)
    return status["trainStart"]


def load_adapter(framework):
    """Load the framework adapter of the recipe."""
    module = importlib.import_module("safe_post_training.frameworks.%s.adapter" % framework)
    return module.Adapter()


def fail(emitter, run_dir, err, sleep=time.sleep):
    """Report a failed stage: an error that would recur is written to .noretry first and given time for the
    controller to disable failover; then the [ERROR] marker is printed last."""
    reason = {"stage": err.stage, "reason": err.reason}
    if not err.retry:
        try:
            with open(os.path.join(run_dir, NORETRY_FILE), "w") as f:
                json.dump(reason, f)
        except OSError:
            pass
        emitter.stage(err.stage, state="failed", detail=reason, retry=False)
        sleep(NORETRY_WAIT)
    emitter.wait_after_last_metric(sleep=sleep)
    emitter.stop_heartbeat()
    emitter.error(err.stage, err.reason, retry=err.retry)
    return 1


def run(recipe, out=None, gpus=stages.ray_gpus, sleep=time.sleep, watchdog=None, sampler=gpu_sampler.GpuSampler):
    """Run a recipe end to end and return the exit code; sampler, when set, samples the GPUs during training."""
    run_dir = recipe["output"]["base"]
    for sub in ("metrics", "logs/nccl"):
        os.makedirs(os.path.join(run_dir, sub), exist_ok=True)
    attempt = next_attempt(run_dir)
    emitter = metrics.Emitter(recipe["runId"], attempt, os.path.join(run_dir, "metrics", "metrics.jsonl"),
                              include=(recipe.get("metrics") or {}).get("include"), out=out)
    emitter.stage("start", detail={"launcher": LAUNCHER_VERSION})
    noretry = os.path.join(run_dir, NORETRY_FILE)
    if os.path.exists(noretry):
        with open(noretry) as f:
            reason = json.load(f)
        emitter.error(reason.get("stage", ""), reason.get("reason", ""), retry=False)
        return 1
    emitter.start_heartbeat()
    try:
        start_sampler = None
        if sampler:
            def start_sampler():
                return sampler(emitter, run_dir, recipe.get("layout"), train_start(run_dir))
        runner = stages.Runner(recipe, run_dir, load_adapter(recipe["framework"]), emitter, gpus=gpus,
                               watchdog=watchdog or Watchdog(emitter, recipe.get("watchdog")), sampler=start_sampler)
        result = runner.run()
    except stages.StageError as err:
        return fail(emitter, run_dir, err, sleep=sleep)
    except Exception as err:  # noqa: BLE001 - any crash must still end with an [ERROR] marker
        stage = emitter.stage_name or "start"
        if isinstance(err, OSError) and err.errno == errno.ENOSPC:
            err = stages.StageError(stage, stages.space_reason(run_dir), retry=False)
        else:
            err = stages.StageError(stage, "launcher error: %s" % err)
        return fail(emitter, run_dir, err, sleep=sleep)
    emitter.wait_after_last_metric(sleep=sleep)
    emitter.stop_heartbeat()
    emitter.success(**result)
    return 0


def main(argv=None):
    """Parse the command line and run the recipe, or print its plan with --dry-run."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dry-run", action="store_true", help="print the stages and commands without running")
    args = parser.parse_args(argv)
    recipe = load_recipe()
    if args.dry_run:
        adapter = load_adapter(recipe["framework"])
        cmd, env, files = adapter.train_command(recipe, recipe["output"]["base"])
        print(json.dumps({"stages": list(stages.STAGES), "train": cmd, "env": env, "files": sorted(files or {})},
                         indent=2))
        return 0
    return run(recipe)


if __name__ == "__main__":
    sys.exit(main())
