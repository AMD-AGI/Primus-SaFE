#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# This script is called by build_ainic.sh with AMD_ANP_VERSION and ROCM_VERSION
#

set -e

# Check required parameters
if [ -z "${AMD_ANP_VERSION}" ]; then
  echo "Error: AMD_ANP_VERSION not specified."
  exit 1
fi

if [ -z "${ROCM_VERSION}" ]; then
  echo "Error: ROCM_VERSION not specified."
  exit 1
fi

if [ -z "${AINIC_DRIVER_VERSION}" ]; then
  echo "Error: AINIC_DRIVER_VERSION not specified."
  exit 1
fi

if [ -z "${LIBIONIC_VERSION}" ]; then
  echo "Error: LIBIONIC_VERSION not specified."
  exit 1
fi

echo "============== begin to install AMD AINIC Network Plugin (amd-anp) ${AMD_ANP_VERSION} =============="
echo "Using ROCM version: ${ROCM_VERSION}, libionic version: ${LIBIONIC_VERSION}"

WORKDIR="/opt"

# ---------------------------------------------------------------------------
# Build rccl
# ---------------------------------------------------------------------------
cd ${WORKDIR}
if [ ! -d "${WORKDIR}/rccl" ]; then
  echo "Cloning and building RCCL (rocm-${ROCM_VERSION})..."
  git clone https://github.com/ROCm/rccl.git
  cd rccl
  git checkout rocm-${ROCM_VERSION}
  if ! ./install.sh -l --prefix build/ --disable-msccl-kernel --amdgpu_targets="gfx950" 2>&1; then
    echo "Error: Failed to build RCCL."
    exit 1
  fi
fi
export RCCL_HOME=${WORKDIR}/rccl


# ---------------------------------------------------------------------------
# Build AMD ANP
# ---------------------------------------------------------------------------

ANP_DIR="amd-anp"
# Check if amd-anp directory already exists
if [ -d "${WORKDIR}/${ANP_DIR}" ]; then
  exit 0
fi

cd ${WORKDIR}
# Install dependencies - add AMD AINIC pensando repository and install libionic-dev
echo "Adding AMD AINIC pensando repository for driver version ${AINIC_DRIVER_VERSION}..."

mkdir -p /etc/apt/keyrings
if [ ! -f /etc/apt/keyrings/rocm.gpg ]; then
  wget -qO- https://repo.radeon.com/rocm/rocm.gpg.key | gpg --dearmor -o /etc/apt/keyrings/rocm.gpg
  if [ $? -ne 0 ]; then
    echo "Error: Failed to add ROCm GPG key."
    exit 1
  fi
fi

echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/amdainic/pensando/ubuntu/${AINIC_DRIVER_VERSION} jammy main" \
    > /etc/apt/sources.list.d/amdainic-pensando.list

apt-get update
if [ $? -ne 0 ]; then
  echo "Error: Failed to update apt repositories."
  exit 1
fi

echo "Installing libionic-dev=${LIBIONIC_VERSION}..."
apt-get install -y --allow-unauthenticated libionic-dev=${LIBIONIC_VERSION}
if [ $? -ne 0 ]; then
  echo "Error: Failed to install libionic-dev=${LIBIONIC_VERSION}."
  exit 1
fi

# Clone AMD ANP repository
git clone https://github.com/rocm/amd-anp.git
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone AMD ANP repository."
  exit 1
fi

cd amd-anp
# Checkout specific version or branch
echo "Checking out version amd-anp-${AMD_ANP_VERSION}..."
if ! git checkout tags/${AMD_ANP_VERSION}; then
  echo "Error: Failed to checkout tag ${AMD_ANP_VERSION}."
  exit 1
fi

sed -i '5a CFLAGS += --offload-arch=gfx950' ./Makefile
echo "Building AMD ANP driver..."
if [ -d "/opt/openmpi" ]; then
  make -j 16 MPI_INCLUDE=/opt/openmpi/include/ \
             MPI_LIB_PATH=/opt/openmpi/lib/ \
             ROCM_PATH=/opt/rocm \
             RCCL_HOME=${RCCL_HOME}
else
  make -j 16 ROCM_PATH=/opt/rocm RCCL_HOME=${RCCL_HOME}
fi
if [ $? -ne 0 ]; then
  echo "Error: Failed to build AMD ANP driver."
  exit 1
fi

# Create symlink librccl-anp.so -> librccl-net.so if needed (RCCL looks for librccl-anp.so)
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