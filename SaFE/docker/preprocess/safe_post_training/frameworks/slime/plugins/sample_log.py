#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""RL sample logging: slime's rollout and evaluation log hooks write the sampled responses of a run, untruncated, to
samples/rollout-<N>.jsonl and samples/eval-<N>.jsonl of the run directory, where the launcher reports them.

Both hooks return False, so slime still computes and prints its own metrics. The counts come from metrics.samples of
the run directory's recipe.json: rolloutGroups prompt groups per rollout, picked at random with the rollout seed and
the rollout id, and the evalPerSet lowest-index samples of every evaluation set, the same questions each time.
"""

import json
import logging
import os
import random

RECIPE_FILE = "recipe.json"
SAMPLES_DIR = "samples"

logger = logging.getLogger(__name__)


def run_dir(args):
    """Return the run directory, the parent of the checkpoint directory."""
    return os.path.dirname(os.path.normpath(args.save))


def limits(directory):
    """Return metrics.samples of the run's recipe, or {} when there is none."""
    try:
        with open(os.path.join(directory, RECIPE_FILE)) as f:
            return (json.load(f).get("metrics") or {}).get("samples") or {}
    except (OSError, ValueError):
        return {}


def text_of(prompt):
    """Return a prompt as text: a message list is written as role: content lines."""
    if isinstance(prompt, list):
        return "\n".join("%s: %s" % (m.get("role", ""), m.get("content", "")) for m in prompt if isinstance(m, dict))
    return "" if prompt is None else str(prompt)


def record(sample, dataset="", key=None):
    """Return the JSON line of one sample; a reward dict is kept as the detail, its key entry as the reward."""
    detail = getattr(sample, "reward", None)
    reward = detail.get(key) if isinstance(detail, dict) and key else detail
    status = getattr(sample, "status", "")
    out = {
        "dataset": dataset,
        "group": getattr(sample, "group_index", 0) or 0,
        "index": getattr(sample, "index", 0) or 0,
        "prompt": text_of(getattr(sample, "prompt", "")),
        "response": getattr(sample, "response", "") or "",
        "label": None if getattr(sample, "label", None) is None else str(sample.label),
        "reward": reward if isinstance(reward, (int, float)) else None,
        "status": getattr(status, "value", status) or "",
        "tokens": getattr(sample, "response_length", 0) or 0,
    }
    if isinstance(detail, dict):
        out["rewardDetail"] = detail
    return out


def write(path, records):
    """Write the records as JSON lines through a temporary file, so a reader never sees half a file."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        for r in records:
            f.write(json.dumps(r, ensure_ascii=False, default=str) + "\n")
    os.replace(tmp, path)


def log_rollout(rollout_id, args, samples, rollout_extra_metrics=None, rollout_time=None):
    """Write the samples of rolloutGroups random prompt groups of a rollout, every response of each group."""
    try:
        directory = run_dir(args)
        groups = int(limits(directory).get("rolloutGroups") or 0)
        if groups <= 0:
            return False
        by_group = {}
        for s in samples or []:
            by_group.setdefault(getattr(s, "group_index", 0) or 0, []).append(s)
        rng = random.Random("%s-%s" % (getattr(args, "rollout_seed", 0), rollout_id))
        picked = sorted(rng.sample(sorted(by_group), min(groups, len(by_group))))
        key = getattr(args, "reward_key", None)
        records = [record(s, key=key) for g in picked
                   for s in sorted(by_group[g], key=lambda s: getattr(s, "index", 0) or 0)]
        write(os.path.join(directory, SAMPLES_DIR, "rollout-%d.jsonl" % rollout_id), records)
    except Exception as e:  # noqa: BLE001 - logging samples must never stop training
        logger.warning("sample_log: rollout %s not logged: %s", rollout_id, e)
    return False


def log_eval(rollout_id, args, data, extra_metrics=None):
    """Write the evalPerSet lowest-index samples of every evaluation set."""
    try:
        directory = run_dir(args)
        per_set = int(limits(directory).get("evalPerSet") or 0)
        if per_set <= 0:
            return False
        reward_key = getattr(args, "eval_reward_key", None) or getattr(args, "reward_key", None)
        records = []
        for key in sorted(data or {}):
            samples = (data[key] or {}).get("samples") or []
            for s in sorted(samples, key=lambda s: getattr(s, "index", 0) or 0)[:per_set]:
                records.append(record(s, dataset=key, key=reward_key))
        write(os.path.join(directory, SAMPLES_DIR, "eval-%d.jsonl" % rollout_id), records)
    except Exception as e:  # noqa: BLE001 - logging samples must never stop training
        logger.warning("sample_log: evaluation %s not logged: %s", rollout_id, e)
    return False
