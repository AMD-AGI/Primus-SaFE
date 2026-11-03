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
import importlib
import json
import os
import sys
import time

if __package__ in (None, ""):
    sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from safe_post_training import metrics, stages  # noqa: E402

LAUNCHER_VERSION = "1.0.0"
NORETRY_FILE = ".noretry"
NORETRY_WAIT = 90
STATUS_FILE = "status.json"


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


def load_adapter(framework):
    """Load the framework adapter of the recipe."""
    module = importlib.import_module("safe_post_training.frameworks.%s.adapter" % framework)
    return module.Adapter()


def fail(emitter, run_dir, err, sleep=time.sleep):
    """Report a failed stage: an error that would recur is written to .noretry first and given time for the
    controller to disable failover; then the [ERROR] marker is printed last."""
    reason = {"stage": err.stage, "reason": err.reason}
    if not err.retry:
        with open(os.path.join(run_dir, NORETRY_FILE), "w") as f:
            json.dump(reason, f)
        emitter.stage(err.stage, state="failed", detail=reason, retry=False)
        sleep(NORETRY_WAIT)
    emitter.wait_after_last_metric(sleep=sleep)
    emitter.stop_heartbeat()
    emitter.error(err.stage, err.reason, retry=err.retry)
    return 1


def run(recipe, out=None, gpus=stages.ray_gpus, sleep=time.sleep):
    """Run a recipe end to end and return the exit code."""
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
        runner = stages.Runner(recipe, run_dir, load_adapter(recipe["framework"]), emitter, gpus=gpus)
        result = runner.run()
    except stages.StageError as err:
        return fail(emitter, run_dir, err, sleep=sleep)
    except Exception as err:  # noqa: BLE001 - any crash must still end with an [ERROR] marker
        stage = emitter.stage_name or "start"
        return fail(emitter, run_dir, stages.StageError(stage, "launcher error: %s" % err), sleep=sleep)
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
