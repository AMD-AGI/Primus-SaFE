#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Run scripts on multiple nodes via SSH.
#
# Usage:
#   ./run_scripts_on_nodes.sh <nodes_file> <scripts_dir>
#
# Arguments:
#   nodes_file  - File containing node hostnames, one per line (comments and empty lines ignored)
#   scripts_dir - Directory containing scripts to execute (executed in alphabetical order)
#
# Prerequisites:
#   - SSH key-based authentication configured (passwordless login)
#   - Scripts in scripts_dir must be executable
#
# Output:
#   Per-node, per-script execution status (OK/FAIL)

set -euo pipefail

SSH_OPTS=(
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
    -o BatchMode=yes
    -o ConnectTimeout=10
)

usage() {
    echo "Usage: $0 <nodes_file> <scripts_dir>"
    echo ""
    echo "  nodes_file  - File with one host per line"
    echo "  scripts_dir - Directory containing scripts to run"
    exit 1
}

if [ $# -ne 2 ]; then
    usage
fi

NODES_FILE="$1"
SCRIPTS_DIR="$2"

if [ ! -f "$NODES_FILE" ]; then
    echo "Error: nodes file not found: $NODES_FILE" >&2
    exit 1
fi

if [ ! -d "$SCRIPTS_DIR" ]; then
    echo "Error: scripts directory not found: $SCRIPTS_DIR" >&2
    exit 1
fi

# Get sorted list of scripts (executable files or .sh files; executed via bash -s)
SCRIPTS=()
while IFS= read -r -d '' f; do
    SCRIPTS+=("$f")
done < <(find "$SCRIPTS_DIR" -maxdepth 1 -type f \( -executable -o -name "*.sh" \) -print0 2>/dev/null | sort -z)

if [ ${#SCRIPTS[@]} -eq 0 ]; then
    echo "Error: no scripts found in $SCRIPTS_DIR (need executable or .sh files)" >&2
    exit 1
fi

# Read nodes (skip empty lines and # comments)
NODES=()
while IFS= read -r line || [ -n "$line" ]; do
    line="${line%%#*}"           # strip comment
    line="${line#"${line%%[![:space:]]*}"}"   # trim leading
    line="${line%"${line##*[![:space:]]}"}"  # trim trailing
    [ -n "$line" ] && NODES+=("$line")
done < "$NODES_FILE"

if [ ${#NODES[@]} -eq 0 ]; then
    echo "Error: no nodes found in $NODES_FILE" >&2
    exit 1
fi

# Results: host,script,status
declare -a RESULTS
FAIL_COUNT=0

for host in "${NODES[@]}"; do
    echo ""
    echo "=== $host ==="
    node_ok=0
    node_fail=0
    for script_path in "${SCRIPTS[@]}"; do
        script_name="$(basename "$script_path")"
        if ssh "${SSH_OPTS[@]}" "$host" "bash -s" < "$script_path" 2>/dev/null; then
            RESULTS+=("${host}|${script_name}|OK")
            echo "  $script_name: OK"
            ((node_ok++)) || true
        else
            RESULTS+=("${host}|${script_name}|FAIL")
            echo "  $script_name: FAIL"
            ((node_fail++)) || true
            ((FAIL_COUNT++)) || true
        fi
    done
    echo "  -> $node_ok ok, $node_fail failed"
done

# Final summary
echo ""
echo "========== Summary =========="
echo "Total: ${#RESULTS[@]} executions, $FAIL_COUNT failed"
[ "$FAIL_COUNT" -gt 0 ] && exit 1 || exit 0
