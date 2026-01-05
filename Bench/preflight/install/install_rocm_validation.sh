#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install ROCm Validation Suite =============="
set -e

RVS_REPO="https://github.com/ROCm/ROCmValidationSuite.git"
RVS_DIR="ROCmValidationSuite"
WORKDIR="/opt"
cd ${WORKDIR}

RVS_TAG=""
if [ "$ROCM_VERSION" = "6.4.3" ]; then
  RVS_TAG="rocm-6.4.3"
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  RVS_TAG="rocm-7.0.2"
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3 and 7.0.3 are supported."
  exit 1
fi

# Clone RVS repository
echo "Cloning ROCm Validation Suite with tag ${RVS_TAG}..."
git clone --branch ${RVS_TAG} --depth 1 ${RVS_REPO}
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone ROCm Validation Suite"
  exit 1
fi

# Build
echo "Building ROCm Validation Suite ${RVS_TAG}..."
cd ${RVS_DIR}
mkdir -p build
cd build
cmake -DROCM_PATH=/opt/rocm -DCMAKE_PREFIX_PATH="/opt/rocm;/opt/rocBLAS" .. > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to configure ROCm Validation Suite"
  exit 1
fi

make -j 16 > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to build ROCm Validation Suite."
  exit 1
fi

# Install
echo "Installing ROCm Validation Suite..."
make install > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to install ROCm Validation Suite"
  exit 1
fi

# Cleanup
echo "Cleaning up temporary files..."
cd ${WORKDIR}
rm -rf ${RVS_DIR}

echo "============== install ROCm Validation Suite successfully =============="