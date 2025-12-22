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

# Modify Makefile for GPU architecture support
if [ -z "${GPU_ARCHS}" ]; then
  echo "Warning: GPU_ARCHS not set, defaulting to gfx950"
  GPU_ARCHS="gfx950"
fi

echo "Modifying Makefile for GPU architectures: ${GPU_ARCHS}..."
# Build CFLAGS line with all specified architectures
ARCH_FLAGS=""
for arch in ${GPU_ARCHS}; do
  ARCH_FLAGS="${ARCH_FLAGS} --offload-arch=${arch}"
done
sed -i "5a CFLAGS +=${ARCH_FLAGS}" ./Makefile
if [ $? -ne 0 ]; then
  echo "Error: Failed to modify Makefile"
  exit 1
fi

# Build
echo "Building AMD ANP driver..."
export RCCL_HOME=/opt/rccl 
# RCCL_BUILD points to where RCCL is installed (with lib/ and include/ subdirectories)
make -j 16 MPI_INCLUDE=/opt/mpich/include/ \
           MPI_LIB_PATH=/opt/mpich/lib/ \
           ROCM_PATH=/opt/rocm 
if [ $? -ne 0 ]; then
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

echo "============== install  AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} successfully =============="
