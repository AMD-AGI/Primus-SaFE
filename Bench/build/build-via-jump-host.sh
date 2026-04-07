#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Run PrimusBench Docker build on a worker node reachable only via a jump host.
# Intended flow: local -> root@JUMP_HOST -> root@BUILD_NODE (Docker + NFS repo).
#
# Environment (optional):
#   SSH_IDENTITY     Path to SSH private key (passed as ssh -i)
#   JUMP_HOST        Default: root@uswslocpm2m-primus-001.amd.com
#   BUILD_NODE       Default: uswslocpm2m-106-2018 (short name; must resolve on jump)
#   BENCH_ON_BUILD   Default: /shared_nfs/haiskong/Primus-SaFE/Bench
#
# Example:
#   SSH_IDENTITY=~/workspace/id_rsa ./build/build-via-jump-host.sh --target full --rocm 7.0.3 --gpu gfx950 --os oci --os-version 22.04
#

set -euo pipefail

JUMP_HOST="${JUMP_HOST:-root@uswslocpm2m-primus-001.amd.com}"
BUILD_NODE="${BUILD_NODE:-uswslocpm2m-106-2018}"
BENCH_ON_BUILD="${BENCH_ON_BUILD:-/shared_nfs/haiskong/Primus-SaFE/Bench}"

FIRST_SSH=(-o StrictHostKeyChecking=no -o ConnectTimeout=20)
if [[ -n "${SSH_IDENTITY:-}" ]]; then
  FIRST_SSH+=(-i "${SSH_IDENTITY}")
fi

# Quote only build.sh arguments for embedding in the inner ssh command (do not quote the whole
# "cd ... && ..." string, or operators like && get escaped and break on the worker).
quoted_args=""
if [[ $# -gt 0 ]]; then
  quoted_args=$(printf '%q ' "$@")
fi

# Second hop uses auth configured on the jump host (no -i from this machine).
exec ssh "${FIRST_SSH[@]}" "$JUMP_HOST" \
  "ssh -o StrictHostKeyChecking=no -o ConnectTimeout=20 root@${BUILD_NODE} \"cd ${BENCH_ON_BUILD} && exec ./build/build.sh ${quoted_args}\""
