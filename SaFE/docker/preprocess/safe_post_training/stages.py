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
import subprocess
import sys
import time
import urllib.error
import urllib.request

from safe_post_training import dataset

STAGES = ("prepare", "convert", "check", "train", "export")
# bf16 weights plus fp32 master weights and two fp32 Adam moments, relative to the bf16 weights on disk.
CHECKPOINT_FACTOR = 7
DISK_DIRS = (("checkpoints", "checkpoints"), ("criticCheckpoints", "critic_checkpoints"),
             ("finalModel", "final_model"), ("finalAdapter", "final_adapter"), ("exports", "exports"))
PROGRESS_KINDS = ("step", "rollout", "eval")
ENOSPC_TEXT = "No space left on device"
CONVERT_LOCK = ".lock"
# FP8_DIR holds the FP8 HF weights next to a conversion, made when an FP8 rollout needs them.
FP8_DIR = "fp8_hf"
SOURCE_FILE = "source.json"
# A conversion lock older than this was left by a conversion that died; the next run takes it over.
LOCK_STALE_SECONDS = 6 * 3600
ITER_DIR = re.compile(r"^iter_(\d{7})$")
SAMPLES_DIR = "samples"
SAMPLE_FILE = re.compile(r"^(rollout|eval)-(\d+)\.jsonl$")
MAX_SAMPLE_LINE = 32 << 10
MAX_SAMPLE_FILE = 1 << 20
VAL_LOSS = re.compile(r"^eval/[^/]+/loss$")
WANDB_DEFAULT_URL = "https://api.wandb.ai"
WANDB_PROBE_SECONDS = 10
REWARD_PROBE_SECONDS = 10
REWARD_KEY_ENV = "PT_RM_API_KEY"
# REWARD_REJECTED_TEXT is what the remote_rm plugin prints when the reward service refuses the key.
REWARD_REJECTED_TEXT = "safe-post-training: the reward service rejected the key"
WANDB_RUN_URL = re.compile(r"View run\b.*?(https?://\S+)")
LOCK_POLL_SECONDS = 30


def http_status(url, timeout, headers=None):
    """Return the status of a HEAD request to url within timeout seconds, or None when no HTTP server answers."""
    try:
        with urllib.request.urlopen(urllib.request.Request(url, method="HEAD", headers=headers or {}),
                                    timeout=timeout) as resp:
            return resp.status
    except urllib.error.HTTPError as e:
        return e.code
    except (urllib.error.URLError, OSError, ValueError):
        return None


def truncate(text, head, tail=0):
    """Keep the head and the tail of a text and name how many characters of the middle were left out."""
    if len(text) <= head + tail:
        return text
    omitted = len(text) - head - tail
    return "%s … [%d characters omitted] … %s" % (text[:head], omitted, text[len(text) - tail:] if tail else "")


def saved_iterations(path):
    """Return the iterations of the iter_NNNNNNN checkpoint directories under path, in order."""
    try:
        names = os.listdir(path)
    except OSError:
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


@contextlib.contextmanager
def cache_lock(root, sleep=time.sleep, clock=time.time):
    """Hold the conversion lock of a cache directory. mkdir is atomic on the shared storage, so one run converts
    while the others wait; a lock older than LOCK_STALE_SECONDS is taken over."""
    os.makedirs(root, exist_ok=True)
    lock = os.path.join(root, CONVERT_LOCK)
    while True:
        try:
            os.mkdir(lock)
            break
        except FileExistsError:
            try:
                age = clock() - os.path.getmtime(lock)
            except OSError:
                continue
            if age > LOCK_STALE_SECONDS:
                shutil.rmtree(lock, ignore_errors=True)
                continue
            sleep(LOCK_POLL_SECONDS)
    try:
        yield
    finally:
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
                 sampler=None, nodes=ray_nodes):
        self.recipe, self.run_dir, self.adapter, self.emitter, self.gpus = recipe, run_dir, adapter, emitter, gpus
        self.nodes = nodes
        self.watchdog, self.sleep = watchdog, sleep
        # sampler, when set, returns the GPU sampler that runs alongside training.
        self.sampler = sampler
        self.enospc = False
        self.reward_rejected = False
        self.checkpoint = None
        self.wandb_link = None
        # val_losses maps a step to its validation loss, for keeping the best checkpoint.
        self.val_losses = {}
        self.reported_samples = set()

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

    def export_only(self):
        """Return the checkpoint export the recipe asks for instead of a run, {"step": N, "dir": ...}, or None."""
        return self.recipe.get("exportOnly")

    def run(self):
        """Run every stage that has not finished yet and return the export result; a checkpoint export runs only the
        export stage, into its own directory."""
        only = self.export_only()
        if only:
            mark = os.path.join(only["dir"], ".done")
            if os.path.exists(mark):
                self.emitter.stage("export", state="skipped")
                return self.read_result(os.path.join(only["dir"], "result.json"), only["dir"])
            self.emitter.stage("export", state="running")
            result = self.stage_export()
            open(mark, "w").close()
            return result
        result = None
        for stage in STAGES:
            if stage == "convert" and not self.recipe.get("convert"):
                continue
            if stage == "convert" and self.reuse_conversion():
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

    def stage_convert(self):
        """Convert the base weights for training under the cache lock, unless a run that held the lock first
        converted them, and record the source fingerprint before the completion mark. HF-direct recipes skip
        torch_dist and only build FP8 weights when the rollout needs them."""
        _, root, complete = self.convert_paths()
        fp8_out, fp8_complete = self.fp8_paths()
        run_command = functools.partial(self.run_command, watch="convert")
        with cache_lock(root, sleep=self.sleep):
            if self.hf_direct():
                if not os.path.exists(os.path.join(root, SOURCE_FILE)):
                    self.write_source(root)
            elif not os.path.exists(complete):
                self.adapter.convert(self.recipe, self.run_dir, run_command)
                self.write_source(root)
                open(complete, "w").close()
            if self.wants_fp8() and not os.path.exists(fp8_complete):
                self.adapter.convert_fp8(self.recipe, self.run_dir, run_command, fp8_out)
                open(fp8_complete, "w").close()
        self.check_source(root)
        self.report_conversion("done")

    def stage_check(self):
        """Check the GPU count, the nodes' host memory and RDMA devices, the inputs and the free space before
        training."""
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
        need = self.space_needed()
        free = shutil.disk_usage(self.run_dir).free
        if free < need:
            raise StageError("check", "shared storage has %d GiB free, the run needs about %d GiB"
                             % (free >> 30, need >> 30), retry=False)
        self.check_wandb()
        self.check_reward()

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

    def stage_train(self):
        """Run the training command, resuming from the latest checkpoint when there is one."""
        cmd, env, files = self.adapter.train_command(self.recipe, self.run_dir)
        for rel, content in (files or {}).items():
            target = self.path(rel)
            os.makedirs(os.path.dirname(target), exist_ok=True)
            with open(target, "w") as f:
                f.write(content)
        self.val_losses.update(load_val_losses(self.emitter.metrics_path))
        resume = self.checkpoint = self.adapter.latest_checkpoint(self.run_dir)
        critic = self.recipe.get("critic")
        if critic:
            critic_dir = critic.get("checkpoints") or "critic_checkpoints"
            critic_step = self.adapter.latest_checkpoint(self.run_dir, critic_dir)
            if critic_step != resume:
                raise StageError("train", "the actor checkpoints end at step %s but the critic checkpoints at step %s"
                                 % (resume, critic_step), retry=False)
        if resume is not None:
            cmd = cmd + ["--load", self.path("checkpoints")]
            self.emitter.stage("train", state="running", detail={"resumeStep": resume})
        sampler = self.sampler() if self.sampler else None
        if sampler:
            sampler.start()
        try:
            code = self.run_command(cmd, env=env, parse=True, watch="train")
        finally:
            if sampler:
                sampler.stop()
            self.sample_tick()
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
        same steps, so actor and critic stay in step."""
        policy = self.recipe.get("checkpoint") or {}
        keep_last = int(policy.get("keepLast") or 0)
        if keep_last <= 0 or latest is None:
            return
        saved = [s for s in saved_iterations(self.path("checkpoints")) if s <= latest]
        keep = set(saved[-keep_last:])
        if policy.get("keepBest") and self.recipe.get("trainType") != "rl":
            scored = [s for s in saved if s in self.val_losses]
            if scored:
                keep.add(min(scored, key=lambda s: self.val_losses[s]))
        deleted = [s for s in saved if s not in keep]
        for step in deleted:
            for sub in ("checkpoints", "critic_checkpoints"):
                shutil.rmtree(self.path(sub, "iter_%07d" % step), ignore_errors=True)
        if deleted:
            self.emitter.notice("retention", {"deleted": deleted, "kept": sorted(keep)})

    def train_tick(self):
        """Run the periodic work of the train stage: checkpoints and samples."""
        self.checkpoint_tick()
        self.sample_tick()

    def sample_tick(self):
        """Report the sample files the sample plugin finished since the last tick, as [SAMPLE] markers."""
        try:
            names = sorted(os.listdir(self.path(SAMPLES_DIR)))
        except OSError:
            return
        limits = (self.recipe.get("metrics") or {}).get("samples") or {}
        for name in names:
            m = SAMPLE_FILE.match(name)
            if m and name not in self.reported_samples:
                self.reported_samples.add(name)
                self.report_samples(self.path(SAMPLES_DIR, name), m.group(1), int(m.group(2)), limits)

    def report_samples(self, path, kind, step, limits):
        """Print the samples of one file, truncated: the prompt's head, the response's head and tail, each line under
        32 KB and at most 1 MB a file; the rest stays in the file."""
        budget = MAX_SAMPLE_FILE
        prompt_chars = limits.get("promptChars") or 2000
        with open(path, encoding="utf-8") as f:
            for line in f:
                try:
                    r = json.loads(line)
                except ValueError:
                    continue
                prompt, response = r.get("prompt") or "", r.get("response") or ""
                reward = r.get("reward")
                fields = {"kind": kind, "step": step, "dataset": r.get("dataset") or "", "group": r.get("group", 0),
                          "index": r.get("index", 0), "prompt": truncate(prompt, prompt_chars), "label": r.get("label"),
                          "reward": reward if isinstance(reward, (int, float)) and math.isfinite(reward) else None,
                          "status": r.get("status") or "", "tokens": r.get("tokens") or 0,
                          "promptChars": len(prompt), "responseChars": len(response)}
                if r.get("rewardDetail") is not None:
                    fields["rewardDetail"] = r["rewardDetail"]
                head, tail = limits.get("responseHeadChars") or 1000, limits.get("responseTailChars") or 3000
                while True:
                    fields["response"] = truncate(response, head, tail)
                    size = len(json.dumps(fields, ensure_ascii=False, default=str))
                    if size <= MAX_SAMPLE_LINE or head + tail == 0:
                        break
                    head, tail = head // 2, tail // 2
                if size > budget:
                    return
                budget -= size
                self.emitter.sample(**fields)

    def report_disk(self):
        """Report the run directory usage in a [STAGE] marker."""
        self.emitter.notice("disk", disk_usage(self.run_dir))

    def stage_export(self):
        """Export the final model, or the checkpoint a checkpoint export names, and return the result the launcher
        reports."""
        only = self.export_only()
        iteration = only["step"] if only else self.adapter.latest_checkpoint(self.run_dir)
        output = self.adapter.export(self.recipe, self.run_dir, self.run_command, iteration=iteration,
                                     out=only["dir"] if only else None)
        missing = [n for n in ("config.json",) if not os.path.exists(os.path.join(output, n))]
        if not any(n.endswith((".safetensors", ".bin")) for n in os.listdir(output)):
            missing.append("weights")
        if not any(n.startswith("tokenizer") for n in os.listdir(output)):
            missing.append("tokenizer")
        if missing:
            raise StageError("export", "the exported model lacks %s" % ", ".join(missing), retry=False)
        result = {"stage": "export", "output": output, "iteration": iteration,
                  "lastMetrics": self.emitter.last_metrics()}
        adapter = self.adapter_dir(only)
        if adapter:
            self.adapter.export_adapter(self.recipe, self.run_dir, self.run_command, iteration, adapter, output)
            lacking = [n for n in ("adapter_model.safetensors", "adapter_config.json")
                       if not os.path.exists(os.path.join(adapter, n))]
            if lacking:
                raise StageError("export", "the PEFT adapter lacks %s" % ", ".join(lacking), retry=False)
            result["adapter"] = adapter
        self.write_result(result, os.path.join(only["dir"], "result.json") if only else None)
        self.report_disk()
        return result

    def adapter_dir(self, only):
        """Return where a LoRA run's PEFT adapter goes when one is asked for: final_adapter/, or next to the model of a
        checkpoint export; None otherwise."""
        if not self.recipe.get("lora"):
            return None
        if only:
            return only["dir"].rstrip("/") + "-adapter" if only.get("peftAdapter") else None
        return self.path("final_adapter") if (self.recipe.get("output") or {}).get("peftAdapter") else None

    def write_result(self, result, path=None):
        """Write result.json, which the verification job reads when the result marker is lost."""
        path = path or self.path("result.json")
        tmp = path + ".tmp"
        with open(tmp, "w") as f:
            json.dump(result, f)
        os.replace(tmp, path)

    def read_result(self, path=None, output=None):
        """Read the result an earlier attempt wrote: the run's result.json, or the one at path for the model in
        output."""
        try:
            with open(path or self.path("result.json")) as f:
                return json.load(f)
        except (OSError, ValueError):
            return {"stage": "export", "output": output or self.path("final_model")}

    def run_command(self, cmd, env=None, parse=False, watch=None):
        """Run a command in its own process group, forward its output and, when parse is set, turn metric lines
        into [METRIC] markers. With watch naming the stage, the watchdog ends the command when it stops making
        progress. Returns the exit code."""
        full_env = dict(os.environ)
        full_env.update(env or {})
        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env=full_env,
                                cwd=self.run_dir, start_new_session=True, text=True, errors="replace", bufsize=1)
        dog = self.watchdog if watch else None
        if dog:
            dog.watch(proc, watch, tick=self.train_tick if watch == "train" else None)
        try:
            for line in proc.stdout:
                self.emitter.forward(line)
                if ENOSPC_TEXT in line:
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
        except (ValueError, SyntaxError):
            self.emitter.parse_failures += 1
            return
        if parsed:
            kind, axis, step, values = parsed
            if self.emitter.metric(kind, axis, step, values) and self.watchdog and kind in PROGRESS_KINDS:
                self.watchdog.progress(step)
            for name, value in values.items():
                if VAL_LOSS.match(name) and isinstance(value, (int, float)) and math.isfinite(value):
                    self.val_losses[int(step)] = value
