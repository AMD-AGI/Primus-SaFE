#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

docker buildx build . -f pytorch/Dockerfile \
  --build-arg ROCM_VERSION=6.4.3 \
  -t primussafe/pytorch:rocm6.4.3_ubuntu22.04_py3.10
