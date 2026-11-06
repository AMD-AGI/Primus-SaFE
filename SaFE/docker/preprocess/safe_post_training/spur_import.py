#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""safe-post-training-import: parse a Spur run directory into a summary and upload it to SaFE.

The slime-rocm job scripts write one directory per run: the verdict (PASS or FAIL), one node log per host with the
BEGIN and END line of every gate, results/, markers/, mon/ resource samples, logs/ with the main log of the training
gate on the head node, and optionally ckpt/ and rollout/. The command reads the directory only, line by line, and
needs nothing beyond the Python 3 standard library, so safe_post_training/ can be copied to a Spur login node and run
there. The upload token comes from SAFE_IMPORT_TOKEN, or a prompt that does not echo it, never from the command line,
which other users of a shared login node can read.
"""
import argparse
import base64
import datetime
import getpass
import gzip
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request

if __package__ in (None, ""):
    sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from safe_post_training import metrics  # noqa: E402
from safe_post_training.frameworks.slime.adapter import Adapter  # noqa: E402

SCHEMA = "slime-rocm-run/1"
PARSER_VERSION = "1"
FORMAT = "slime-rocm"
# TERMINATION_LOG is where Kubernetes reads a container's termination message from.
TERMINATION_LOG = "/dev/termination-log"
TOKEN_ENV = "SAFE_IMPORT_TOKEN"
MAX_SUMMARY = 16 << 20
MAX_CHECK_LINES = 400
MAX_LINE = 1000

# STAGES orders the stages gates are grouped into; stage_of names the stage of a gate.
STAGES = (("preflight", "Preflight"), ("ray", "Ray"), ("dryrun", "Dry run"), ("train", "Train"),
          ("verdict", "Verdict"))
PREFLIGHT_GATES = ("a1_host", "a1_nics", "a1_overlay", "a1_imports")

# KEPT_ARGS are the arguments the summary keeps: base model, data, parallelism, batch, learning rate, loss, saving,
# and for RL the rollout and the custom functions. The others are only counted.
KEPT_ARGS = (
    "load", "ref_load", "hf_checkpoint", "prompt_data", "input_key", "eval_prompt_data",
    "tensor_model_parallel_size", "pipeline_model_parallel_size", "context_parallel_size",
    "expert_model_parallel_size", "actor_num_nodes", "actor_num_gpus_per_node", "rollout_num_gpus",
    "rollout_num_gpus_per_engine", "global_batch_size", "rollout_batch_size", "n_samples_per_prompt",
    "num_rollout", "num_epoch", "max_tokens_per_gpu", "lr", "min_lr", "lr_decay_style", "loss_type",
    "advantage_estimator", "use_kl_loss", "kl_loss_coef", "save", "save_interval", "rm_type", "custom_rm_path",
    "rollout_function_path",
)
# SECRET_WORDS mark an argument name as a credential's; for key and token the value must also look like one, since
# slime and Megatron name many fields that way (input_key, max_tokens_per_gpu).
SECRET_WORDS = {"key", "token", "secret", "password", "passwd", "apikey", "credential", "credentials"}
ALWAYS_SECRET = {"secret", "password", "passwd", "credential", "credentials"}
_CREDENTIAL = re.compile(r"^[A-Za-z0-9_.=+:-]{16,}$")

_NODE_HEAD = re.compile(r"^\[(?P<ts>[^\]]+)\] \[(?P<host>[^\] ]+)\] rank=(?P<rank>\d+)/(?P<size>\d+) ip=(?P<ip>\S+)"
                        r" head=(?P<head>[^(\s]+)\S* image=(?P<image>\S+)")
_NODE_GATE = re.compile(r"^\[(?P<ts>[^\]]+)\] \[(?P<host>\S+) r\d+\] (?P<what>BEGIN|END) (?P<gate>\S+)"
                        r"(?: (?P<res>\S+))?")
_ARG = re.compile(r"^\s+(\w+) \.{2,} (.*)$")
_LOG_TIME = re.compile(r"\[(\d{4}-\d\d-\d\d) (\d\d:\d\d:\d\d)\]")
_PLAIN = re.compile(r"^(?!\x1b|\(|\[\d{4}-)")


class RunDirError(Exception):
    """A run directory that cannot be imported, with the reason shown to the user."""


def stage_of(gate):
    """Return the stage a gate belongs to, or None for a gate the parser does not know."""
    if gate in PREFLIGHT_GATES:
        return "preflight"
    if gate == "a1_ray":
        return "ray"
    if gate.endswith(("_pool", "_dryrun")):
        return "dryrun"
    if gate.startswith("b"):
        return "train"
    return None


def utc(text):
    """Parse an ISO UTC time such as 2026-10-21T10:13:43Z into epoch seconds, or None."""
    try:
        return int(datetime.datetime.strptime(text, "%Y-%m-%dT%H:%M:%SZ").replace(
            tzinfo=datetime.timezone.utc).timestamp())
    except (TypeError, ValueError):
        return None


def iso(seconds):
    """Format epoch seconds as an ISO UTC time, or None."""
    if seconds is None:
        return None
    return datetime.datetime.fromtimestamp(seconds, datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def mtime(path):
    """Return the modification time of a path in epoch seconds, or None when it does not exist."""
    try:
        return int(os.stat(path).st_mtime)
    except OSError:
        return None


def read_rows(path):
    """Read the tab-separated gate, node, result, seconds rows of a results file."""
    rows = []
    try:
        with open(path, errors="replace") as f:
            for line in f:
                parts = line.rstrip("\n").split("\t")
                if len(parts) == 4:
                    seconds = int(parts[3]) if parts[3].isdigit() else None
                    rows.append({"gate": parts[0], "node": parts[1], "result": parts[2], "seconds": seconds})
    except OSError:
        pass
    return rows


def read_verdict(root):
    """Return the verdict, PASS, FAIL or "", and the failed items of a FAIL file."""
    if os.path.exists(os.path.join(root, "PASS")):
        return "PASS", []
    path = os.path.join(root, "FAIL")
    if not os.path.exists(path):
        return "", []
    return "FAIL", read_rows(path)


def read_results(root):
    """Read results/results.tsv, or merge the per-node results files when it is missing."""
    path = os.path.join(root, "results", "results.tsv")
    if os.path.exists(path):
        return read_rows(path)
    rows = []
    for name in sorted(os.listdir(os.path.join(root, "results")) if os.path.isdir(os.path.join(root, "results"))
                       else []):
        if name.startswith("results-") and name.endswith(".tsv"):
            rows.extend(read_rows(os.path.join(root, "results", name)))
    return rows


def read_nodes(root):
    """Read the node logs: each host with its rank, ip, image and head, and the BEGIN and END time of every gate."""
    nodes, gates = [], {}
    for name in sorted(os.listdir(root)):
        if not (name.startswith("node-") and name.endswith(".log")):
            continue
        node = {"host": name[len("node-"):-len(".log")]}
        with open(os.path.join(root, name), errors="replace") as f:
            for line in f:
                m = _NODE_HEAD.match(line)
                if m and "rank" not in node:
                    node.update(host=m.group("host"), rank=int(m.group("rank")), ip=m.group("ip"),
                                image=m.group("image"), headHost=m.group("head"))
                    continue
                m = _NODE_GATE.match(line)
                if m:
                    g = gates.setdefault((m.group("gate"), m.group("host")), {})
                    key = "start" if m.group("what") == "BEGIN" else "end"
                    g[key] = utc(m.group("ts"))
                    if key == "end":
                        g["result"] = m.group("res") or ""
        nodes.append(node)
    nodes.sort(key=lambda n: n.get("rank", 1 << 30))
    head = next((n.get("headHost") for n in nodes if n.get("headHost")), nodes[0]["host"] if nodes else "")
    for n in nodes:
        n["head"] = n["host"] == head
        n.pop("headHost", None)
    return nodes, head, gates


def read_markers(root):
    """List the marker files with their modification times; the files are empty."""
    path = os.path.join(root, "markers")
    if not os.path.isdir(path):
        return []
    out = [{"name": n, "time": iso(mtime(os.path.join(path, n)))} for n in os.listdir(path)]
    return sorted(out, key=lambda m: (m["time"] or "", m["name"]))


def read_mon(root):
    """Read mon/<host>.tsv: host memory in use (total - available) and the largest per-GPU VRAM, with peaks."""
    out = {}
    path = os.path.join(root, "mon")
    if not os.path.isdir(path):
        return out
    for name in sorted(os.listdir(path)):
        if not name.endswith(".tsv"):
            continue
        samples, mem_peak, vram_peak = [], 0, 0
        with open(os.path.join(path, name), errors="replace") as f:
            for line in f:
                parts = line.rstrip("\n").split("\t")
                if len(parts) < 5 or not parts[1].isdigit():
                    continue
                ts, total, avail, vram = utc(parts[0]), int(parts[1]), int(parts[2]), int(parts[3])
                used = max(0, total - avail)
                samples.append([ts, used, vram])
                mem_peak, vram_peak = max(mem_peak, used), max(vram_peak, vram)
        out[name[:-len(".tsv")]] = {"samples": samples, "hostMemPeakKb": mem_peak, "vramPeakBytes": vram_peak}
    return out


def dir_size(path):
    """Return the number of files under a directory and their total size."""
    files, size = 0, 0
    for base, _, names in os.walk(path):
        for n in names:
            try:
                size += os.stat(os.path.join(base, n)).st_size
                files += 1
            except OSError:
                pass
    return files, size


def read_checkpoints(root, steps_per_rollout):
    """List ckpt/: the latest iteration and each iter_* with its files, size, time and trained steps; and rollout/."""
    out = {"latest": None, "iterations": [], "rollout": None}
    ckpt = os.path.join(root, "ckpt")
    if os.path.isdir(ckpt):
        try:
            with open(os.path.join(ckpt, "latest_checkpointed_iteration.txt")) as f:
                out["latest"] = int(f.read().strip())
        except (OSError, ValueError):
            pass
        for name in sorted(os.listdir(ckpt)):
            m = re.fullmatch(r"iter_(\d+)", name)
            if m:
                files, size = dir_size(os.path.join(ckpt, name))
                it = int(m.group(1))
                out["iterations"].append({"iteration": it, "step": (it + 1) * steps_per_rollout, "files": files,
                                          "bytes": size, "time": iso(mtime(os.path.join(ckpt, name)))})
    rollout = os.path.join(root, "rollout")
    if os.path.isdir(rollout):
        files, size = dir_size(rollout)
        out["rollout"] = {"files": files, "bytes": size}
    return out


def parse_value(text):
    """Turn an argument value printed by slime into a JSON value where it reads as one."""
    text = text.strip()
    if text in ("None", ""):
        return None
    if text in ("True", "False"):
        return text == "True"
    for kind in (int, float):
        try:
            return kind(text)
        except ValueError:
            pass
    return text


def read_main_log(path, adapter):
    """Read the main log line by line: the arguments section, the metric points with their wall times, and the
    check output printed without a Ray prefix at its end."""
    args, in_args, check = {}, False, []
    series = {}
    errors = lines = 0
    with open(path, errors="replace") as f:
        for line in f:
            if "--- arguments ---" in line:
                in_args = True
                continue
            if in_args:
                if "--- end of arguments ---" in line:
                    in_args = False
                    adapter.train_type = "sft" if args.get("loss_type") == "sft_loss" else "rl"
                    continue
                m = _ARG.match(line)
                if m:
                    args[m.group(1)] = parse_value(m.group(2))
                continue
            if ": {" in line:
                stripped = metrics.strip_prefixes(line)
                try:
                    parsed = adapter.parse_metric_line(line)
                except (ValueError, SyntaxError):
                    parsed, errors = None, errors + 1
                if parsed:
                    kind, axis, step, values = parsed
                    lines += kind == "step"
                    t = _LOG_TIME.search(stripped)
                    wall = utc("%sT%sZ" % t.groups()) if t else None
                    for name, value in values.items():
                        s = series.setdefault(name, {"axis": axis, "points": {}})
                        s["points"][step] = [step, value, wall]
                    check = []
                    continue
            if _PLAIN.match(line) and line.strip():
                if len(check) < MAX_CHECK_LINES:
                    check.append(line.rstrip("\n")[:MAX_LINE])
            else:
                check = []
    return args, series, check, errors, lines


def is_secret(name, value):
    """Tell whether an argument holds a credential: its name has a secret word and, for a key or a token, its value
    looks like a credential rather than a field name or a path."""
    words = set(name.lower().split("_"))
    if value is None or not words & SECRET_WORDS:
        return False
    if words & ALWAYS_SECRET:
        return True
    return isinstance(value, str) and bool(_CREDENTIAL.match(value))


def kept_args(args):
    """Keep the key arguments and every argument holding a credential, the credentials masked as ***; count all."""
    kept = {}
    for name, value in args.items():
        if is_secret(name, value):
            kept[name] = "***"
        elif name in KEPT_ARGS:
            kept[name] = value
    return {"kept": kept, "total": len(args)}


def stages_of(gates, head, verdict, verdict_time, warnings):
    """Group the head node's gates into stages with their times, results and gates, unknown gates on their own."""
    by_stage, order = {}, []
    for (gate, host), g in sorted(gates.items(), key=lambda kv: kv[1].get("start") or 0):
        if host != head:
            continue
        stage = stage_of(gate)
        if stage is None:
            stage = gate
            warnings.append("Gate %s is not a known gate; it is shown as its own stage." % gate)
        if stage not in by_stage:
            by_stage[stage] = {"stage": stage, "label": dict(STAGES).get(stage, gate), "gates": [], "start": None,
                               "end": None, "result": "pass"}
            order.append(stage)
        s = by_stage[stage]
        s["gates"].append(gate)
        if g.get("start") is not None and (s["start"] is None or g["start"] < s["start"]):
            s["start"] = g["start"]
        if g.get("end") is not None and (s["end"] is None or g["end"] > s["end"]):
            s["end"] = g["end"]
        if g.get("end") is None:
            s["result"] = "fail" if s["result"] == "fail" else "running"
        elif g.get("result", "pass") != "pass":
            s["result"] = "fail"
    known = [k for k, _ in STAGES]
    order.sort(key=lambda k: known.index(k) if k in known else len(known) - 1)
    out = [by_stage[k] for k in order]
    if verdict:
        out.append({"stage": "verdict", "label": "Verdict", "gates": [], "start": verdict_time, "end": verdict_time,
                    "result": "pass" if verdict == "PASS" else "fail"})
    for s in out:
        s["start"], s["end"] = iso(s["start"]), iso(s["end"])
    return out


def gate_rows(results, gates, warnings):
    """List every gate on every node with its result, time and seconds; results.tsv wins over the node logs, and
    a disagreement is reported."""
    rows = []
    seen = set()
    for r in results:
        g = gates.get((r["gate"], r["node"]), {})
        if g.get("result") and g["result"] != r["result"]:
            warnings.append("%s on %s: results.tsv says %s but the node log says %s." % (
                r["gate"], r["node"], r["result"], g["result"]))
        rows.append(dict(r, start=iso(g.get("start")), end=iso(g.get("end"))))
        seen.add((r["gate"], r["node"]))
    for (gate, host), g in gates.items():
        if (gate, host) not in seen:
            seconds = g["end"] - g["start"] if g.get("end") and g.get("start") else None
            rows.append({"gate": gate, "node": host, "result": g.get("result") or "running", "seconds": seconds,
                         "start": iso(g.get("start")), "end": iso(g.get("end"))})
    return sorted(rows, key=lambda r: (r["start"] or "", r["node"]))


def final_eval(check, results):
    """Return the final evaluation a check printed as one JSON line, with the result of its gate, or None."""
    for line in reversed(check):
        text = line.strip()
        if text.startswith("{") and text.endswith("}"):
            try:
                data = json.loads(text)
            except ValueError:
                continue
            gate = data.get("gate") if isinstance(data.get("gate"), str) else ""
            result = next((r["result"] for r in results if r["gate"] == gate), "")
            return {"gate": gate, "result": result, "data": data}
    return None


def steps_per_rollout(args, train_type):
    """Return how many training steps one rollout makes."""
    if train_type == "sft":
        return 1
    try:
        return max(1, int(args["rollout_batch_size"]) * int(args.get("n_samples_per_prompt") or 1)
                   // int(args["global_batch_size"]))
    except (KeyError, TypeError, ValueError, ZeroDivisionError):
        return 1


def parse_slime_rocm(root):
    """Parse a slime-rocm run directory into a summary (schema slime-rocm-run/1)."""
    if not os.path.isdir(root):
        raise RunDirError("%s is not a directory" % root)
    warnings = []
    verdict, failed = read_verdict(root)
    results = read_results(root)
    nodes, head, gates = read_nodes(root)
    if not nodes:
        raise RunDirError("%s has no node-*.log, so it is not a slime-rocm run directory" % root)
    train_gate = next((g for (g, h) in sorted(gates, key=lambda k: gates[k].get("start") or 0)
                       if h == head and stage_of(g) == "train"), "")
    main = os.path.join(root, "logs", "%s-%s.log" % (train_gate, head)) if train_gate else ""
    adapter = Adapter(env={})
    adapter.train_type = "sft"
    args, series, check, parse_errors, metric_lines = {}, {}, [], 0, 0
    if main and os.path.exists(main):
        args, series, check, parse_errors, metric_lines = read_main_log(main, adapter)
    else:
        warnings.append("No main log of a training gate on the head node %s." % head)
    if parse_errors:
        warnings.append("%d metric lines could not be parsed." % parse_errors)
    train_type = "sft" if args.get("loss_type") == "sft_loss" else ("rl" if args else "sft")
    spr = steps_per_rollout(args, train_type)
    markers = read_markers(root)
    verdict_file = os.path.join(root, "PASS" if verdict == "PASS" else "FAIL")
    starts = [g["start"] for g in gates.values() if g.get("start")]
    ends = [g["end"] for g in gates.values() if g.get("end")]
    start = min(starts) if starts else None
    end = max(ends) if ends else None
    fail_line = next((line.strip() for line in check if line.strip().startswith("FAIL")), "")
    if verdict == "PASS":
        status, failed_stage, message = "Succeeded", "", ""
    elif verdict == "FAIL":
        first = failed[0] if failed else {}
        status, failed_stage = "Failed", stage_of(first.get("gate", "")) or first.get("gate", "")
        message = fail_line or ("%s failed on %s: %s" % (first.get("gate"), first.get("node"), first.get("result"))
                                if first else "The run directory has a FAIL file")
    else:
        status, failed_stage = "Stopped", ""
        last_marker = markers[-1] if markers else None
        log_time = mtime(main) if main else None
        end = log_time or end
        message = "No verdict in the run directory; the last marker is %s and the main log was last written at %s." % (
            ("%s at %s" % (last_marker["name"], last_marker["time"])) if last_marker else "none", iso(log_time))
    trainable = next((line.split(":", 1)[1].strip() for line in check if line.startswith("trainable:")), "")
    load = args.get("load") or args.get("hf_checkpoint") or ""
    metrics_out = {}
    for name, s in series.items():
        points = [s["points"][k] for k in sorted(s["points"])]
        metrics_out[name] = {"axis": s["axis"], "count": len(points), "downsampled": False, "points": points}
    m = re.search(r"-(\d+)(?:\.[^/]*)?$", os.path.basename(root))
    summary = {
        "schema": SCHEMA, "parserVersion": PARSER_VERSION, "format": FORMAT, "path": root,
        "name": os.path.basename(root), "slurmJobId": m.group(1) if m else "",
        "verdict": verdict, "status": status, "failedStage": failed_stage, "message": message,
        "trainType": train_type, "algorithm": args.get("advantage_estimator") if train_type == "rl" else "",
        "method": "lora" if "lora" in str(args.get("custom_model_provider_path") or "") else "full",
        "startTime": iso(start), "endTime": iso(end),
        "nodes": nodes, "head": head, "image": next((n.get("image") for n in nodes if n.get("image")), ""),
        "gpusPerNode": args.get("actor_num_gpus_per_node") or 8,
        "baseModel": os.path.basename(str(load).rstrip("/")), "dataset": args.get("prompt_data") or "",
        "stepsPerRollout": spr, "saveSet": args.get("save") is not None,
        "mainLog": {"name": os.path.basename(main), "bytes": os.path.getsize(main), "metricLines": metric_lines}
        if main and os.path.exists(main) else None,
        "stages": stages_of(gates, head, verdict, mtime(verdict_file), warnings),
        "gates": gate_rows(results, gates, warnings),
        "markers": markers, "check": check, "failLine": fail_line, "trainable": trainable,
        "finalEval": final_eval(check, results),
        "arguments": kept_args(args), "resources": read_mon(root),
        "checkpoints": read_checkpoints(root, spr), "metrics": metrics_out, "warnings": warnings,
    }
    return summary


FORMATS = {FORMAT: parse_slime_rocm}


def downsample(points, buckets):
    """Keep the lowest and highest point of each of buckets equal step ranges, in step order."""
    if len(points) <= buckets * 2:
        return points
    lo, hi = points[0][0], points[-1][0]
    width = (hi - lo) / float(buckets) or 1.0
    groups = {}
    for p in points:
        groups.setdefault(min(buckets - 1, int((p[0] - lo) / width)), []).append(p)
    out = []
    for k in sorted(groups):
        g = groups[k]
        low, high = min(g, key=lambda p: p[1]), max(g, key=lambda p: p[1])
        out.extend(sorted({id(low): low, id(high): high}.values(), key=lambda p: p[0]))
    return out


def encode(summary, limit=None):
    """Serialize a summary, downsampling its longest series until it fits under limit bytes."""
    limit = limit or MAX_SUMMARY
    data = json.dumps(summary, separators=(",", ":")).encode()
    max_points = 1000
    while len(data) > limit and max_points >= 2:
        for s in summary["metrics"].values():
            if len(s["points"]) > max_points:
                s["points"] = downsample(s["points"], max_points // 2)
                s["downsampled"] = True
        data = json.dumps(summary, separators=(",", ":")).encode()
        max_points //= 2
    if len(data) > limit:
        raise RunDirError("the summary is %d bytes even after downsampling, above the limit of %d" % (len(data),
                                                                                                     limit))
    return data


def token_address(token):
    """Return the http(s) SaFE address a token names: sst_<base64url of the address>.<secret>."""
    if not token.startswith("sst_") or "." not in token:
        raise RunDirError("the token is not a SaFE import token")
    head = token[len("sst_"):].split(".", 1)[0]
    try:
        address = base64.urlsafe_b64decode(head + "=" * (-len(head) % 4)).decode()
    except (ValueError, UnicodeDecodeError):
        raise RunDirError("the token is not a SaFE import token")
    try:
        parts = urllib.parse.urlsplit(address)
        valid = parts.scheme in ("http", "https") and bool(parts.hostname)
    except ValueError:
        valid = False
    if not valid:
        raise RunDirError("the token names %r, which is not an http or https address" % address)
    return address


def read_token():
    """Read the token from SAFE_IMPORT_TOKEN, or prompt for it without echo."""
    token = os.environ.get(TOKEN_ENV, "").strip()
    if not token and sys.stdin.isatty():
        token = getpass.getpass("SaFE import token: ").strip()
    if not token:
        raise RunDirError("set %s to the token the import dialog gave" % TOKEN_ENV)
    return token


def upload(data, token, stage=False, opener=urllib.request.urlopen):
    """POST a gzip-compressed summary to SaFE with the token; return the HTTP status and the JSON answer."""
    address = token_address(token)
    if urllib.parse.urlsplit(address).scheme == "http":
        print("warning: %s is plain http, so the token and the summary travel unencrypted" % address, file=sys.stderr)
    url = address.rstrip("/") + "/api/v1/post-training/imports" + ("?stage=true" if stage else "")
    req = urllib.request.Request(url, data=gzip.compress(data), method="POST", headers={
        "Authorization": "Bearer " + token, "Content-Type": "application/json", "Content-Encoding": "gzip"})
    try:
        with opener(req, timeout=300) as r:
            return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read() or b"{}")
        except ValueError:
            return e.code, {}


def size_text(n):
    """Format a byte count the way the import dialog does, such as 2.7 MB."""
    for unit in ("B", "KB", "MB", "GB"):
        if n < 1000 or unit == "GB":
            return ("%d %s" if unit == "B" else "%.1f %s") % (n, unit)
        n /= 1000.0


def report(summary, out=sys.stderr):
    """Print what a summary holds, line by line as the import dialog previews it."""
    verdict = summary["verdict"] or "no verdict"
    failed = next((g for g in summary["gates"] if g["result"] not in ("pass", "running")), None)
    if summary["verdict"] == "FAIL" and failed:
        verdict += " (%s %s on %s)" % (failed["gate"], failed["result"], failed["node"])
    samples = sum(len(r["samples"]) for r in summary["resources"].values())
    ckpt = summary["checkpoints"]
    if ckpt["iterations"]:
        ckpt_text = "%d checkpoints" % len(ckpt["iterations"])
    else:
        ckpt_text = "no checkpoints" if summary["saveSet"] else "--save not set"
    log = summary["mainLog"]
    log_text = "none"
    if log:
        log_text = "%s · %s · %d step lines" % (log["name"], size_text(log["bytes"]), log["metricLines"])
    print("Scanning %s" % summary["name"], file=out)
    print("  node-*.log   %d nodes · %d gate results · %s" % (len(summary["nodes"]), len(summary["gates"]), verdict),
          file=out)
    print("  markers/     %d files" % len(summary["markers"]), file=out)
    print("  mon/         %d samples from %d nodes" % (samples, len(summary["resources"])), file=out)
    print("  ckpt/        %s" % ckpt_text, file=out)
    print("  main log     %s" % log_text, file=out)
    print("  arguments    {:,} parsed · {:d} kept".format(summary["arguments"]["total"],
                                                         len(summary["arguments"]["kept"])), file=out)
    for w in summary["warnings"]:
        print("  note: %s" % w, file=out)


def main(argv=None):
    """Parse a run directory and upload its summary, or write it to a file, or upload a written one."""
    p = argparse.ArgumentParser(prog="safe-post-training-import", description=__doc__.split("\n\n")[0])
    p.add_argument("run_dir", nargs="?", help="the Spur run directory")
    p.add_argument("--format", default=FORMAT, choices=sorted(FORMATS))
    mode = p.add_mutually_exclusive_group()
    mode.add_argument("--dry-run", action="store_true", help="print the summary without uploading it")
    mode.add_argument("--out", help="write the summary to a file instead of uploading it")
    mode.add_argument("--upload", help="upload a summary written with --out")
    mode.add_argument("--stage", action="store_true", help=argparse.SUPPRESS)
    p.add_argument("--root", help=argparse.SUPPRESS)
    args = p.parse_args(argv)
    try:
        if args.upload:
            with open(args.upload, "rb") as f:
                data = f.read()
            data = gzip.decompress(data) if data[:2] == b"\x1f\x8b" else data
        else:
            if not args.run_dir:
                p.error("the run directory is required")
            root = os.path.realpath(args.run_dir).rstrip("/")
            if args.root and not root.startswith(os.path.realpath(args.root).rstrip("/") + "/"):
                raise RunDirError("The directory must be under %s." % args.root)
            summary = FORMATS[args.format](root)
            data = encode(summary)
            report(summary)
            if args.dry_run:
                for s in summary["metrics"].values():
                    s["points"] = "%d points" % len(s["points"])
                print(json.dumps(summary, indent=1))
                return 0
            if args.out:
                out = os.path.realpath(args.out)
                if out == root or out.startswith(root + os.sep):
                    raise RunDirError("--out must not be inside the run directory, which the import only reads")
                with open(out, "wb") as f:
                    f.write(gzip.compress(data) if out.endswith(".gz") else data)
                print("wrote the summary to %s" % out, file=sys.stderr)
                return 0
        token = read_token()
        status, answer = upload(data, token, stage=args.stage)
    except (RunDirError, OSError, urllib.error.URLError) as e:
        print("error: %s" % e, file=sys.stderr)
        note_scan_failure(args, str(e))
        return 1
    detail = token_address(token).rstrip("/") + "/post-training/detail?runId="
    if status in (200, 201):
        if args.stage:
            print("Staged the summary as %s." % answer.get("importId", ""), file=sys.stderr)
        else:
            print("Uploading the summary to workspace %s ... done" % answer.get("workspace", ""),
                  file=sys.stderr)
            print("Imported as %s: %s%s" % (answer.get("displayName", ""), detail, answer.get("runId", "")),
                  file=sys.stderr)
        return 0
    if status == 401:
        message = "Token expired or already used. Get a new command from the Import dialog in SaFE."
    elif status == 409 and answer.get("runId"):
        message = "This directory is already imported: %s%s" % (detail, answer["runId"])
    else:
        message = "The upload was refused: %s" % (answer.get("errorMessage") or answer.get("message") or
                                                  "HTTP %d" % status)
    print(message, file=sys.stderr)
    note_scan_failure(args, message)
    return 2


def note_scan_failure(args, message):
    """Leave a scan job's failure as its container's termination message, which the import dialog shows."""
    if not args.stage:
        return
    try:
        with open(TERMINATION_LOG, "w") as f:
            f.write(message[:4096])
    except OSError:
        pass


if __name__ == "__main__":
    sys.exit(main())
