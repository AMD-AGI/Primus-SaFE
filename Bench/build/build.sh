#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Unified build script for PrimusBench multi-stage Docker images.
#
# Usage:
#   ./build/build.sh [OPTIONS]
#
# Options:
#   --target <stage>       Build target: base, bench, pytorch, full (default: full)
#   --rocm <version>       ROCm version: 6.4.3, 7.0.3, 7.2.0 (default: 7.0.3)
#   --gpu <arch>           GPU architecture: gfx942, gfx950 (default: gfx950)
#   --os <name>            OS name: ubuntu, oci (default: oci)
#   --os-version <ver>     OS version: 22.04, 24.04 (default: 22.04)
#   --ainic <path>         Path to AINIC bundle tarball (optional)
#   --turbo-commit <hash>  Primus-Turbo commit (default: 79373eb...)
#   --te-branch <branch>   Transformer Engine branch (default: stable)
#   --tag <tag>            Custom image tag (overrides auto-generated tag)
#   --no-cache             Build without Docker cache
#   --dry-run              Print the docker build command without executing
#   -h, --help             Show this help message
#

set -e
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCH_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
TARGET="full"
ROCM_VERSION="7.0.3"
GPU_ARCHS="gfx950"
OS_NAME="oci"
OS_VERSION="22.04"
AINIC_BUNDLE_PATH=""
PRIMUS_TURBO_COMMIT="79373eb781a54fd49aed9430c8718489409d1dd0"
TE_BRANCH="stable"
CUSTOM_TAG=""
NO_CACHE=""
DRY_RUN=false

usage() {
    head -25 "${BASH_SOURCE[0]}" | grep '^#' | sed 's/^# \?//'
    exit 0
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --target)   TARGET="$2"; shift 2 ;;
        --rocm)     ROCM_VERSION="$2"; shift 2 ;;
        --gpu)      GPU_ARCHS="$2"; shift 2 ;;
        --os)       OS_NAME="$2"; shift 2 ;;
        --os-version) OS_VERSION="$2"; shift 2 ;;
        --ainic)    AINIC_BUNDLE_PATH="$2"; shift 2 ;;
        --turbo-commit) PRIMUS_TURBO_COMMIT="$2"; shift 2 ;;
        --te-branch) TE_BRANCH="$2"; shift 2 ;;
        --tag)      CUSTOM_TAG="$2"; shift 2 ;;
        --no-cache) NO_CACHE="--no-cache"; shift ;;
        --dry-run)  DRY_RUN=true; shift ;;
        -h|--help)  usage ;;
        *) echo "Unknown option: $1"; usage ;;
    esac
done

# Validate target
case "${TARGET}" in
    base|bench|pytorch|full) ;;
    *) echo "Error: Invalid target '${TARGET}'. Must be: base, bench, pytorch, full"; exit 1 ;;
esac

# Handle AINIC bundle
AINIC_FILENAME=""
if [ -n "${AINIC_BUNDLE_PATH}" ] && [ -f "${AINIC_BUNDLE_PATH}" ]; then
    AINIC_FILENAME=$(basename "${AINIC_BUNDLE_PATH}")
    cp "${AINIC_BUNDLE_PATH}" "${BENCH_DIR}/preflight/install/${AINIC_FILENAME}"
    echo "Copied AINIC bundle to preflight/install/${AINIC_FILENAME}"
fi

# Auto-generate image tag
IMAGE_VERSION=$(date +%Y%m%d%H%M)
if [ -n "${CUSTOM_TAG}" ]; then
    IMAGE_TAG="${CUSTOM_TAG}"
else
    AINIC_SUFFIX=""
    if [ -n "${AINIC_FILENAME}" ]; then AINIC_SUFFIX="_ainic"; fi
    IMAGE_TAG="primussafe/primusbench:${TARGET}_rocm${ROCM_VERSION}_${GPU_ARCHS}_${OS_NAME}${OS_VERSION}${AINIC_SUFFIX}_${IMAGE_VERSION}"
fi

echo "============================================"
echo "PrimusBench Docker Build"
echo "============================================"
echo "  Target:       ${TARGET}"
echo "  ROCm:         ${ROCM_VERSION}"
echo "  GPU:          ${GPU_ARCHS}"
echo "  OS:           ${OS_NAME} ${OS_VERSION}"
echo "  AINIC:        ${AINIC_FILENAME:-none}"
echo "  Turbo commit: ${PRIMUS_TURBO_COMMIT}"
echo "  TE branch:    ${TE_BRANCH}"
echo "  Image tag:    ${IMAGE_TAG}"
echo "============================================"

BUILD_CMD=(
    docker buildx build "${BENCH_DIR}"
    -f "${SCRIPT_DIR}/Dockerfile"
    --target "${TARGET}"
    --build-arg ROCM_VERSION="${ROCM_VERSION}"
    --build-arg GPU_ARCHS="${GPU_ARCHS}"
    --build-arg OS_VERSION="${OS_VERSION}"
    --build-arg OS_NAME="${OS_NAME}"
    --build-arg AINIC_BUNDLE_FILENAME="${AINIC_FILENAME}"
    --build-arg PRIMUS_TURBO_COMMIT="${PRIMUS_TURBO_COMMIT}"
    --build-arg TE_BRANCH="${TE_BRANCH}"
    --network=host
    --load
    ${NO_CACHE}
    -t "${IMAGE_TAG}"
)

if [ "${DRY_RUN}" = true ]; then
    echo ""
    echo "Dry run - would execute:"
    echo "${BUILD_CMD[@]}"
    exit 0
fi

echo ""
echo "Building..."
"${BUILD_CMD[@]}" 2>&1 | tee "${BENCH_DIR}/build/build.log"

# Cleanup AINIC bundle copy
if [ -n "${AINIC_FILENAME}" ] && [ -f "${BENCH_DIR}/preflight/install/${AINIC_FILENAME}" ]; then
    rm -f "${BENCH_DIR}/preflight/install/${AINIC_FILENAME}"
    echo "Cleaned up preflight/install/${AINIC_FILENAME}"
fi

echo ""
echo "Build complete: ${IMAGE_TAG}"
