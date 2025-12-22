#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

docker buildx build . -f pytorch/Dockerfile \
  --build-arg ROCM_VERSION=7.0.3 \
  --build-arg GPU_ARCHS="gfx950" \
  -t primussafe/pytorch:rocm7.0.3_gfx950_ubuntu22.04_py3.10