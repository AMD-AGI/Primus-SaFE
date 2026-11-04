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
             ("finalModel", "final_model"), ("finalAdapter", "final_adapter"))
PROGRESS_KINDS = ("step", "rollout", "eval")
ENOSPC_TEXT = "No space left on device"
CONVERT_LOCK = ".lock"
SOURCE_FILE = "source.json"
# A conversion lock older than this was left by a conversion that died; the next run takes it over.
LOCK_STALE_SECONDS = 6 * 3600
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


class Runner:
    """Runs the stages of one recipe."""

    def __init__(self, recipe, run_dir, adapter, emitter, gpus=ray_gpus, watchdog=None, sleep=time.sleep,
                 sampler=None):
        self.recipe, self.run_dir, self.adapter, self.emitter, self.gpus = recipe, run_dir, adapter, emitter, gpus
        self.watchdog, self.sleep = watchdog, sleep
        # sampler, when set, returns the GPU sampler that runs alongside training.
        self.sampler = sampler
        self.enospc = False
        self.reward_rejected = False
        self.checkpoint = None
        self.wandb_link = None

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

    def report_conversion(self, state):
        """Print the convert [STAGE] that registers a shared conversion with the controller."""
        out, root, _ = self.convert_paths()
        detail = None
        if (self.recipe.get("convert") or {}).get("cacheKey"):
            detail = {"cacheKey": self.recipe["convert"]["cacheKey"], "dir": root, "sizeBytes": dir_size(out)}
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

    def reuse_conversion(self):
        """Report a complete conversion of the model's current weights as skipped; returns whether there is one."""
        _, root, complete = self.convert_paths()
        if not os.path.exists(complete):
            return False
        self.check_source(root)
        self.report_conversion("skipped")
        return True

    def run(self):
        """Run every stage that has not finished yet and return the export result."""
        result = None
        for stage in STAGES:
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
        converted them, and record the source fingerprint before the completion mark."""
        _, root, complete = self.convert_paths()
        with cache_lock(root, sleep=self.sleep):
            if not os.path.exists(complete):
                self.adapter.convert(self.recipe, self.run_dir, functools.partial(self.run_command, watch="convert"))
                source = {"model": self.recipe["model"]["path"],
                          "fingerprint": weights_fingerprint(self.recipe["model"]["path"])}
                with open(os.path.join(root, SOURCE_FILE), "w") as f:
                    json.dump(source, f)
                open(complete, "w").close()
        self.check_source(root)
        self.report_conversion("done")

    def stage_check(self):
        """Check the GPU count, the inputs and the free space before training."""
        layout = self.recipe["layout"]
        want = layout["nodes"] * layout["gpusPerNode"]
        have = self.gpus()
        if have < want:
            raise StageError("check", "the Ray cluster has %d GPUs, the recipe needs %d" % (have, want))
        model = self.recipe["model"]["path"]
        for path in (os.path.join(model, "config.json"), self.path("data", "train.jsonl")):
            if not os.path.exists(path):
                raise StageError("check", "missing input %s" % path)
        need = self.space_needed()
        free = shutil.disk_usage(self.run_dir).free
        if free < need:
            raise StageError("check", "shared storage has %d GiB free, the run needs about %d GiB"
                             % (free >> 30, need >> 30), retry=False)
        self.check_wandb()
        self.check_reward()

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
        """Estimate the bytes the run writes: the checkpoints kept at the same time plus the exported model."""
        weights = weights_size(self.recipe["model"]["path"])
        keep = (self.recipe.get("checkpoint") or {}).get("keepLast", 0)
        count = keep + 2 if keep > 0 else 3
        return weights * CHECKPOINT_FACTOR * count + weights

    def stage_train(self):
        """Run the training command, resuming from the latest checkpoint when there is one."""
        cmd, env, files = self.adapter.train_command(self.recipe, self.run_dir)
        for rel, content in (files or {}).items():
            target = self.path(rel)
            os.makedirs(os.path.dirname(target), exist_ok=True)
            with open(target, "w") as f:
                f.write(content)
        resume = self.checkpoint = self.adapter.latest_checkpoint(self.run_dir)
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
            self.report_disk()

    def report_disk(self):
        """Report the run directory usage in a [STAGE] marker."""
        self.emitter.notice("disk", disk_usage(self.run_dir))

    def stage_export(self):
        """Export the final model and return the result the launcher reports."""
        output = self.adapter.export(self.recipe, self.run_dir, self.run_command)
        missing = [n for n in ("config.json",) if not os.path.exists(os.path.join(output, n))]
        if not any(n.endswith((".safetensors", ".bin")) for n in os.listdir(output)):
            missing.append("weights")
        if not any(n.startswith("tokenizer") for n in os.listdir(output)):
            missing.append("tokenizer")
        if missing:
            raise StageError("export", "the exported model lacks %s" % ", ".join(missing), retry=False)
        result = {"stage": "export", "output": output, "iteration": self.adapter.latest_checkpoint(self.run_dir),
                  "lastMetrics": self.emitter.last_metrics()}
        self.write_result(result)
        self.report_disk()
        return result

    def write_result(self, result):
        """Write result.json, which the verification job reads when the result marker is lost."""
        tmp = self.path("result.json.tmp")
        with open(tmp, "w") as f:
            json.dump(result, f)
        os.replace(tmp, self.path("result.json"))

    def read_result(self):
        """Read the result an earlier attempt wrote."""
        try:
            with open(self.path("result.json")) as f:
                return json.load(f)
        except (OSError, ValueError):
            return {"stage": "export", "output": self.path("final_model")}

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
            dog.watch(proc, watch, tick=self.checkpoint_tick if watch == "train" else None)
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
