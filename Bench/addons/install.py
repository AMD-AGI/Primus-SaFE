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
# Behavior:
#   - Copies entire scripts_dir (including subdirs) to each host before running
#   - Each script runs with cwd=its directory, so scripts can call siblings (e.g. bash other.sh)
#
# Output:
#   Per-node, per-script execution status (OK/FAIL). Hosts are processed in parallel.

import argparse
import os
import subprocess
import sys
import threading
import uuid
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Optional

SSH_OPTS = [
    "-o", "StrictHostKeyChecking=no",
    "-o", "UserKnownHostsFile=/dev/null",
    "-o", "BatchMode=yes",
    "-o", "ConnectTimeout=10",
]
SCP_OPTS = SSH_OPTS  # scp uses same options

print_lock = threading.Lock()


def log(msg: str) -> None:
    with print_lock:
        print(msg, flush=True)


def _copy_scripts_to_host(host: str, scripts_dir: Path, remote_base: str) -> bool:
    """Copy entire scripts_dir (including subdirs) to remote host. Returns True on success."""
    try:
        subprocess.run(
            ["ssh"] + SSH_OPTS + [host, f"mkdir -p {remote_base}"],
            capture_output=True,
            timeout=10,
            check=True,
        )
        subprocess.run(
            ["scp"] + ["-r"] + SCP_OPTS + [str(scripts_dir), f"{host}:{remote_base}/"],
            capture_output=True,
            timeout=120,
            check=True,
        )
        return True
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, Exception):
        return False


def _run_script_on_remote(host: str, script_path: Path, scripts_dir: Path, remote_base: str) -> tuple[str, str, bool]:
    """Run script on remote host. Script runs with cwd=its directory so relative paths work."""
    script_path = script_path.resolve()
    scripts_dir = scripts_dir.resolve()
    script_name = script_path.name
    try:
        # Relative path from scripts_dir, e.g. "oci" for scripts/oci/foo.sh
        rel = script_path.parent.relative_to(scripts_dir)
        remote_script_dir = f"{remote_base}/{scripts_dir.name}/{rel}" if rel != Path(".") else f"{remote_base}/{scripts_dir.name}"
        cmd = f"cd {remote_script_dir} && bash {script_name}"
        result = subprocess.run(
            ["ssh"] + SSH_OPTS + [host, cmd],
            capture_output=True,
            timeout=300,
        )
        success = result.returncode == 0
        return (host, script_name, success)
    except subprocess.TimeoutExpired:
        return (host, script_name, False)
    except Exception:
        return (host, script_name, False)


def run_host(host: str, scripts: list[Path], scripts_dir: Path) -> list[tuple[str, str, bool]]:
    """Copy scripts to host, run each with correct cwd, cleanup. Returns list of (host, script_name, success)."""
    remote_base = f"/tmp/primus-addons-{uuid.uuid4().hex[:12]}"
    results = []

    if not _copy_scripts_to_host(host, scripts_dir, remote_base):
        log(f"  {host}: FAIL (could not copy scripts)")
        return [(host, sp.name, False) for sp in scripts]

    try:
        for script_path in scripts:
            r = _run_script_on_remote(host, script_path, scripts_dir, remote_base)
            results.append(r)
            status = "OK" if r[2] else "FAIL"
            log(f"  {host}: {r[1]}: {status}")
    finally:
        try:
            subprocess.run(
                ["ssh"] + SSH_OPTS + [host, f"rm -rf {remote_base}"],
                capture_output=True,
                timeout=10,
            )
        except Exception:
            pass

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
    scripts_dir_path = Path(args.scripts_dir).resolve()

    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        futures = {executor.submit(run_host, host, scripts, scripts_dir_path): host for host in nodes}
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
