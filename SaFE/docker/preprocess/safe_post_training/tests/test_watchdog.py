#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import io
import json
import sys
import time

from safe_post_training import launch, metrics, stages


class Clock:
    """A clock the test moves by hand."""

    def __init__(self):
        self.now = 1000.0

    def __call__(self):
        return self.now

    def advance(self, minutes):
        self.now += minutes * 60


def stage_markers(out, state):
    """Return the detail of every [STAGE] marker with state printed to out."""
    found = []
    for line in out.getvalue().splitlines():
        if line.startswith("[STAGE] "):
            body = json.loads(line[len("[STAGE] "):])
            if body.get("state") == state:
                found.append(body.get("detail"))
    return found


def new_watchdog(**config):
    """Return a watchdog over a fresh emitter and the hand-moved clock."""
    out, clock = io.StringIO(), Clock()
    emitter = metrics.Emitter("pt-x", "a1", out=out)
    emitter.stage("train", state="running")
    dog = launch.Watchdog(emitter, config, clock=clock)
    dog.reset("train")
    return dog, out, clock


def test_warns_recovers_and_stops():
    dog, out, clock = new_watchdog(minStallMinutes=30, stallFactor=3)
    for step in range(11):
        dog.progress(step)
        clock.advance(20)
    assert dog.thresholds() == (60, 120), "3 x the 20 minute median step beats the 30 minute minimum"
    clock.advance(41)
    assert dog.check() is None
    assert stage_markers(out, "stalled")[0]["lastStep"] == 10
    assert dog.check() is None and len(stage_markers(out, "stalled")) == 1, "the warning is printed once"
    dog.progress(11)
    assert stage_markers(out, "recovered") == [{"lastStep": 11}]
    clock.advance(121)
    assert dog.check() == "121 minutes without training progress (last step 11)"


def test_startup_and_checkpoint_thresholds():
    dog, _, clock = new_watchdog(startupMinutes=30, saveMinutes=20)
    assert dog.thresholds() == (15, 30), "before the first step the startup minutes are the stop threshold"
    clock.advance(20)
    dog.line("loading weights\n")
    clock.advance(20)
    assert dog.check() is None, "output before the first step counts as progress"
    dog.progress(0)
    dog.line("[2026-11-03 10:00:00] saving checkpoint at iteration 100 to /run/checkpoints\n")
    assert dog.thresholds() == (20, 40)
    dog.saved()
    assert dog.thresholds() == (30, 60)


def test_stop_ends_the_process_group(tmp_path):
    out = io.StringIO()
    emitter = metrics.Emitter("pt-x", "a1", out=out)
    dog = launch.Watchdog(emitter, {"convertMinutes": 0.01}, interval=0.05, grace=1)
    runner = stages.Runner({}, str(tmp_path), None, emitter, watchdog=dog)
    child = "import subprocess, sys, time; subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)']); " \
            "print('started', flush=True); time.sleep(60)"
    began = time.time()
    code = runner.run_command([sys.executable, "-c", child], watch="convert")
    assert code != 0 and time.time() - began < 20
    assert dog.reason.startswith("0 minutes without training progress")


def test_disk_usage_and_space_reason(tmp_path):
    (tmp_path / "checkpoints" / "iter_0000010").mkdir(parents=True)
    (tmp_path / "checkpoints" / "iter_0000010" / "model.pt").write_bytes(b"x" * 1024)
    (tmp_path / "final_model").mkdir()
    (tmp_path / "final_model" / "model.safetensors").write_bytes(b"x" * 512)
    usage = stages.disk_usage(str(tmp_path))
    assert usage == {"checkpoints": 1024, "criticCheckpoints": 0, "finalModel": 512, "finalAdapter": 0, "total": 1536}
    assert stages.space_reason(str(tmp_path)).startswith("shared storage is out of space (")
