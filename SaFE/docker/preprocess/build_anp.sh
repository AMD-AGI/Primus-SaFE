#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

# Check if AMD_ANP_VERSION is not set
if [ -z "$AMD_ANP_VERSION" ]; then
  echo "AMD_ANP_VERSION is not set. Skipping build."
  exit 0
fi

ANP_REPO="https://github.com/rocm/amd-anp.git"
ANP_DIR="amd-anp"
WORKDIR="/opt"

# Check if amd-anp directory already exists
if [ -d "${WORKDIR}/${ANP_DIR}" ]; then
  echo "AMD ANP directory already exists at ${WORKDIR}/${ANP_DIR}. Skipping build."
  exit 0
fi

cd ${WORKDIR}

# Clone AMD ANP repository
echo "Cloning AMD ANP repository..."
git clone ${ANP_REPO}
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone AMD ANP repository from ${ANP_REPO}"
  exit 1
fi

# Checkout specific version or branch
echo "Checking out version ${AMD_ANP_VERSION}..."
cd ${ANP_DIR}
git checkout tags/${AMD_ANP_VERSION}
if [ $? -ne 0 ]; then
  echo "Error: Failed to checkout version ${AMD_ANP_VERSION}"
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
ret=0
export RCCL_HOME=/opt/rccl 
if [ -d "/opt/openmpi" ]; then
  make -j 16 MPI_INCLUDE=/opt/openmpi/include/ \
             MPI_LIB_PATH=/opt/openmpi/lib/ \
             ROCM_PATH=/opt/rocm 
  ret=$?
else
  make -j 16 ROCM_PATH=/opt/rocm
  ret=$?
fi
if [ $ret -ne 0 ]; then
  echo "Error: Failed to build AMD ANP driver."
  exit 1
fi

# Create symlink librccl-net.so -> librccl-anp.so if needed (RCCL looks for librccl-net.so)
ANP_BUILD_DIR="${WORKDIR}/${ANP_DIR}/build"
if [ -f "${ANP_BUILD_DIR}/librccl-anp.so" ] && [ ! -e "${ANP_BUILD_DIR}/librccl-net.so" ]; then
  echo "Creating symlink: librccl-net.so -> librccl-anp.so"
  cd ${ANP_BUILD_DIR}
  ln -sf librccl-anp.so librccl-net.so
  if [ $? -eq 0 ]; then
    echo "Symlink created successfully."
  else
    echo "Warning: Failed to create symlink."
  fi
fi

echo "============== install AMD AINIC Network Plugin (amd-anp) ${AMD_ANP_VERSION} successfully =============="
