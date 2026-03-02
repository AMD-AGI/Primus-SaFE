#!/bin/sh

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

# Get container CPU limit (cgroups v1/v2) or fall back to nproc
get_cpu_count() {
  # Try cgroups v2 first
  if [ -f /sys/fs/cgroup/cpu.max ]; then
    cpu_quota=$(cut -d' ' -f1 /sys/fs/cgroup/cpu.max)
    cpu_period=$(cut -d' ' -f2 /sys/fs/cgroup/cpu.max)
    if [ "$cpu_quota" != "max" ] && [ -n "$cpu_period" ]; then
      echo $(( cpu_quota / cpu_period ))
      return
    fi
  fi
  # Try cgroups v1
  if [ -f /sys/fs/cgroup/cpu/cpu.cfs_quota_us ] && [ -f /sys/fs/cgroup/cpu/cpu.cfs_period_us ]; then
    cpu_quota=$(cat /sys/fs/cgroup/cpu/cpu.cfs_quota_us)
    cpu_period=$(cat /sys/fs/cgroup/cpu/cpu.cfs_period_us)
    if [ "$cpu_quota" -gt 0 ] && [ -n "$cpu_period" ]; then
      echo $(( cpu_quota / cpu_period ))
      return
    fi
  fi
  # Fall back to nproc or default
  nproc 2>/dev/null || echo 16
}
NPROC=$(get_cpu_count)
echo "Using ${NPROC} parallel jobs for compilation"

WORKDIR="/opt"

# ---------------------------------------------------------------------------
# Build rccl
# ---------------------------------------------------------------------------
cd ${WORKDIR}
if [ ! -d "${WORKDIR}/rccl" ]; then
  echo "Cloning and building RCCL (rocm-${ROCM_VERSION})..."
  git clone -q https://github.com/ROCm/rccl.git
  cd rccl
  git checkout -q rocm-${ROCM_VERSION}
  if ! ./install.sh -j ${NPROC} -l --prefix build/ --disable-msccl-kernel --amdgpu_targets="gfx950" >/dev/null 2>&1; then
    echo "Error: Failed to build RCCL."
    exit 1
  fi
fi
export RCCL_HOME=${WORKDIR}/rccl
echo "Install RCCL (rocm-${ROCM_VERSION}) successfully, RCCL_HOME: ${RCCL_HOME}"


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
git clone -q https://github.com/rocm/amd-anp.git
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone AMD ANP repository."
  exit 1
fi

cd amd-anp
# Checkout specific version or branch
echo "Checking out version amd-anp-${AMD_ANP_VERSION}..."
if ! git checkout -q tags/${AMD_ANP_VERSION}; then
  echo "Error: Failed to checkout tag ${AMD_ANP_VERSION}."
  exit 1
fi

sed -i '5a CFLAGS += --offload-arch=gfx950' ./Makefile
echo "Building AMD ANP driver..."

if [ -d "/opt/openmpi" ]; then
  make -j ${NPROC} MPI_INCLUDE=/opt/openmpi/include/ \
                   MPI_LIB_PATH=/opt/openmpi/lib/ \
                   ROCM_PATH=/opt/rocm \
                   RCCL_HOME=${RCCL_HOME} >/dev/null 2>&1
else
  make -j ${NPROC} ROCM_PATH=/opt/rocm RCCL_HOME=${RCCL_HOME} >/dev/null 2>&1
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

echo "============== install AMD AINIC Network Plugin (amd-anp) ${AMD_ANP_VERSION} successfully =============="