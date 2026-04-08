#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Build and push bench / pytorch / full images (ROCm 7.0.3, gfx950, oci 22.04).
# No registry credentials in this script; run harbor-login-k8s.sh (or docker login) first.
#
# Environment:
#   HARBOR_REGISTRY   default harbor.oci-slc.primus-safe.amd.com
#   HARBOR_PROJECT    default primussafe
#   TAG_SUFFIX        default current YYYYMMDDHHMM (same suffix for all three images)
#   AINIC_BUNDLE      path to ainic_bundle*.tar.gz (optional; skip --ainic if unset/empty)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

HARBOR_REGISTRY="${HARBOR_REGISTRY:-harbor.oci-slc.primus-safe.amd.com}"
HARBOR_PROJECT="${HARBOR_PROJECT:-primussafe}"
TAG_SUFFIX="${TAG_SUFFIX:-$(date +%Y%m%d%H%M)}"

ROCM_VER="${ROCM_VER:-7.0.3}"
GPU_ARCH="${GPU_ARCH:-gfx950}"
OS_NAME="${OS_NAME:-oci}"
OS_VER="${OS_VER:-22.04}"

AINIC_ARGS=()
if [ -n "${AINIC_BUNDLE:-}" ] && [ -f "${AINIC_BUNDLE}" ]; then
    AINIC_ARGS=(--ainic "${AINIC_BUNDLE}")
elif [ -f "${BENCH_DIR}/preflight/install/ainic_bundle_1.117.5-a-56.tar.gz" ]; then
    AINIC_ARGS=(--ainic "${BENCH_DIR}/preflight/install/ainic_bundle_1.117.5-a-56.tar.gz")
fi

PREFIX="${HARBOR_REGISTRY}/${HARBOR_PROJECT}/primusbench"
TAG_MID="multi-rocm703-gfx950-oci-${TAG_SUFFIX}"

for T in bench pytorch full; do
    echo "========== ${T} =========="
    "${SCRIPT_DIR}/build.sh" \
        --target "${T}" \
        --rocm "${ROCM_VER}" \
        --gpu "${GPU_ARCH}" \
        --os "${OS_NAME}" \
        --os-version "${OS_VER}" \
        "${AINIC_ARGS[@]}" \
        --tag "${PREFIX}:${T}-${TAG_MID}" \
        --push
done

echo "All three images pushed under ${PREFIX} with suffix ${TAG_MID}"
