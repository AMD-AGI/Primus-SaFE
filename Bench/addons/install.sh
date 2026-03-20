#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Run scripts on multiple nodes via SSH (parallel across hosts).
#
# Usage:
#   ./install.sh <nodes_file> <scripts_dir> [cluster_name]
#
# Arguments:
#   nodes_file   - File containing node hostnames, one per line (comments and empty lines ignored)
#   scripts_dir  - Directory containing scripts to execute (executed in alphabetical order)
#   cluster_name - Optional. If provided, additionally runs scripts from scripts_dir/<cluster_name>/
#
# Prerequisites:
#   - SSH key-based authentication configured (passwordless login)
#   - Scripts in scripts_dir must be executable
#
# Output:
#   Per-node, per-script execution status (OK/FAIL). Hosts are processed in parallel.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if command -v python3 >/dev/null 2>&1; then
    exec python3 "${SCRIPT_DIR}/install.py" "$@"
else
    exec python "${SCRIPT_DIR}/install.py" "$@"
fi
