#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
set -e 

if [ -d "/opt/TransferBench" ]; then
  exit 0
fi

# Check required environment variables
if [ -z "$ROCM_VERSION" ]; then
  echo "Error: ROCM_VERSION environment variable is not set" >&2
  exit 1
fi

if [ -z "$GPU_ARCHS" ]; then
  echo "Error: GPU_ARCHS environment variable is not set" >&2
  exit 1
fi

TRANSFER_TAG=""
if [ "$ROCM_VERSION" = "6.4.3" ]; then
  TRANSFER_TAG="rocm-6.4.3"
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  TRANSFER_TAG="rocm-7.0.2"
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3 and 7.0.3 are supported." >&2
  exit 1
fi

REPO_URL="https://github.com/ROCm/TransferBench.git"
cd /opt
git clone --branch "$TRANSFER_TAG" --depth 1 "$REPO_URL"

cd "./TransferBench"
# GPU_ARCHS: gfx942 (mi300x/mi325x), gfx950 (mi355x)
echo "Building TransferBench with GPU_TARGETS=$GPU_ARCHS"
CC=hipcc make GPU_TARGETS="$GPU_ARCHS"