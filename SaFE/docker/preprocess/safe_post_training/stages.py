#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""The launcher's stages: prepare, convert, check, train and export.

Each stage leaves a completion mark, so a retried workload skips the stages that already finished. The framework
specific work (conversion, the training command, export and metric lines) comes from the recipe's adapter.
"""

import contextlib
import functools
import hashlib
import json
import math
import os
import re
import shutil
import signal
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid

from safe_post_training import dataset

STAGES = ("prepare", "convert", "check", "train", "export")
# bf16 weights plus fp32 master weights and two fp32 Adam moments, relative to the bf16 weights on disk.
CHECKPOINT_FACTOR = 7
DISK_DIRS = (("checkpoints", "checkpoints"), ("criticCheckpoints", "critic_checkpoints"),
             ("finalModel", "final_model"))
PROGRESS_KINDS = ("step", "rollout", "eval")
ENOSPC_TEXT = "No space left on device"
# With less than FULL_FREE_BYTES free the storage of a run directory counts as full.
FULL_FREE_BYTES = 1 << 30
CONVERT_LOCK = ".lock"
# FP8_DIR holds the FP8 HF weights next to a conversion, made when an FP8 rollout needs them.
FP8_DIR = "fp8_hf"
SOURCE_FILE = "source.json"
# The holder of a conversion lock refreshes it every LOCK_HEARTBEAT_SECONDS; a lock not refreshed for
# LOCK_STALE_SECONDS was left by a launcher that died, and the next run takes it over.
LOCK_HEARTBEAT_SECONDS = 60
LOCK_STALE_SECONDS = 10 * 60
# A lock without an owner record is one of a launcher from before the heartbeat, which holds it for a whole
# conversion without refreshing it.
LOCK_UNOWNED_STALE_SECONDS = 6 * 3600
LOCK_OWNER = "owner.json"
ITER_DIR = re.compile(r"^iter_(\d{7})$")
VAL_LOSS = re.compile(r"^eval/[^/]+/loss$")
WANDB_DEFAULT_URL = "https://api.wandb.ai"
WANDB_PROBE_SECONDS = 10
REWARD_PROBE_SECONDS = 10
REWARD_KEY_ENV = "PT_RM_API_KEY"
# REWARD_REJECTED_TEXT is what the remote_rm plugin prints when the reward service refuses the key.
REWARD_REJECTED_TEXT = "safe-post-training: the reward service rejected the key"
WANDB_RUN_URL = re.compile(r"View run\b.*?(https?://\S+)")
LOCK_POLL_SECONDS = 30
KILL_GRACE = 30


class NoRedirect(urllib.request.HTTPRedirectHandler):
    """Refuses redirects, so credentials go only to the address they were meant for."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        """Return no new request, which makes the redirect an HTTPError."""
        return None


def http_status(url, timeout, headers=None):
    """Return the status of a HEAD request to url within timeout seconds, or None when no HTTP server answers. A
    redirect is answered as its status, not followed."""
    opener = urllib.request.build_opener(NoRedirect)
    try:
        with opener.open(urllib.request.Request(url, method="HEAD", headers=headers or {}), timeout=timeout) as resp:
            return resp.status
    except urllib.error.HTTPError as e:
        return e.code
    except (urllib.error.URLError, OSError, ValueError):
        return None


def saved_iterations(path, strict=False):
    """Return the iterations of the iter_NNNNNNN checkpoint directories under path, in order. A path that cannot be
    listed has none, unless strict is set: then only a missing path has none, and any other failure is raised."""
    try:
        names = os.listdir(path)
    except FileNotFoundError:
        return []
    except OSError:
        if strict:
            raise
        return []
    return sorted(int(m.group(1)) for m in (ITER_DIR.match(n) for n in names) if m)


def load_val_losses(path):
    """Return the validation losses by step that earlier attempts recorded in metrics.jsonl."""
    out = {}
    if not path or not os.path.exists(path):
        return out
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                record = json.loads(line)
            except ValueError:
                continue
            for name, value in (record.get("v") or {}).items():
                if VAL_LOSS.match(name) and isinstance(value, (int, float)) and math.isfinite(value):
                    out[int(record.get("step", -1))] = value
    return out


class StageError(Exception):
    """A stage failure; retry is False when running the workload again would fail the same way."""

    def __init__(self, stage, reason, retry=True):
        super().__init__(reason)
        self.stage, self.reason, self.retry = stage, reason, retry


def dir_size(path):
    """Return the total size of the files under path, 0 when it does not exist."""
    total = 0
    for root, _, names in os.walk(path):
        for name in names:
            try:
                total += os.path.getsize(os.path.join(root, name))
            except OSError:
                pass
    return total


def disk_usage(run_dir):
    """Return the sizes of the run directory parts that hold checkpoints and exported weights, and their total."""
    sizes = {key: dir_size(os.path.join(run_dir, name)) for key, name in DISK_DIRS}
    sizes["total"] = sum(sizes.values())
    return sizes


def under(root, path):
    """Report whether path is root or lies inside it, comparing absolute paths."""
    root, path = os.path.abspath(root), os.path.abspath(path)
    return path == root or path.startswith(root.rstrip(os.sep) + os.sep)


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


def storage_full(run_dir):
    """Tell whether the storage of the run directory has less than FULL_FREE_BYTES free."""
    try:
        return shutil.disk_usage(run_dir).free < FULL_FREE_BYTES
    except OSError:
        return False


def space_reason(run_dir):
    """Describe a full shared storage with its free space and the run directory's usage."""
    try:
        free = shutil.disk_usage(run_dir).free
    except OSError:
        free = 0
    return "shared storage is out of space (%.1f GiB free, run directory %.1f GiB)" % (
        free / float(1 << 30), disk_usage(run_dir)["total"] / float(1 << 30))


def weights_fingerprint(model_dir):
    """Return the sha256 of the names, sizes and modification times of a model's config and weight files."""
    digest = hashlib.sha256()
    for name in sorted(os.listdir(model_dir)):
        if name == "config.json" or name.endswith((".safetensors", ".bin", ".pth", ".pt")):
            st = os.stat(os.path.join(model_dir, name))
            digest.update(("%s\0%d\0%d\n" % (name, st.st_size, int(st.st_mtime))).encode())
    return digest.hexdigest()


def lock_owner(lock):
    """Return the owner record of a conversion lock, {} when it has none yet or it cannot be read."""
    try:
        with open(os.path.join(lock, LOCK_OWNER)) as f:
            owner = json.load(f)
    except (OSError, ValueError):
        return {}
    return owner if isinstance(owner, dict) else {}


def write_owner(lock, owner):
    """Write the owner record of a conversion lock in one step, so that a reader never finds half of it."""
    tmp = os.path.join(lock, LOCK_OWNER + ".tmp")
    with open(tmp, "w") as f:
        json.dump(owner, f)
    os.replace(tmp, os.path.join(lock, LOCK_OWNER))


def take_over(lock, held, me):
    """Take a stale lock whose owner record was held over for me, and return whether me holds it now. The lock
    stays where it is: the owner record is what is taken, by a rename only one run can win, and a record that is
    not the one found stale belongs to a run that took the lock meanwhile and goes back. A lock without a record,
    which a launcher from before the heartbeat left, is moved away as a whole for the next mkdir to win."""
    if not held:
        gone = "%s.%s" % (lock, me["token"])
        try:
            os.rename(lock, gone)
        except OSError:
            return False
        shutil.rmtree(gone, ignore_errors=True)
        return False
    record = os.path.join(lock, LOCK_OWNER)
    claimed = "%s.%s" % (record, me["token"])
    try:
        # Refreshed first, so that no other run finds the lock stale while it has no owner record.
        os.utime(lock)
        os.rename(record, claimed)
    except OSError:
        return False
    try:
        with open(claimed) as f:
            taken = json.load(f)
    except (OSError, ValueError):
        taken = None
    if not isinstance(taken, dict) or taken.get("token") != held.get("token"):
        try:
            os.rename(claimed, record)
        except OSError:
            pass
        return False
    try:
        write_owner(lock, me)
        os.remove(claimed)
    except OSError:
        return False
    return True


@contextlib.contextmanager
def cache_lock(root, owner=None, sleep=time.sleep, clock=time.time):
    """Hold the conversion lock of a cache directory. mkdir is atomic on the shared storage, so one run converts
    while the others wait. The holder records owner and a token in the lock and refreshes it every
    LOCK_HEARTBEAT_SECONDS; a lock not refreshed for LOCK_STALE_SECONDS, or for LOCK_UNOWNED_STALE_SECONDS when it
    has no owner record, or left by an earlier attempt of the same run, is taken over, and a holder removes the lock
    only while its token is still there. It yields a function that tells whether the lock is still the holder's."""
    os.makedirs(root, exist_ok=True)
    lock = os.path.join(root, CONVERT_LOCK)
    me = dict(owner or {}, token=uuid.uuid4().hex)
    while True:
        try:
            os.mkdir(lock)
        except FileExistsError:
            try:
                age = clock() - os.path.getmtime(lock)
            except OSError:
                continue
            held = lock_owner(lock)
            earlier = me.get("run") and held.get("run") == me["run"] and held.get("attempt") != me.get("attempt")
            if age > (LOCK_STALE_SECONDS if held else LOCK_UNOWNED_STALE_SECONDS) or earlier:
                if take_over(lock, held, me):
                    break
                continue
            sleep(LOCK_POLL_SECONDS)
            continue
        try:
            write_owner(lock, me)
        except OSError:
            # A lock without an owner record would be waited for far longer than one whose holder died.
            shutil.rmtree(lock, ignore_errors=True)
            raise
        break
    stop = threading.Event()

    def other():
        """Return the token of another run in the lock, None when the lock has none or cannot be read."""
        token = lock_owner(lock).get("token")
        return token if token != me["token"] else None

    def heartbeat():
        """Refresh the lock's mtime until the holder is done or another run took the lock over."""
        while not stop.wait(LOCK_HEARTBEAT_SECONDS):
            if other():
                return
            try:
                os.utime(lock)
            except OSError:
                pass

    beat = threading.Thread(target=heartbeat, daemon=True)
    beat.start()
    try:
        yield lambda: os.path.isdir(lock) and not other()
    finally:
        stop.set()
        beat.join()
        if lock_owner(lock).get("token") == me["token"]:
            shutil.rmtree(lock, ignore_errors=True)


def weights_size(model_dir):
    """Return the size of a HF model's weight files."""
    total = 0
    for name in os.listdir(model_dir) if os.path.isdir(model_dir) else []:
        if name.endswith((".safetensors", ".bin")):
            total += os.path.getsize(os.path.join(model_dir, name))
    return total


def ray_gpus():
    """Return the GPU count of the job's Ray cluster."""
    code = "import json, ray; ray.init(address='auto', logging_level='ERROR'); print(json.dumps(ray.cluster_resources()))"
    out = subprocess.run([sys.executable, "-c", code], capture_output=True, text=True, timeout=300)
    if out.returncode != 0:
        raise StageError("check", "cannot read the Ray cluster resources: " + out.stderr.strip()[-500:])
    return int(json.loads(out.stdout.strip().splitlines()[-1]).get("GPU", 0))


# NODE_PROBE_CODE prints {host, memory, rdma} for each GPU node of the Ray cluster: the smaller of MemTotal and the
# container's cgroup memory limit, which is what training can use there, and the RDMA devices the node has.
NODE_PROBE_CODE = """
import json, ray
from ray.util.scheduling_strategies import NodeAffinitySchedulingStrategy
ray.init(address="auto", logging_level="ERROR")

@ray.remote(num_cpus=0)
def probe():
    import os, socket
    total = 0
    with open("/proc/meminfo") as f:
        for line in f:
            if line.startswith("MemTotal:"):
                total = int(line.split()[1]) * 1024
    for path in ("/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"):
        try:
            with open(path) as f:
                limit = f.read().strip()
        except OSError:
            continue
        if limit.isdigit() and 0 < int(limit) < total:
            total = int(limit)
        break
    try:
        rdma = sorted(os.listdir("/sys/class/infiniband"))
    except OSError:
        rdma = []
    return {"host": socket.gethostname(), "memory": total, "rdma": rdma}

nodes = [n["NodeID"] for n in ray.nodes() if n.get("Alive") and n.get("Resources", {}).get("GPU", 0) > 0]
refs = [probe.options(scheduling_strategy=NodeAffinitySchedulingStrategy(n, soft=False)).remote() for n in nodes]
print(json.dumps(ray.get(refs, timeout=240)))
"""


def ray_nodes():
    """Return each GPU node of the job's Ray cluster as {host, memory, rdma}: the host memory training can use there
    and the node's RDMA devices."""
    out = subprocess.run([sys.executable, "-c", NODE_PROBE_CODE], capture_output=True, text=True, timeout=300)
    if out.returncode != 0:
        raise StageError("check", "cannot probe the nodes: " + out.stderr.strip()[-500:])
    return json.loads(out.stdout.strip().splitlines()[-1])


class Runner:
    """Runs the stages of one recipe."""

    def __init__(self, recipe, run_dir, adapter, emitter, gpus=ray_gpus, watchdog=None, sleep=time.sleep,
                 nodes=ray_nodes):
        self.recipe, self.run_dir, self.adapter, self.emitter, self.gpus = recipe, run_dir, adapter, emitter, gpus
        self.nodes = nodes
        self.watchdog, self.sleep = watchdog, sleep
        self.enospc = False
        self.reward_rejected = False
        self.checkpoint = None
        self.wandb_link = None
        # val_losses maps a step to its validation loss, for keeping the best checkpoint.
        self.val_losses = {}

    def path(self, *parts):
        """Return a path under the run directory."""
        return os.path.join(self.run_dir, *parts)

    def done_mark(self, stage):
        """Return the file whose presence means a stage finished, or None for stages that always run."""
        convert = self.recipe.get("convert") or {}
        return {
            "prepare": self.path("data", ".done"),
            "train": self.path(".train.done"),
            "export": self.path("final_model", ".done"),
        }.get(stage)

    def convert_paths(self):
        """Return the conversion's output directory, its cache directory and its completion mark."""
        out = (self.recipe.get("convert") or {}).get("dir", self.path("convert", "torch_dist"))
        return out, os.path.dirname(out), os.path.join(out, ".complete")

    def fp8_paths(self):
        """Return the FP8 HF weights' directory next to the conversion and its completion mark."""
        out = os.path.join(self.convert_paths()[1], FP8_DIR)
        return out, os.path.join(out, ".complete")

    def wants_fp8(self):
        """Tell whether the rollout serves FP8 weights, which the convert stage makes next to the conversion."""
        return bool((self.recipe.get("convert") or {}).get("fp8"))

    def hf_direct(self):
        """Tell whether training loads the base HF weights and convert only builds FP8 when asked."""
        return bool((self.recipe.get("convert") or {}).get("hfDirect"))

    def report_conversion(self, state):
        """Print the convert [STAGE] that registers a shared conversion with the controller."""
        out, root, _ = self.convert_paths()
        fp8_out, fp8_complete = self.fp8_paths()
        detail = None
        if (self.recipe.get("convert") or {}).get("cacheKey"):
            fp8 = os.path.exists(fp8_complete)
            torch_size = 0 if self.hf_direct() else dir_size(out)
            detail = {"cacheKey": self.recipe["convert"]["cacheKey"], "dir": root, "fp8": fp8,
                      "sizeBytes": torch_size + (dir_size(fp8_out) if fp8 else 0)}
        self.emitter.stage("convert", state=state, detail=detail)

    def check_source(self, root):
        """Fail unless the conversion in root was made from the model's current weights."""
        try:
            with open(os.path.join(root, SOURCE_FILE)) as f:
                source = json.load(f)
        except (OSError, ValueError):
            source = {}
        if source.get("fingerprint") != weights_fingerprint(self.recipe["model"]["path"]):
            raise StageError("convert", "the conversion cache does not match the model weights", retry=False)

    def write_source(self, root):
        """Record the model fingerprint next to a shared conversion so later runs can refuse a stale cache."""
        source = {"model": self.recipe["model"]["path"],
                  "fingerprint": weights_fingerprint(self.recipe["model"]["path"])}
        with open(os.path.join(root, SOURCE_FILE), "w") as f:
            json.dump(source, f)

    def reuse_conversion(self):
        """Report a complete conversion of the model's current weights as skipped; returns whether there is one."""
        _, root, complete = self.convert_paths()
        if self.hf_direct():
            complete = os.path.join(root, SOURCE_FILE)
        if not os.path.exists(complete) or (self.wants_fp8() and not os.path.exists(self.fp8_paths()[1])):
            return False
        self.check_source(root)
        self.report_conversion("skipped")
        return True

    def run(self):
        """Run every stage that has not finished yet and return the export result."""
        result = None
        for stage in STAGES:
            if stage == "convert" and not self.recipe.get("convert"):
                continue
            if stage == "convert" and self.reuse_conversion():
                continue
            if stage == "export" and not self.exports_hf():
                self.emitter.stage(stage, state="skipped")
                result = self.no_export_result()
                continue
            mark = self.done_mark(stage)
            if mark and os.path.exists(mark):
                self.emitter.stage(stage, state="skipped")
                if stage == "export":
                    result = self.read_result()
                continue
            self.emitter.stage(stage, state="running")
            out = getattr(self, "stage_" + stage)()
            if mark:
                os.makedirs(os.path.dirname(mark), exist_ok=True)
                open(mark, "w").close()
            if stage == "export":
                result = out
        return result

    def stage_prepare(self):
        """Convert the training data, and the validation data when the recipe has some."""
        data, train_type = self.recipe["data"], self.recipe["trainType"]
        field_map = data.get("fieldMap") or {}
        val = self.recipe.get("val") or {}
        split = val.get("split") or 0
        try:
            stats = {"train": dataset.prepare(
                data["train"], self.path("data", "train.jsonl"), train_type, field_map,
                split_dst=self.path("data", "eval.jsonl") if split else None, split=split,
                split_limit=val.get("maxSamples") or None)}
            if data.get("eval"):
                stats["eval"] = dataset.prepare(data["eval"], self.path("data", "eval.jsonl"), train_type, field_map,
                                                limit=val.get("maxSamples") or None)
        except dataset.DatasetError as e:
            raise StageError("prepare", str(e), retry=False) from e
        with open(self.path("data", "stats.json"), "w") as f:
            json.dump(stats, f)
        self.emitter.stage("prepare", state="done", detail=stats)

    def keep_conversion(self, held):
        """Fail the stage instead of marking a conversion complete when another run took the lock over meanwhile,
        as both then wrote the cache directory; the next attempt converts again under the lock."""
        if not held():
            raise StageError("convert", "another run took the conversion lock over; the conversion is not kept")

    def stage_convert(self):
        """Convert the base weights for training under the cache lock, unless a run that held the lock first
        converted them, and record the source fingerprint before the completion mark. HF-direct recipes skip
        torch_dist and only build FP8 weights when the rollout needs them."""
        _, root, complete = self.convert_paths()
        fp8_out, fp8_complete = self.fp8_paths()
        run_command = functools.partial(self.run_command, watch="convert")
        owner = {"run": self.emitter.run_id, "attempt": self.emitter.attempt, "host": socket.gethostname(),
                 "pid": os.getpid()}
        with cache_lock(root, owner=owner, sleep=self.sleep) as held:
            if self.hf_direct():
                if not os.path.exists(os.path.join(root, SOURCE_FILE)):
                    self.write_source(root)
            elif not os.path.exists(complete):
                self.adapter.convert(self.recipe, self.run_dir, run_command)
                self.keep_conversion(held)
                self.write_source(root)
                open(complete, "w").close()
            if self.wants_fp8() and not os.path.exists(fp8_complete):
                self.adapter.convert_fp8(self.recipe, self.run_dir, run_command, fp8_out)
                self.keep_conversion(held)
                open(fp8_complete, "w").close()
        self.check_source(root)
        self.report_conversion("done")

    def stage_check(self):
        """Check the GPU count, the nodes' host memory and RDMA devices, the inputs and the free space before
        training; the checkpoints and model an earlier attempt already wrote count as space the run has."""
        layout = self.recipe["layout"]
        want = layout["nodes"] * layout["gpusPerNode"]
        have = self.gpus()
        if have < want:
            raise StageError("check", "the Ray cluster has %d GPUs, the recipe needs %d" % (have, want))
        if self.recipe.get("minHostMemoryTB") or self.rdma_devices():
            nodes = self.nodes()
            self.check_host_memory(nodes)
            self.check_rdma(nodes)
        model = self.recipe["model"]["path"]
        for path in (os.path.join(model, "config.json"), self.path("data", "train.jsonl")):
            if not os.path.exists(path):
                raise StageError("check", "missing input %s" % path)
        self.check_freeze()
        usage = disk_usage(self.run_dir)
        need = max(0, self.space_needed() - usage["total"])
        free = shutil.disk_usage(self.run_dir).free
        if free < need:
            raise StageError("check", "shared storage has %d GiB free, the run needs about %d GiB"
                             % (free >> 30, need >> 30), retry=False)
        self.check_wandb()
        self.check_reward()
        self.check_val()

    def check_val(self):
        """Fail before training when an SFT run's validation set has fewer rows than the data-parallel ranks, which
        its eval loss needs, and note the batch the eval loss uses when the set is smaller than a global batch."""
        if self.recipe.get("trainType") != "sft":
            return
        try:
            with open(self.path("data", "stats.json")) as f:
                stats = json.load(f)
        except (OSError, ValueError):
            return
        rows = (stats.get("eval") or {}).get("written", (stats.get("train") or {}).get("split", 0))
        if not rows:
            return
        choices = self.recipe.get("choices") or {}
        model_parallel = (choices.get("tp") or 1) * (choices.get("pp") or 1) * (choices.get("cp") or 1)
        dp = max(1, self.recipe["layout"]["actorGpus"] // model_parallel)
        if rows < dp:
            raise StageError("check", "the validation set has %d rows, fewer than the %d data-parallel ranks its "
                             "eval loss splits them over" % (rows, dp), retry=False)
        if rows < (choices.get("batchSize") or 0):
            self.emitter.stage("check", state="running", detail={"valRows": rows, "valBatch": rows // dp * dp})

    def check_freeze(self):
        """Fail before training when a pattern of the parameters to train is not a Python regex, which slime
        matches with re.search."""
        for pattern in ((self.recipe.get("choices") or {}).get("freeze") or {}).get("trainOnly") or []:
            try:
                re.compile(pattern)
            except re.error as e:
                raise StageError("check", "the pattern to train %r is not a valid regex: %s" % (pattern, e),
                                 retry=False)

    def check_host_memory(self, nodes):
        """Fail before training when a node has less host memory than the model's spec needs."""
        need = self.recipe.get("minHostMemoryTB") or 0
        short = [n for n in nodes if need and n["memory"] < need * 1e12]
        if short:
            raise StageError("check", "%s needs %.1f TB of host memory per node: %s" % (
                (self.recipe.get("model") or {}).get("spec") or "the model", need,
                ", ".join("%s has %.2f TB" % (n["host"], n["memory"] / 1e12) for n in short)), retry=False)

    def rdma_devices(self):
        """Return the RDMA devices Mooncake PD moves the KV cache over, or none without PD."""
        return ((self.recipe.get("rollout") or {}).get("pd") or {}).get("rdmaDevices") or []

    def check_rdma(self, nodes):
        """Fail before training when Mooncake PD is on and a node lacks one of the cluster's RDMA devices."""
        want = self.rdma_devices()
        missing = [(n["host"], [d for d in want if d not in n["rdma"]]) for n in nodes]
        missing = [(host, devices) for host, devices in missing if devices]
        if missing:
            raise StageError("check", "Mooncake PD needs the RDMA devices %s on every node: %s" % (
                ", ".join(want), "; ".join("%s lacks %s" % (host, ", ".join(devices)) for host, devices in missing)),
                retry=False)

    def check_wandb(self, timeout=WANDB_PROBE_SECONDS):
        """Fail before training when W&B reporting is on and the W&B server does not answer."""
        wandb = (self.recipe.get("tracking") or {}).get("wandb") or {}
        if not wandb.get("enabled"):
            return
        url = os.environ.get("WANDB_BASE_URL") or WANDB_DEFAULT_URL
        if http_status(url, timeout) is None:
            raise StageError("check", "the training nodes can't reach wandb at %s" % url)

    def check_reward(self, timeout=REWARD_PROBE_SECONDS):
        """Fail before training when an RL run's reward can't work: the custom reward file is missing, or the remote
        reward service does not answer or refuses the key."""
        if self.recipe.get("trainType") != "rl":
            return
        choices = self.recipe.get("choices") or {}
        if choices.get("rewardType") == "custom":
            path = (choices.get("rewardHook") or "").partition(":")[0]
            if not os.path.isfile(path):
                raise StageError("check", "the custom reward file %s does not exist" % path, retry=False)
        elif choices.get("rewardType") == "remote":
            remote = choices.get("rewardRemote") or {}
            url = remote.get("url", "")
            headers = {}
            if remote.get("secretId"):
                headers["Authorization"] = "Bearer " + os.environ.get(REWARD_KEY_ENV, "")
            status = http_status(url, timeout, headers)
            if status is None:
                raise StageError("check", "the training nodes can't reach the reward service at %s" % url)
            if headers and status in (401, 403):
                raise StageError("check", "the reward service at %s rejected the key (HTTP %d)" % (url, status),
                                 retry=False)

    def space_needed(self):
        """Estimate the bytes the run writes: the checkpoints kept at the same time plus the exported model. With a
        retention policy that is keepLast, the best one and the one being written; without one, every save. A PPO
        critic keeps as many checkpoints as the actor. The spec's checkpointTB, measured for its default freezing,
        replaces the estimate of one checkpoint."""
        weights = weights_size(self.recipe["model"]["path"])
        keep = (self.recipe.get("checkpoint") or {}).get("keepLast", 0)
        choices = self.recipe.get("choices") or {}
        steps, interval = choices.get("numRollout") or 0, choices.get("saveInterval") or 0
        if keep > 0:
            count = keep + 2
        elif steps and interval:
            count = max(1, steps // interval)
        else:
            count = 3
        if self.recipe.get("critic"):
            count *= 2
        per = int((self.recipe.get("checkpointTB") or 0) * 1e12) or weights * CHECKPOINT_FACTOR
        return per * count + weights

    def critic_dir(self):
        """Return the directory of a PPO run's critic checkpoints, or None when the run has no critic."""
        critic = self.recipe.get("critic")
        return (critic.get("checkpoints") or "critic_checkpoints") if critic else None

    def tracker(self, sub):
        """Return the iteration the checkpoint tracker of sub names, or None when sub never finished a save. A
        tracker that is there and names none fails the stage before any checkpoint is removed: a read error may
        pass on the next attempt, an empty or damaged tracker does not."""
        try:
            return self.adapter.latest_checkpoint(self.run_dir, sub, strict=True)
        except OSError as e:
            raise StageError("train", "the checkpoint tracker of %s cannot be read: %s" % (sub, e))
        except ValueError:
            raise StageError("train", "the checkpoint tracker of %s is empty or damaged; its checkpoints are kept"
                             % sub, retry=False)

    def align_critic(self):
        """Point a PPO run's actor and critic at the latest iteration both saved, which a crash between their saves
        or a critic-only step can leave apart, and remove their newer iterations; with none shared both start
        over."""
        sub = self.critic_dir()
        if not sub:
            return
        actor, critic = self.tracker("checkpoints"), self.tracker(sub)
        if actor == critic:
            return
        limit = -1 if actor is None or critic is None else min(actor, critic)
        try:
            # A directory that cannot be listed is not one without checkpoints, which would remove the other's.
            saved = {name: saved_iterations(self.path(name), strict=True) for name in ("checkpoints", sub)}
        except OSError as e:
            raise StageError("train", "the checkpoints cannot be listed: %s" % e)
        shared = set(saved["checkpoints"]) & set(saved[sub])
        common = max((s for s in shared if s <= limit), default=None)
        for name in ("checkpoints", sub):
            for step in saved[name]:
                if common is None or step > common:
                    shutil.rmtree(self.path(name, "iter_%07d" % step), ignore_errors=True)
            self.adapter.set_latest_checkpoint(self.run_dir, name, common)
        self.emitter.notice("checkpoint", {"actor": actor, "critic": critic, "resume": common})

    def stage_train(self):
        """Run the training command, resuming from the latest checkpoint when there is one."""
        self.align_critic()
        cmd, env, files = self.adapter.train_command(self.recipe, self.run_dir)
        for rel, content in (files or {}).items():
            target = self.path(rel)
            os.makedirs(os.path.dirname(target), exist_ok=True)
            with open(target, "w") as f:
                f.write(content)
        self.val_losses.update(load_val_losses(self.emitter.metrics_path))
        # Read strictly: a tracker that is there and names no iteration must not start the training over.
        resume = self.checkpoint = self.tracker("checkpoints")
        if resume is not None:
            cmd = cmd + ["--load", self.path("checkpoints")]
            self.emitter.stage("train", state="running", detail={"resumeStep": resume})
        code = self.run_command(cmd, env=env, parse=True, watch="train")
        # A command, of this attempt or an earlier one, can end within a tick of a save, before the retention ran
        # for it; a critic can finish a save after the tick that saw the actor's.
        self.apply_retention(self.adapter.latest_checkpoint(self.run_dir))
        self.report_disk()
        if code != 0:
            if self.watchdog and self.watchdog.reason:
                raise StageError("train", self.watchdog.reason)
            if self.enospc:
                raise StageError("train", space_reason(self.run_dir), retry=False)
            if self.reward_rejected:
                raise StageError("train", "the reward service rejected the key", retry=False)
            raise StageError("train", "training exited with code %d" % code)

    def checkpoint_tick(self):
        """Notice a checkpoint that finished saving: it counts as progress and the disk usage is reported."""
        latest = self.adapter.latest_checkpoint(self.run_dir)
        if latest != self.checkpoint:
            self.checkpoint = latest
            self.watchdog.saved()
            self.apply_retention(latest)
            self.report_disk()

    def apply_retention(self, latest):
        """Delete the saved checkpoints the spec's retention no longer keeps once latest is saved: all but the latest
        keepLast and, for SFT with keepBest, the one with the lowest validation loss. PPO critic checkpoints go at the
        same steps, and only up to the latest iteration the critic also saved, so actor and critic keep one they
        share."""
        policy = self.recipe.get("checkpoint") or {}
        keep_last = int(policy.get("keepLast") or 0)
        critic_sub = self.critic_dir()
        if critic_sub and latest is not None:
            critic = self.adapter.latest_checkpoint(self.run_dir, critic_sub)
            latest = None if critic is None else min(latest, critic)
        if keep_last <= 0 or latest is None:
            return
        saved = [s for s in saved_iterations(self.path("checkpoints")) if s <= latest]
        keep = set(saved[-keep_last:])
        if policy.get("keepBest") and self.recipe.get("trainType") != "rl":
            # The validation after rollout s, which slime saves as iter_s, is recorded at step s + 1.
            scored = [s for s in saved if s + 1 in self.val_losses]
            if scored:
                keep.add(min(scored, key=lambda s: self.val_losses[s + 1]))
        deleted = [s for s in saved if s not in keep]
        for step in deleted:
            for sub in ("checkpoints", critic_sub or "critic_checkpoints"):
                shutil.rmtree(self.path(sub, "iter_%07d" % step), ignore_errors=True)
        if deleted:
            self.emitter.notice("retention", {"deleted": deleted, "kept": sorted(keep)})

    def report_disk(self):
        """Report the run directory usage in a [STAGE] marker."""
        self.emitter.notice("disk", disk_usage(self.run_dir))

    def stage_export(self):
        """Export the final model and return the result the launcher reports."""
        iteration = self.adapter.latest_checkpoint(self.run_dir)
        output = self.adapter.export(self.recipe, self.run_dir, self.run_command, iteration=iteration)
        missing = [n for n in ("config.json",) if not os.path.exists(os.path.join(output, n))]
        if not any(n.endswith((".safetensors", ".bin")) for n in os.listdir(output)):
            missing.append("weights")
        if not any(n.startswith("tokenizer") for n in os.listdir(output)):
            missing.append("tokenizer")
        if missing:
            raise StageError("export", "the exported model lacks %s" % ", ".join(missing), retry=False)
        result = {"stage": "export", "output": output, "iteration": iteration,
                  "lastMetrics": self.emitter.last_metrics()}
        self.write_result(result)
        self.report_disk()
        return result

    def exports_hf(self):
        """Report whether the run exports a Hugging Face model, which it does unless the recipe turns it off."""
        return (self.recipe.get("output") or {}).get("exportHf", True) is not False

    def no_export_result(self):
        """Return and write the result of a run that exports no model: the latest checkpoint and no output."""
        result = {"stage": "export", "output": "", "iteration": self.adapter.latest_checkpoint(self.run_dir),
                  "lastMetrics": self.emitter.last_metrics()}
        self.write_result(result)
        self.report_disk()
        return result

    def write_result(self, result):
        """Write result.json, which the verification job reads when the result marker is lost."""
        path = self.path("result.json")
        tmp = path + ".tmp"
        with open(tmp, "w") as f:
            json.dump(result, f)
        os.replace(tmp, path)

    def read_result(self):
        """Read the result an earlier attempt wrote in the run's result.json."""
        try:
            with open(self.path("result.json")) as f:
                return json.load(f)
        except (OSError, ValueError):
            return {"stage": "export", "output": self.path("final_model")}

    def run_command(self, cmd, env=None, parse=False, watch=None):
        """Run a command in its own process group, forward its output and, when parse is set, turn metric lines
        into [METRIC] markers. With watch naming the stage, the watchdog ends the command when it stops making
        progress; so does any error while reading its output. Only an out-of-space line that names a path in the run
        directory counts as the shared storage being full. Returns the exit code."""
        full_env = dict(os.environ)
        full_env.update(env or {})
        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env=full_env,
                                cwd=self.run_dir, start_new_session=True, text=True, errors="replace", bufsize=1)
        dog = self.watchdog if watch else None
        if dog:
            dog.watch(proc, watch, tick=self.checkpoint_tick if watch == "train" else None)
        try:
            for line in proc.stdout:
                self.emitter.forward(line)
                # A write to an open file fails without naming it, so a line that does not name the run directory
                # counts once the storage of the run directory is full.
                if ENOSPC_TEXT in line and (self.run_dir in line or storage_full(self.run_dir)):
                    self.enospc = True
                if REWARD_REJECTED_TEXT in line:
                    self.reward_rejected = True
                if dog:
                    dog.line(line)
                if parse:
                    self.parse_line(line)
            return proc.wait()
        finally:
            if dog:
                dog.unwatch()
            if proc.poll() is None:
                stop_group(proc, KILL_GRACE)

    def parse_line(self, line):
        """Report the metrics of one training output line, or the first W&B run link; a line that looks like metrics
        but does not parse is counted."""
        if self.wandb_link is None:
            m = WANDB_RUN_URL.search(line)
            if m:
                self.wandb_link = m.group(1)
                self.emitter.link("wandb", self.wandb_link)
                return
        try:
            parsed = self.adapter.parse_metric_line(line)
        except Exception:  # noqa: BLE001 - a line the parser cannot handle must not stop the run
            self.emitter.parse_failures += 1
            return
        if parsed:
            kind, axis, step, values = parsed
            if self.emitter.metric(kind, axis, step, values) and self.watchdog and kind in PROGRESS_KINDS:
                self.watchdog.progress(step)
            for name, value in values.items():
                if VAL_LOSS.match(name) and isinstance(value, (int, float)) and math.isfinite(value):
                    self.val_losses[int(step)] = value
