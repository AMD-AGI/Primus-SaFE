#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

ROCM_VERSION=7.0.3
GPU_ARCHS=gfx950
OS_VERSION=22.04
PY_VERSION=3.10

docker buildx build . -f pytorch/Dockerfile \
  --build-arg ROCM_VERSION=${ROCM_VERSION} \
  --build-arg GPU_ARCHS="${GPU_ARCHS}" \
  --build-arg OS_VERSION="${OS_VERSION}" \
  --build-arg PY_VERSION="${PY_VERSION}" \
  -t primussafe/pytorch:rocm${ROCM_VERSION}_${GPU_ARCHS}_oci${OS_VERSION}_py${PY_VERSION}