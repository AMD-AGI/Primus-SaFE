#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

echo "============== begin to install AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} =============="

# Set ANP version based on ROCM_VERSION
if [ "$ROCM_VERSION" = "7.0.3" ]; then
  ANP_VERSION="v1.1.0-5"
elif [ "$ROCM_VERSION" = "7.1" ]; then
  ANP_VERSION="v1.3.0"
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 7.0.3 and 7.1 are supported."
  exit 1
fi

ANP_REPO="https://github.com/rocm/amd-anp.git"
ANP_DIR="amd-anp"
WORKDIR="/opt"

cd ${WORKDIR}

# Clone AMD ANP repository
echo "Cloning AMD ANP repository..."
git clone ${ANP_REPO}
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone AMD ANP repository from ${ANP_REPO}"
  exit 1
fi

# Checkout specific version or branch
echo "Checking out version ${ANP_VERSION}..."
cd ${ANP_DIR}
git checkout tags/${ANP_VERSION}
if [ $? -ne 0 ]; then
  echo "Error: Failed to checkout version ${ANP_VERSION}"
  exit 1
fi

# Modify Makefile for gfx950 support
echo "Modifying Makefile for gfx950 offload architecture..."
sed -i '5a CFLAGS += --offload-arch=gfx950' ./Makefile
if [ $? -ne 0 ]; then
  echo "Error: Failed to modify Makefile"
  exit 1
fi

# Build
echo "Building AMD ANP driver..."
make -j 16 RCCL_HOME=/opt/rccl \
           MPI_INCLUDE=/opt/openmpi/include/ \
           MPI_LIB_PATH=/opt/openmpi/lib/ \
           ROCM_PATH=/opt/rocm
if [ $? -ne 0 ]; then
  echo "Error: Failed to build AMD ANP driver."
  exit 1
fi

echo "============== install  AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} successfully =============="
