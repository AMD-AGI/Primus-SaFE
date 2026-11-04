#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""GPU utilization and memory sampling for the System metrics of a run.

During the train stage the launcher starts this module as a child process: a second driver of the job's Ray cluster
that places one reader actor without resources on every alive node and prints one JSON line of raw readings per
interval. A reader counts only the GPUs whose render node this container sees under /dev/dri, and reads them through
ROCm SMI (librocm_smi64.so, loaded at run time as wandb's system monitor does) or, without the library, amd-smi.

The launcher side, GpuSampler, reads those lines from a thread, aggregates them into one [METRIC] of kind system on
the time axis (split into train and rollout GPUs when they are separate), and appends the raw readings to
metrics/system.jsonl. Its failures never reach training: a child that dies is started once more, and after a second
failure sampling stops with a note in the heartbeat.
"""

import argparse
import ctypes
import glob
import json
import os
import re
import select
import socket
import subprocess
import sys
import threading
import time

INTERVAL_SECONDS = 30
TIMEOUT_SECONDS = 10
# STARTUP_SECONDS bounds connecting to Ray and starting the reader actors before the first line.
STARTUP_SECONDS = 60
RSMI_LIBS = ("librocm_smi64.so", "/opt/rocm/lib/librocm_smi64.so")
RSMI_MEM_TYPE_VRAM = 0
GIB = float(1 << 30)
SYSTEM_FILE = "system.jsonl"
_RENDER_NODE = re.compile(r"renderD(\d+)$")


def visible_render_minors(dri="/dev/dri"):
    """Return the render minors of the GPUs this container sees, empty when it sees none."""
    minors = set()
    for path in glob.glob(os.path.join(dri, "renderD*")):
        m = _RENDER_NODE.search(path)
        if m:
            minors.add(int(m.group(1)))
    return minors


class RocmSmi:
    """Reads the GPUs through librocm_smi64.so."""

    def __init__(self, libs=RSMI_LIBS):
        self.lib = None
        for name in libs:
            try:
                self.lib = ctypes.CDLL(name)
                break
            except OSError:
                continue
        if self.lib is None:
            raise OSError("librocm_smi64.so is not available")
        if self.lib.rsmi_init(ctypes.c_uint64(0)) != 0:
            raise OSError("rsmi_init failed")

    def read(self, minors):
        """Return the utilization and memory of the devices whose render minor is in minors."""
        count = ctypes.c_uint32()
        if self.lib.rsmi_num_monitor_devices(ctypes.byref(count)) != 0:
            return []
        out = []
        for index in range(count.value):
            minor, busy = ctypes.c_uint32(), ctypes.c_uint32()
            used, total = ctypes.c_uint64(), ctypes.c_uint64()
            if self.lib.rsmi_dev_drm_render_minor_get(index, ctypes.byref(minor)) != 0 or minor.value not in minors:
                continue
            if (self.lib.rsmi_dev_busy_percent_get(index, ctypes.byref(busy)) != 0
                    or self.lib.rsmi_dev_memory_usage_get(index, RSMI_MEM_TYPE_VRAM, ctypes.byref(used)) != 0
                    or self.lib.rsmi_dev_memory_total_get(index, RSMI_MEM_TYPE_VRAM, ctypes.byref(total)) != 0):
                continue
            out.append({"minor": minor.value, "util": busy.value, "memUsed": used.value, "memTotal": total.value})
        return out


class AmdSmi:
    """Reads the GPUs through the amd-smi command, for images without the ROCm SMI library."""

    def read(self, minors):
        """Return the utilization and memory amd-smi reports, when this container sees any GPU."""
        if not minors:
            return []
        out = subprocess.run(["amd-smi", "metric", "--usage", "--mem-usage", "--json"], capture_output=True,
                             text=True, timeout=TIMEOUT_SECONDS)
        if out.returncode != 0:
            raise OSError("amd-smi exited with code %d" % out.returncode)
        return parse_amd_smi(json.loads(out.stdout))


def parse_amd_smi(data):
    """Turn amd-smi metric JSON, a list or {"gpu_data": [...]}, into readings ordered by GPU index."""
    entries = data.get("gpu_data", []) if isinstance(data, dict) else data
    out = []
    for entry in entries or []:
        usage, mem = entry.get("usage") or {}, entry.get("mem_usage") or {}
        util, used, total = usage.get("gfx_activity"), mem.get("used_vram"), mem.get("total_vram")
        if not (util and used and total):
            continue
        out.append({"minor": int(entry.get("gpu", len(out))), "util": _number(util),
                    "memUsed": _bytes(used), "memTotal": _bytes(total)})
    return out


def _number(field):
    """Return the value of an amd-smi field, {"value": v, "unit": u} or a bare number."""
    value = field.get("value") if isinstance(field, dict) else field
    return float(value)


def _bytes(field):
    """Return an amd-smi memory field in bytes."""
    unit = (field.get("unit") or "MB").upper() if isinstance(field, dict) else "MB"
    scale = {"B": 1, "KB": 1 << 10, "MB": 1 << 20, "GB": 1 << 30}.get(unit, 1 << 20)
    return int(_number(field) * scale)


def open_source():
    """Return the ROCm SMI reader, or the amd-smi reader when the library can't be loaded."""
    try:
        return RocmSmi()
    except OSError:
        return AmdSmi()


class GpuReader:
    """The per-node reader actor: the node's IP and the readings of the GPUs this container sees."""

    def __init__(self):
        self.source = open_source()

    def read(self):
        """Return {"ip": ..., "gpus": [...]} for this node."""
        import ray

        return {"ip": ray.util.get_node_ip_address(), "gpus": self.source.read(visible_render_minors())}


def ip_key(ip):
    """Sort key of a node the way slime orders its placement bundles: by the numbers of its IP address."""
    try:
        return [int(p) for p in ip.split(".")]
    except ValueError:
        try:
            return [int(p) for p in socket.gethostbyname(ip).split(".")]
        except (OSError, ValueError):
            return [ord(c) for c in ip]


def aggregate(readings, layout=None):
    """Return the system metric values of one sample. With separate training and rollout GPUs, the first actorGpus
    GPUs in slime's order (node IP, then device) are training GPUs and the rest rollout GPUs, each with its own
    group."""
    gpus = []
    for node in readings:
        for gpu in sorted(node.get("gpus") or [], key=lambda g: g["minor"]):
            gpus.append((ip_key(node.get("ip", "")), gpu["minor"], gpu))
    gpus = [g for _, _, g in sorted(gpus, key=lambda x: (x[0], x[1]))]
    groups = {"": gpus}
    if layout and not layout.get("colocate", True):
        actor = int(layout.get("actorGpus", 0))
        groups = {"train/": gpus[:actor], "rollout/": gpus[actor:]}
    values = {}
    for prefix, group in groups.items():
        if not group:
            continue
        utils = [float(g["util"]) for g in group]
        used = [g["memUsed"] / GIB for g in group]
        name = "system/" + prefix
        values.update({
            name + "gpu_count": len(group),
            name + "gpu_util_mean": round(sum(utils) / len(utils), 2),
            name + "gpu_util_min": min(utils),
            name + "gpu_util_max": max(utils),
            name + "gpu_mem_used_mean_gb": round(sum(used) / len(used), 3),
            name + "gpu_mem_used_max_gb": round(max(used), 3),
            name + "gpu_mem_total_gb": round(max(g["memTotal"] for g in group) / GIB, 3),
        })
    return values


class GpuSampler:
    """Runs the sampling child during training and turns its lines into system metrics; it never fails training."""

    def __init__(self, emitter, run_dir, layout, train_start, interval=INTERVAL_SECONDS, command=None,
                 clock=time.time, startup=STARTUP_SECONDS, timeout=TIMEOUT_SECONDS):
        self.emitter, self.layout, self.train_start, self.clock = emitter, layout, train_start, clock
        self.interval, self.startup, self.timeout = interval, startup, timeout
        self.path = os.path.join(run_dir, "metrics", SYSTEM_FILE)
        self.command = command or [sys.executable, "-m", "safe_post_training.gpu_sampler",
                                   "--interval", str(interval), "--timeout", str(timeout)]
        self._stop = threading.Event()
        self._thread = None
        self._proc = None

    def start(self):
        """Start sampling in a background thread."""
        self._thread = threading.Thread(target=self._loop, name="pt-gpu-sampler", daemon=True)
        self._thread.start()

    def stop(self):
        """Stop sampling and the child."""
        self._stop.set()
        self._kill()
        if self._thread:
            self._thread.join(timeout=self.timeout)

    def _loop(self):
        """Run the child, once more after a failure; a second failure stops sampling with a heartbeat note."""
        for attempt in range(2):
            reason = self._run_child()
            if self._stop.is_set():
                return
            if attempt == 1:
                self.emitter.notes["gpuSampling"] = "stopped: %s" % reason

    def _run_child(self):
        """Read the child's lines until it fails or sampling stops; return why it ended."""
        env = dict(os.environ)
        env["PYTHONPATH"] = os.pathsep.join(filter(None, [os.path.dirname(os.path.dirname(os.path.abspath(
            __file__))), env.get("PYTHONPATH")]))
        try:
            self._proc = subprocess.Popen(self.command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, env=env,
                                          text=True, start_new_session=True)
        except OSError as e:
            return "cannot start the sampler: %s" % e
        deadline = self.startup
        try:
            while not self._stop.is_set():
                ready, _, _ = select.select([self._proc.stdout], [], [], deadline)
                if not ready:
                    return "no GPU reading for %d seconds" % deadline
                line = self._proc.stdout.readline()
                if not line:
                    return "the sampler exited with code %s" % self._proc.wait()
                self._report(line)
                deadline = self.interval + self.timeout
        finally:
            self._kill()
        return "stopped"

    def _report(self, line):
        """Report one line of readings: the aggregate as a [METRIC], the raw readings to system.jsonl."""
        try:
            sample = json.loads(line)
        except ValueError:
            return
        step = max(0, int(self.clock() - self.train_start))
        values = aggregate(sample.get("nodes") or [], self.layout)
        if values:
            self.emitter.metric("system", "time", step, values)
        try:
            with open(self.path, "a", encoding="utf-8") as f:
                f.write(json.dumps({"step": step, "t": sample.get("t"), "nodes": sample.get("nodes")},
                                   separators=(",", ":")) + "\n")
        except OSError:
            pass

    def _kill(self):
        """End the child, if it still runs."""
        proc, self._proc = self._proc, None
        if proc and proc.poll() is None:
            proc.kill()
            proc.wait()


def main(argv=None):
    """Sample the GPUs of every alive node of the job's Ray cluster and print one JSON line per interval."""
    parser = argparse.ArgumentParser(description=main.__doc__)
    parser.add_argument("--interval", type=float, default=INTERVAL_SECONDS)
    parser.add_argument("--timeout", type=float, default=TIMEOUT_SECONDS)
    args = parser.parse_args(argv)
    import ray
    from ray.util.scheduling_strategies import NodeAffinitySchedulingStrategy

    ray.init(address="auto", logging_level="ERROR", log_to_driver=False)
    reader = ray.remote(num_cpus=0)(GpuReader)
    actors = [reader.options(scheduling_strategy=NodeAffinitySchedulingStrategy(n["NodeID"], soft=False)).remote()
              for n in ray.nodes() if n.get("Alive")]
    while True:
        nodes = ray.get([a.read.remote() for a in actors], timeout=args.timeout)
        print(json.dumps({"t": round(time.time(), 3), "nodes": nodes}, separators=(",", ":")), flush=True)
        time.sleep(args.interval)


if __name__ == "__main__":
    main()
