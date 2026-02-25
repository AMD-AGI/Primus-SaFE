#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

ANP_REPO="https://github.com/rocm/amd-anp.git"
ANP_DIR="amd-anp"
ANP_VERSION="v1.3.0"
LIBIONIC_VERSION="54.0-184"
WORKDIR="/opt"

# Get AINIC_DRIVER_VERSION from environment or extract from AINIC_BUNDLE_PATH
if [ -z "${AINIC_DRIVER_VERSION}" ] && [ -n "${AINIC_BUNDLE_PATH}" ]; then
  # Extract version from filename like ainic_bundle_1.117.5-a-56.tar.gz -> 1.117.5-a-56
  AINIC_BUNDLE_FILENAME=$(basename "${AINIC_BUNDLE_PATH}")
  AINIC_DRIVER_VERSION=$(echo "${AINIC_BUNDLE_FILENAME}" | sed -n 's/ainic_bundle_\(.*\)\.tar\.gz/\1/p')
fi

if [ -z "${AINIC_DRIVER_VERSION}" ]; then
  echo "Error: AINIC_DRIVER_VERSION not specified and could not be extracted from AINIC_BUNDLE_PATH"
  exit 1
fi

echo "============== begin to install AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} =============="
echo "AINIC Driver Version: ${AINIC_DRIVER_VERSION}"

cd ${WORKDIR}

# Install dependencies - add AMD AINIC pensando repository and install libionic-dev
echo "Adding AMD AINIC pensando repository for driver version ${AINIC_DRIVER_VERSION}..."

# Add repository with trusted=yes to bypass GPG signature verification
# This is consistent with using --allow-unauthenticated for apt-get install
echo "deb [arch=amd64 trusted=yes] https://repo.radeon.com/amdainic/pensando/ubuntu/${AINIC_DRIVER_VERSION} jammy main" \
    > /etc/apt/sources.list.d/amdainic-pensando.list

apt-get update >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "Warning: apt-get update had issues, continuing anyway..."
fi

echo "Installing libionic-dev=${LIBIONIC_VERSION}..."
apt-get install -y --allow-unauthenticated libionic-dev=${LIBIONIC_VERSION} >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "Error: Failed to install libionic-dev=${LIBIONIC_VERSION}."
  exit 1
fi

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
git checkout -q tags/${ANP_VERSION}
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
           ROCM_PATH=/opt/rocm \
           RCCL_HOME=/opt/rccl >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "Error: Failed to build AMD ANP driver."
  exit 1
fi

ANP_BUILD_DIR="${WORKDIR}/${ANP_DIR}/build"
if [ ! -f "${ANP_BUILD_DIR}/librccl-anp.so" ] && [ -f "${ANP_BUILD_DIR}/librccl-net.so" ]; then
  echo "Creating symlink: librccl-anp.so -> librccl-net.so"
  cd ${ANP_BUILD_DIR}
  ln -sf librccl-net.so librccl-anp.so
  if [ $? -eq 0 ]; then
    echo "Symlink created successfully."
  else
    echo "Warning: Failed to create symlink."
  fi
fi

echo "============== install  AMD AINIC Network Plugin (amd-anp) ${ANP_VERSION} successfully =============="
