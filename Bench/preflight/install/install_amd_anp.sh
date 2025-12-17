#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

ANP_VERSION="v1.3.0"

echo "============== begin to install AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} =============="

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

# Checkout specific version
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
           ROCM_PATH=/opt/rocm 2>&1 | tee log_amd_anp_build.txt
if [ ${PIPESTATUS[0]} -ne 0 ]; then
  echo "Error: Failed to build AMD ANP driver. See log_amd_anp_build.txt for details."
  exit 1
fi

echo "============== install  AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} successfully =============="
