#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""slime adapter: weight conversion, the train.py command, HF export and slime's metric log lines.

slime prints each metric group as "[time] file:line - <kind> <step>: {...}" (Ray actors add a colored
"(Actor pid=..., ip=...)" prefix). The dict holds full metric names; its */step keys are dropped.
"""

import ast
import json
import os
import re
import shutil
import subprocess
import sys

from safe_post_training import metrics
from safe_post_training.stages import StageError

SLIME_DIRS = ("/root/slime", "/workspace/slime", "/sgl-workspace/slime", "/opt/slime")
_METRIC_LINE = re.compile(r"(critic-)?(step|rollout|perf|eval|passrate) (\d+): (\{.*\})\s*$")
_NON_FINITE = re.compile(r"(?<=[:\[,\s])(-?)(nan|inf)(?=[,}\]\s])")
_TOKENIZER_FILES = ("tokenizer.json", "tokenizer_config.json", "special_tokens_map.json", "vocab.json",
                    "merges.txt", "tokenizer.model", "chat_template.jinja", "generation_config.json")


def slime_dir(env=os.environ):
    """Return slime's checkout in the image: SLIME_DIR when set, otherwise the first known location."""
    if env.get("SLIME_DIR"):
        return env["SLIME_DIR"]
    for path in SLIME_DIRS:
        if os.path.exists(os.path.join(path, "train.py")):
            return path
    raise StageError("train", "cannot find slime's train.py; set SLIME_DIR in the image table", retry=False)


# PLUGINS holds the export tools the adapter runs; TRAINABLE_ONLY_MARKER is the file slime writes into a checkpoint
# that holds the trainable parameters only.
PLUGINS = os.path.join(os.path.dirname(os.path.abspath(__file__)), "plugins")
TRAINABLE_ONLY_MARKER = "trainable_only"


def lora_args(lora):
    """Return the export tools' LoRA arguments of a recipe's lora section."""
    return ["--rank", str(lora["rank"]), "--alpha", str(lora["alpha"]), "--dropout", str(lora.get("dropout") or 0)]


def critic_config(critic, resume):
    """Return the megatron.yaml of a PPO critic, written as JSON, which YAML reads. slime does not fall back to the
    base model for a role's own load directory, so load is left out until the critic has a checkpoint."""
    overrides = dict(critic.get("overrides") or {})
    if not resume:
        overrides.pop("load", None)
    return json.dumps({"megatron": [{"name": "critic", "role": "critic", "overrides": overrides}]}, indent=2) + "\n"


def python_path(root, env=os.environ):
    """Return the PYTHONPATH for slime and, when MEGATRON_DIR is set, Megatron-LM."""
    parts = [root]
    if env.get("MEGATRON_DIR"):
        parts.append(env["MEGATRON_DIR"])
    if env.get("PYTHONPATH"):
        parts.append(env["PYTHONPATH"])
    return os.pathsep.join(parts)


def model_args(root, script):
    """Return the MODEL_ARGS array the spec's model script defines."""
    path = os.path.join(root, script)
    if not os.path.exists(path):
        raise StageError("train", "model script %s does not exist in the image" % script, retry=False)
    out = subprocess.run(["bash", "-c", 'source "$0" >/dev/null && printf "%s\\n" "${MODEL_ARGS[@]}"', path],
                         capture_output=True, text=True, cwd=root, timeout=60)
    if out.returncode != 0:
        raise StageError("train", "model script %s failed: %s" % (script, out.stderr.strip()[-500:]), retry=False)
    return [a for a in out.stdout.splitlines() if a]


class Adapter:
    """The slime implementation of the launcher's framework adapter."""

    def __init__(self, env=os.environ):
        self.env = env
        self.train_type = ""
        self.seen_eval_zero = False

    def convert(self, recipe, run_dir, run_command):
        """Convert the base HF weights to torch_dist in the recipe's convert directory on one node."""
        conv = recipe["convert"]
        if conv.get("nodes", 1) > 1:
            raise StageError("convert", "a %d-node conversion runs in its own convert workload" % conv["nodes"])
        root = slime_dir(self.env)
        tmp = conv["dir"] + ".partial"
        shutil.rmtree(tmp, ignore_errors=True)
        cmd = ["torchrun", "--standalone", "--nproc-per-node", str(recipe["layout"]["gpusPerNode"]),
               os.path.join(root, "tools", "convert_hf_to_torch_dist.py")]
        cmd += model_args(root, recipe["model"]["script"])
        cmd += ["--hf-checkpoint", recipe["model"]["path"], "--save", tmp]
        code = run_command(cmd, env={"PYTHONPATH": python_path(root, self.env)})
        if code != 0:
            shutil.rmtree(tmp, ignore_errors=True)
            raise StageError("convert", "weight conversion exited with code %d" % code)
        shutil.rmtree(conv["dir"], ignore_errors=True)
        os.makedirs(os.path.dirname(conv["dir"]), exist_ok=True)
        os.replace(tmp, conv["dir"])

    def train_command(self, recipe, run_dir):
        """Return the train.py command, its environment and the files to write before it starts."""
        self.train_type = recipe.get("trainType", "")
        root = slime_dir(self.env)
        cmd = [sys.executable, os.path.join(root, "train.py")]
        cmd += model_args(root, recipe["model"]["script"]) + list(recipe["frameworkArgs"])
        files = dict(recipe.get("files") or {})
        critic = recipe.get("critic")
        if critic:
            resume = self.latest_checkpoint(run_dir, critic.get("checkpoints") or "critic_checkpoints") is not None
            files[critic["megatronConfig"]] = critic_config(critic, resume)
        return cmd, {"PYTHONPATH": python_path(root, self.env)}, files

    def latest_checkpoint(self, run_dir, sub="checkpoints"):
        """Return the iteration of the latest checkpoint saved in sub, or None when there is none."""
        path = os.path.join(run_dir, sub, "latest_checkpointed_iteration.txt")
        try:
            with open(path) as f:
                return int(f.read().strip())
        except (OSError, ValueError):
            return None

    def export(self, recipe, run_dir, run_command, iteration=None, out=None):
        """Convert a checkpoint, the latest unless iteration names one, to HF in out (final_model/ by default) and
        copy the tokenizer from the base model. A LoRA checkpoint is exported with its adapters merged."""
        if iteration is None:
            iteration = self.latest_checkpoint(run_dir)
        if iteration is None:
            raise StageError("export", "there is no checkpoint to export", retry=False)
        root, base = slime_dir(self.env), recipe["model"]["path"]
        out = out or os.path.join(run_dir, "final_model")
        src = os.path.join(run_dir, "checkpoints", "iter_%07d" % iteration)
        if not os.path.isdir(src):
            raise StageError("export", "the checkpoint of iteration %d no longer exists" % iteration, retry=False)
        lora, scratch = recipe.get("lora"), None
        if lora and os.path.exists(os.path.join(src, TRAINABLE_ONLY_MARKER)):
            # The checkpoint holds the adapters only: they are merged into the base model on the HF side.
            scratch = out.rstrip("/") + ".adapter-tmp"
            cmd = [sys.executable, os.path.join(PLUGINS, "export_peft.py"), "--slime-dir", root, "--input-dir", src,
                   "--origin-hf-dir", base, "--output-dir", scratch, "--merge-into", out] + lora_args(lora)
        elif lora:
            cmd = [sys.executable, os.path.join(PLUGINS, "merge_lora.py"), "--slime-dir", root, "--input-dir", src,
                   "--output-dir", out, "--origin-hf-dir", base, "--rank", str(lora["rank"]),
                   "--alpha", str(lora["alpha"])]
        else:
            cmd = [sys.executable, os.path.join(root, "tools", "convert_torch_dist_to_hf.py"),
                   "--input-dir", src, "--output-dir", out, "--origin-hf-dir", base, "-f"]
        code = run_command(cmd, env={"PYTHONPATH": python_path(root, self.env)})
        if scratch:
            shutil.rmtree(scratch, ignore_errors=True)
        if code != 0:
            raise StageError("export", "HF export exited with code %d" % code)
        os.makedirs(out, exist_ok=True)
        for name in _TOKENIZER_FILES:
            if os.path.exists(os.path.join(base, name)) and not os.path.exists(os.path.join(out, name)):
                shutil.copy(os.path.join(base, name), os.path.join(out, name))
        return out

    def export_adapter(self, recipe, run_dir, run_command, iteration, out, merged):
        """Write the HF PEFT adapter of a LoRA checkpoint to out and check it against the merged model."""
        root, base = slime_dir(self.env), recipe["model"]["path"]
        src = os.path.join(run_dir, "checkpoints", "iter_%07d" % iteration)
        cmd = [sys.executable, os.path.join(PLUGINS, "export_peft.py"), "--slime-dir", root, "--input-dir", src,
               "--origin-hf-dir", base, "--output-dir", out, "--verify-merged", merged] + lora_args(recipe["lora"])
        code = run_command(cmd, env={"PYTHONPATH": python_path(root, self.env)})
        if code != 0:
            raise StageError("export", "PEFT adapter export exited with code %d" % code)
        return out

    def parse_metric_line(self, line):
        """Parse one slime metric line into (kind, axis, step, values), or None when the line is not one."""
        m = _METRIC_LINE.search(metrics.strip_prefixes(line).rstrip("\n"))
        if not m:
            return None
        kind, step = m.group(2), int(m.group(3))
        data = ast.literal_eval(_NON_FINITE.sub(lambda x: "'%s%s'" % (x.group(1), x.group(2)), m.group(4)))
        if not isinstance(data, dict):
            raise ValueError("metric line holds no dict")
        values = {}
        for name, value in data.items():
            if not isinstance(name, str) or name.endswith("/step"):
                continue
            if isinstance(value, str) and value.lstrip("-") in ("nan", "inf"):
                value = float(value)
            if isinstance(value, bool) or not isinstance(value, (int, float)):
                continue
            values[name] = float(value)
        axis = "train" if kind == "step" else "rollout"
        if kind == "eval" and self.train_type == "sft":
            # SFT validation runs before training as eval 0 and after rollout N as eval N, i.e. after N+1 steps.
            axis = "train"
            if step == 0 and not self.seen_eval_zero:
                self.seen_eval_zero = True
            else:
                step += 1
        return kind, axis, step, values
