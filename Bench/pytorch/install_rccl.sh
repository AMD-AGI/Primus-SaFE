#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "==============  begin to install rccl for ROCm $ROCM_VERSION =============="
set -e

FLAGS="--disable-mscclpp --disable-msccl-kernel"
# Set the RCCL tag based on ROCM_VERSION
if [ "$ROCM_VERSION" = "6.4.3" ]; then
  RCCL_TAG="rocm-6.4.3"
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  RCCL_TAG="drop/2025-08"
elif [ "$ROCM_VERSION" = "7.2.0" ]; then
  RCCL_TAG="rocm-7.2.0"
  FLAGS=""
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3, 7.0.3 and 7.2.0 are supported."
  exit 1
fi

echo "Cloning RCCL with tag $RCCL_TAG..."
cd /opt && git clone --branch $RCCL_TAG --depth 1 https://github.com/ROCm/rccl
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone RCCL repository with tag $RCCL_TAG"
  exit 1
fi

echo "Building and installing RCCL for GPU architectures: $GPU_ARCHS..."
cd rccl

# Check if GPU_ARCHS is set
if [ -z "$GPU_ARCHS" ]; then
  echo "Error: GPU_ARCHS environment variable is not set"
  exit 1
fi

bash ./install.sh $FLAGS --prefix=/opt/rccl/build --amdgpu_targets="$GPU_ARCHS"
if [ $? -ne 0 ]; then
  echo "Error: Failed to build and install RCCL"
  exit 1
fi

# Verify the build contains correct GPU architecture
echo "Verifying RCCL build..."
if strings /opt/rccl/build/release/librccl.so.1 2>/dev/null | grep -q "$GPU_ARCHS"; then
  echo "RCCL successfully built for $GPU_ARCHS"
else
  echo "Warning: Could not verify $GPU_ARCHS in RCCL binary"
fi

echo "==============  install rccl $RCCL_TAG successfully =============="
