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

# Get sorted list of executable scripts (all executable files in scripts_dir)
SCRIPTS=()
while IFS= read -r -d '' f; do
    SCRIPTS+=("$f")
done < <(find "$SCRIPTS_DIR" -maxdepth 1 -type f -executable -print0 2>/dev/null | sort -z)

if [ ${#SCRIPTS[@]} -eq 0 ]; then
    echo "Error: no executable scripts found in $SCRIPTS_DIR" >&2
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

for host in "${NODES[@]}"; do
    for script_path in "${SCRIPTS[@]}"; do
        script_name="$(basename "$script_path")"
        if ssh "${SSH_OPTS[@]}" "$host" "bash -s" < "$script_path" 2>/dev/null; then
            RESULTS+=("${host}|${script_name}|OK")
        else
            RESULTS+=("${host}|${script_name}|FAIL")
        fi
    done
done

# Output summary
echo ""
echo "========== Execution Summary =========="
printf "%-40s %-40s %s\n" "NODE" "SCRIPT" "STATUS"
echo "----------------------------------------"

for r in "${RESULTS[@]}"; do
    IFS='|' read -r node script status <<< "$r"
    printf "%-40s %-40s %s\n" "$node" "$script" "$status"
done

# Count failures
FAIL_COUNT=0
for r in "${RESULTS[@]}"; do
    [[ "$r" == *"|FAIL" ]] && ((FAIL_COUNT++)) || true
done

echo "----------------------------------------"
echo "Total: ${#RESULTS[@]} executions, $FAIL_COUNT failed"
[ "$FAIL_COUNT" -gt 0 ] && exit 1 || exit 0
