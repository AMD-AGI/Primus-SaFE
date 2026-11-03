#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""slime adapter: weight conversion, the train.py command, HF export and slime's metric log lines.

slime prints each metric group as "[time] file:line - <kind> <step>: {...}" (Ray actors add a colored
"(Actor pid=..., ip=...)" prefix). The dict holds full metric names; its */step keys are dropped.
"""

import ast
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
        root = slime_dir(self.env)
        cmd = [sys.executable, os.path.join(root, "train.py")]
        cmd += model_args(root, recipe["model"]["script"]) + list(recipe["frameworkArgs"])
        return cmd, {"PYTHONPATH": python_path(root, self.env)}, recipe.get("files") or {}

    def latest_checkpoint(self, run_dir):
        """Return the iteration of the latest saved checkpoint, or None when there is none."""
        path = os.path.join(run_dir, "checkpoints", "latest_checkpointed_iteration.txt")
        try:
            with open(path) as f:
                return int(f.read().strip())
        except (OSError, ValueError):
            return None

    def export(self, recipe, run_dir, run_command):
        """Convert the latest checkpoint to HF in final_model/ and copy the tokenizer from the base model."""
        iteration = self.latest_checkpoint(run_dir)
        if iteration is None:
            raise StageError("export", "there is no checkpoint to export", retry=False)
        root, base = slime_dir(self.env), recipe["model"]["path"]
        out = os.path.join(run_dir, "final_model")
        src = os.path.join(run_dir, "checkpoints", "iter_%07d" % iteration)
        cmd = [sys.executable, os.path.join(root, "tools", "convert_torch_dist_to_hf.py"),
               "--input-dir", src, "--output-dir", out, "--origin-hf-dir", base, "-f"]
        code = run_command(cmd, env={"PYTHONPATH": python_path(root, self.env)})
        if code != 0:
            raise StageError("export", "HF export exited with code %d" % code)
        for name in _TOKENIZER_FILES:
            if os.path.exists(os.path.join(base, name)) and not os.path.exists(os.path.join(out, name)):
                shutil.copy(os.path.join(base, name), os.path.join(out, name))
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
        return kind, axis, step, values
