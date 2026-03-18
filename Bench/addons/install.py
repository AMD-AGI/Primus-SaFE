#!/usr/bin/env python3
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Run scripts on multiple nodes via SSH (parallel across hosts).
#
# Usage:
#   python3 install.py <nodes_file> <scripts_dir> [cluster_name]
#
# Arguments:
#   nodes_file   - File containing node hostnames, one per line (comments and empty lines ignored)
#   scripts_dir  - Directory containing scripts to execute (top-level only, no subdirs)
#   cluster_name - Optional. If provided, additionally runs scripts from scripts_dir/<cluster_name>/
#
# Prerequisites:
#   - SSH key-based authentication configured (passwordless login)
#   - Scripts in scripts_dir must be executable
#
# Output:
#   Per-node, per-script execution status (OK/FAIL). Hosts are processed in parallel.

import argparse
import os
import subprocess
import sys
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Optional

SSH_OPTS = [
    "-o", "StrictHostKeyChecking=no",
    "-o", "UserKnownHostsFile=/dev/null",
    "-o", "BatchMode=yes",
    "-o", "ConnectTimeout=10",
]

print_lock = threading.Lock()


def log(msg: str) -> None:
    with print_lock:
        print(msg, flush=True)


def run_script_on_host(host: str, script_path: Path) -> tuple[str, str, bool]:
    """Run a single script on a host via SSH. Returns (host, script_name, success)."""
    script_name = script_path.name
    try:
        with open(script_path, "rb") as f:
            result = subprocess.run(
                ["ssh"] + SSH_OPTS + [host, "bash -s"],
                stdin=f,
                capture_output=True,
                timeout=300,
            )
        success = result.returncode == 0
        return (host, script_name, success)
    except subprocess.TimeoutExpired:
        return (host, script_name, False)
    except Exception:
        return (host, script_name, False)


def run_host(host: str, scripts: list[Path]) -> list[tuple[str, str, bool]]:
    """Run all scripts on one host sequentially. Returns list of (host, script_name, success)."""
    results = []
    for script_path in scripts:
        r = run_script_on_host(host, script_path)
        results.append(r)
        status = "OK" if r[2] else "FAIL"
        log(f"  {host}: {r[1]}: {status}")
    return results


def load_nodes(nodes_file: str) -> list[str]:
    nodes = []
    with open(nodes_file) as f:
        for line in f:
            line = line.split("#")[0].strip()
            if line:
                nodes.append(line)
    return nodes


def _collect_scripts_from_dir(dir_path: Path) -> list[Path]:
    """Collect executable or .sh files from a directory (top-level only, no subdirs)."""
    candidates = []
    for f in dir_path.iterdir():
        if not f.is_file():
            continue
        if f.suffix == ".sh" or os.access(f, os.X_OK):
            candidates.append(f)
    return sorted(candidates, key=lambda p: p.name)


def get_scripts(scripts_dir: str, cluster_name: Optional[str]) -> list[Path]:
    """Get scripts: always from scripts_dir (top-level), plus scripts_dir/<cluster_name>/ if cluster given."""
    scripts_dir_path = Path(scripts_dir)
    if not scripts_dir_path.is_dir():
        return []

    # Base: scripts in scripts_dir (top-level only, no subdirs)
    scripts = _collect_scripts_from_dir(scripts_dir_path)

    # If cluster_name provided, additionally add scripts from scripts_dir/<cluster_name>/
    if cluster_name:
        cluster_dir = scripts_dir_path / cluster_name
        if cluster_dir.is_dir():
            cluster_scripts = _collect_scripts_from_dir(cluster_dir)
            scripts = scripts + cluster_scripts  # base first, then cluster-specific

    return scripts


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Run scripts on multiple nodes via SSH (parallel across hosts)."
    )
    parser.add_argument("nodes_file", help="File with one host per line")
    parser.add_argument("scripts_dir", help="Directory containing scripts to run")
    parser.add_argument(
        "cluster_name",
        nargs="?",
        default=None,
        help='Optional. If provided, additionally run scripts from scripts_dir/<cluster_name>/',
    )
    args = parser.parse_args()

    if not os.path.isfile(args.nodes_file):
        print(f"Error: nodes file not found: {args.nodes_file}", file=sys.stderr)
        return 1

    if not os.path.isdir(args.scripts_dir):
        print(f"Error: scripts directory not found: {args.scripts_dir}", file=sys.stderr)
        return 1

    scripts = get_scripts(args.scripts_dir, args.cluster_name)
    if not scripts:
        print(f"Error: no scripts found in {args.scripts_dir}", file=sys.stderr)
        return 1

    nodes = load_nodes(args.nodes_file)
    if not nodes:
        print(f"Error: no nodes found in {args.nodes_file}", file=sys.stderr)
        return 1

    if args.cluster_name:
        log(f"Cluster '{args.cluster_name}': running {len(scripts)} scripts on {len(nodes)} hosts (parallel)")
    else:
        log(f"Running {len(scripts)} scripts on {len(nodes)} hosts (parallel)")

    all_results: list[tuple[str, str, bool]] = []
    max_workers = min(len(nodes), 32)

    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        futures = {executor.submit(run_host, host, scripts): host for host in nodes}
        for future in as_completed(futures):
            host = futures[future]
            try:
                results = future.result()
                all_results.extend(results)
            except Exception as e:
                log(f"  {host}: ERROR: {e}")
                for sp in scripts:
                    all_results.append((host, sp.name, False))

    fail_count = sum(1 for _, _, ok in all_results if not ok)
    log("")
    log("========== Summary ==========")
    log(f"Total: {len(all_results)} executions, {fail_count} failed")

    return 1 if fail_count > 0 else 0


if __name__ == "__main__":
    sys.exit(main())
