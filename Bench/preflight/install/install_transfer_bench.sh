#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
set -e 

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
rm -rf TransferBench
git clone --branch "$TRANSFER_TAG" --depth 1 "$REPO_URL"

cd "./TransferBench"
# GPU_ARCHS: gfx942 (mi300x/mi325x), gfx950 (mi355x)
echo "Building TransferBench with GPU_TARGETS=$GPU_ARCHS and ROCM_VERSION=$ROCM_VERSION"

# Build directly with hipcc since Makefile doesn't support GPU_TARGETS
echo "Building TransferBench with --offload-arch=$GPU_ARCHS"
/opt/rocm/bin/hipcc \
    --offload-arch="$GPU_ARCHS" \
    -I/opt/rocm/include \
    -I./src/header -I./src/client -I./src/client/Presets \
    -O3 \
    -lnuma -L/opt/rocm/lib -lhsa-runtime64 \
    src/client/Client.cpp \
    -o TransferBench \
    -lpthread -libverbs -DNIC_EXEC_ENABLED

# Verify the build
echo "Verifying TransferBench GPU architecture:"
strings ./TransferBench | grep 'amdgcn.*gfx'
