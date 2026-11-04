#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import io
import json
import sys

from safe_post_training import gpu_sampler, launch, metrics
from safe_post_training.tests.test_launch import FakeAdapter, markers, recipe_for

GIB = 1 << 30


def gpu(minor, util, used_gib, total_gib=192):
    return {"minor": minor, "util": util, "memUsed": used_gib * GIB, "memTotal": total_gib * GIB}


def test_aggregate_all_gpus_and_by_role():
    readings = [{"ip": "10.0.0.10", "gpus": [gpu(129, 10, 20), gpu(128, 30, 40)]},
                {"ip": "10.0.0.9", "gpus": [gpu(128, 90, 100), gpu(129, 70, 80)]}]
    values = gpu_sampler.aggregate(readings, {"colocate": True})
    assert values["system/gpu_count"] == 4
    assert values["system/gpu_util_mean"] == 50 and values["system/gpu_util_min"] == 10
    assert values["system/gpu_util_max"] == 90
    assert values["system/gpu_mem_used_mean_gb"] == 60 and values["system/gpu_mem_used_max_gb"] == 100
    assert values["system/gpu_mem_total_gb"] == 192

    split = gpu_sampler.aggregate(readings, {"colocate": False, "actorGpus": 2})
    assert "system/gpu_count" not in split
    assert split["system/train/gpu_util_min"] == 70, "10.0.0.9 sorts before 10.0.0.10, as in slime"
    assert split["system/rollout/gpu_util_max"] == 30 and split["system/rollout/gpu_count"] == 2
    assert gpu_sampler.aggregate([{"ip": "10.0.0.9", "gpus": []}]) == {}


def test_visible_render_minors(tmp_path):
    for name in ("renderD128", "renderD130", "card0"):
        (tmp_path / name).touch()
    assert gpu_sampler.visible_render_minors(str(tmp_path)) == {128, 130}
    assert gpu_sampler.visible_render_minors(str(tmp_path / "missing")) == set()


class FakeRsmi:
    """Two devices, only the second of which this container sees."""

    def rsmi_num_monitor_devices(self, count):
        count._obj.value = 2
        return 0

    def rsmi_dev_drm_render_minor_get(self, index, minor):
        minor._obj.value = 128 + index
        return 0

    def rsmi_dev_busy_percent_get(self, index, busy):
        busy._obj.value = 40 + index
        return 0

    def rsmi_dev_memory_usage_get(self, index, kind, used):
        used._obj.value = 8 * GIB
        return 0

    def rsmi_dev_memory_total_get(self, index, kind, total):
        total._obj.value = 192 * GIB
        return 0


def test_rocm_smi_reads_visible_devices():
    smi = object.__new__(gpu_sampler.RocmSmi)
    smi.lib = FakeRsmi()
    assert smi.read({129}) == [{"minor": 129, "util": 41, "memUsed": 8 * GIB, "memTotal": 192 * GIB}]


def test_parse_amd_smi():
    data = {"gpu_data": [{"gpu": 0, "usage": {"gfx_activity": {"value": 55, "unit": "%"}},
                          "mem_usage": {"used_vram": {"value": 1024, "unit": "MB"},
                                        "total_vram": {"value": 196608, "unit": "MB"}}},
                         {"gpu": 1, "usage": {}}]}
    assert gpu_sampler.parse_amd_smi(data) == [{"minor": 0, "util": 55.0, "memUsed": GIB, "memTotal": 192 * GIB}]


def test_sampler_reports_and_gives_up_after_two_failures(tmp_path):
    (tmp_path / "metrics").mkdir()
    out = io.StringIO()
    emitter = metrics.Emitter("pt-x", "a1", str(tmp_path / "metrics" / "metrics.jsonl"), include=[], out=out)
    line = json.dumps({"t": 1.0, "nodes": [{"ip": "10.0.0.9", "gpus": [gpu(128, 60, 10)]}]})
    child = [sys.executable, "-c", "print(%r)\nprint('ray: not json')\nraise SystemExit(3)" % line]
    sampler = gpu_sampler.GpuSampler(emitter, str(tmp_path), {"colocate": True}, train_start=1000.0,
                                     command=child, clock=lambda: 1042.5, startup=10)
    sampler.start()
    sampler._thread.join(timeout=30)
    sampler.stop()
    points = [m[1] for m in markers(out) if m[0] == "[METRIC]"]
    assert len(points) == 2, "the child is started once more after it exits"
    assert points[0]["kind"] == "system" and points[0]["axis"] == "time" and points[0]["step"] == 42
    assert points[0]["v"]["system/gpu_util_mean"] == 60 and points[0]["v"]["system/gpu_count"] == 1
    assert emitter.notes["gpuSampling"] == "stopped: the sampler exited with code 3"
    raw = (tmp_path / "metrics" / "system.jsonl").read_text().splitlines()
    assert len(raw) == 2 and json.loads(raw[0])["nodes"][0]["ip"] == "10.0.0.9"


def test_train_start_is_kept_across_attempts(tmp_path):
    assert launch.train_start(str(tmp_path), clock=lambda: 100.0) == 100.0
    assert launch.train_start(str(tmp_path), clock=lambda: 900.0) == 100.0


class RecordingSampler:
    events = []

    def __init__(self, emitter, run_dir, layout, train_start):
        RecordingSampler.events.append(("new", layout["gpusPerNode"], train_start > 0))

    def start(self):
        RecordingSampler.events.append(("start",))

    def stop(self):
        RecordingSampler.events.append(("stop",))


def test_launcher_samples_during_training(monkeypatch, tmp_path):
    monkeypatch.setattr(launch, "load_adapter", lambda name: FakeAdapter())
    RecordingSampler.events = []
    code = launch.run(recipe_for(tmp_path), out=io.StringIO(), gpus=lambda: 8, sleep=lambda s: None,
                      sampler=RecordingSampler)
    assert code == 0
    assert RecordingSampler.events == [("new", 8, True), ("start",), ("stop",)]
