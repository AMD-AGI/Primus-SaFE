#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "==============  begin to install rccl for ROCm $ROCM_VERSION =============="
set -e

# Set the RCCL tag based on ROCM_VERSION
if [ "$ROCM_VERSION" = "6.4.3" ]; then
  RCCL_TAG="rocm-6.4.3"
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  RCCL_TAG="rocm-7.0.2"
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3 and 7.0.3 are supported."
  exit 1
fi

echo "Cloning RCCL with tag $RCCL_TAG..."
cd /opt && git clone --branch $RCCL_TAG --depth 1 https://github.com/ROCm/rccl
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone RCCL repository with tag $RCCL_TAG"
  exit 1
fi

echo "Building and installing RCCL..."
cd rccl
bash ./install.sh -l --disable-mscclpp --disable-msccl-kernel > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to build and install RCCL"
  exit 1
fi
echo "==============  install rccl $RCCL_TAG successfully =============="
