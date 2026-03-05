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
if [ "${NPROC}" -gt 16 ] 2>/dev/null; then
  NPROC=16
fi
echo "Using ${NPROC} parallel jobs for compilation"

# Get GPU architecture (gfx942, gfx950, etc.) via rocm-smi or rocminfo. Exits with error if not found.
get_gfx_arch() {
  _arch=""
  if rocm-smi --showhw 2>/dev/null | grep -qE 'gfx[0-9]{3,4}'; then
    _arch=$(rocm-smi --showhw 2>/dev/null | grep -oE 'gfx[0-9]{3,4}' | head -1)
  fi
  if [ -z "${_arch}" ] && rocminfo 2>/dev/null | grep -qE 'gfx[0-9]{3,4}'; then
    _arch=$(rocminfo 2>/dev/null | grep -oE 'gfx[0-9]{3,4}' | head -1)
  fi
  if [ -z "${_arch}" ]; then
    echo "Error: Could not detect GPU arch via rocm-smi or rocminfo." >&2
    exit 1
  fi
  echo "${_arch}"
}

WORKDIR="/opt"

# ---------------------------------------------------------------------------
# Build rccl
# ---------------------------------------------------------------------------
# RCCL tag and build flags by ROCm version (aligned with Bench/pytorch/install_rccl.sh)
RCCL_FLAGS="--disable-mscclpp --disable-msccl-kernel"
case "${ROCM_VERSION}" in
  6.4.2|6.4.3|6.4.4)
    RCCL_TAG="rocm-6.4.3"
    ;;
  7.0.0|7.0.1|7.0.2|7.1.0|7.1.1)
    RCCL_TAG="rocm-7.1.0"
    RCCL_FLAGS="--disable-msccl-kernel"
    ;;
  7.2.0)
    RCCL_TAG="rocm-7.2.0"
    RCCL_FLAGS=""
    ;;
  *)
    echo "Error: Unsupported ROCM_VERSION '${ROCM_VERSION}'. Supported: 6.4.3, 7.0.0, 7.0.1, 7.0.2, 7.1.0, 7.1.1, 7.2.0"
    exit 1
    ;;
esac

# Increase git buffer for submodule clone (avoids "RPC failed; curl 56" / "early EOF")
git config --global http.postBuffer 524288000

cd ${WORKDIR}
if [ ! -d "${WORKDIR}/rccl" ]; then
  # Use RCCL from /shared-data: /shared-data/apps/rccl/${RCCL_TAG} (e.g. rocm-7.1.0)
  RCCL_SRC=""
  if [ -d "/shared-data/apps/rccl/${RCCL_TAG}" ]; then
    RCCL_SRC="/shared-data/apps/rccl/${RCCL_TAG}"
  fi

  if [ -n "${RCCL_SRC}" ]; then
    echo "Using RCCL from /shared-data/apps/rccl"
    cp -r "${RCCL_SRC}" "${WORKDIR}/rccl"
  else
    echo "Cloning RCCL (${RCCL_TAG})..."
    git config --global advice.detachedHead false
    _clone_retries=3
    _clone_attempt=1
    while [ $_clone_attempt -le $_clone_retries ]; do
      if git clone -q --branch "${RCCL_TAG}" --depth 1 https://github.com/ROCm/rccl.git; then
        break
      fi
      rm -rf rccl
      if [ $_clone_attempt -eq $_clone_retries ]; then
        echo "Error: Failed to clone RCCL after $_clone_retries attempts."
        exit 1
      fi
      echo "Retrying in 5 seconds..."
      sleep 5
      _clone_attempt=$((_clone_attempt + 1))
    done
  fi

  # Build RCCL (skip only if build/release exists and GPU_TARGETS matches current GPU arch)
  cd ${WORKDIR}/rccl
  GFX_ARCH=$(get_gfx_arch)
  echo "Detected GPU arch: ${GFX_ARCH}"

  RCCL_BUILD_DIR="${WORKDIR}/rccl/build/release"
  CMAKE_CACHE="${RCCL_BUILD_DIR}/CMakeCache.txt"
  if [ -d "${RCCL_BUILD_DIR}" ] && [ -f "${CMAKE_CACHE}" ] && grep -q "GPU_TARGETS.*${GFX_ARCH}" "${CMAKE_CACHE}" 2>/dev/null; then
    echo "RCCL build/release exists and GPU_TARGETS matches ${GFX_ARCH}, skipping build."
  else
    export CMAKE_POLICY_VERSION_MINIMUM=3.5
    _start=$(date +%s)
    if ! ./install.sh -j ${NPROC} -l --prefix build/ ${RCCL_FLAGS} --amdgpu_targets="${GFX_ARCH}" >/dev/null 2>&1; then
      echo "Error: Failed to build RCCL."
      exit 1
    fi
    _end=$(date +%s)
    echo "RCCL install.sh completed in $((_end - _start)) seconds"
  fi
fi
export RCCL_HOME=${WORKDIR}/rccl
echo "Install RCCL (${RCCL_TAG}) successfully, RCCL_HOME: ${RCCL_HOME}"


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

# Get AMD ANP from /shared-data: /shared-data/apps/amd-anp/${AMD_ANP_VERSION} (e.g. v1.3.0)
ANP_SRC=""
if [ -d "/shared-data/apps/amd-anp/${AMD_ANP_VERSION}" ]; then
  ANP_SRC="/shared-data/apps/amd-anp/${AMD_ANP_VERSION}"
fi

if [ -n "${ANP_SRC}" ]; then
  echo "Using AMD ANP from /shared-data/apps/amd-anp"
  cp -r "${ANP_SRC}" "${WORKDIR}/amd-anp"
else
  echo "Cloning AMD ANP repository..."
  _anp_retries=3
  _anp_attempt=1
  while [ $_anp_attempt -le $_anp_retries ]; do
    if git clone -q https://github.com/rocm/amd-anp.git; then
      break
    fi
    rm -rf amd-anp
    if [ $_anp_attempt -eq $_anp_retries ]; then
      echo "Error: Failed to clone AMD ANP repository after $_anp_retries attempts."
      exit 1
    fi
    echo "Retrying in 5 seconds..."
    sleep 5
    _anp_attempt=$((_anp_attempt + 1))
  done

  cd amd-anp
  echo "Checking out version amd-anp-${AMD_ANP_VERSION}..."
  if ! git checkout -q tags/${AMD_ANP_VERSION}; then
    echo "Error: Failed to checkout tag ${AMD_ANP_VERSION}."
    exit 1
  fi
  cd ${WORKDIR}
fi

cd amd-anp
# Prebuilt: has build/librccl-anp.so or build/librccl-net.so (skip build when no GPU)
if [ -f "./build/librccl-anp.so" ] || [ -f "./build/librccl-net.so" ]; then
  echo "AMD ANP prebuilt found in shared-data, skipping build."
else
  GFX_ARCH=$(get_gfx_arch)
  if ! grep -q "offload-arch=${GFX_ARCH}" ./Makefile 2>/dev/null; then
    sed -i "5a CFLAGS += --offload-arch=${GFX_ARCH}" ./Makefile
  fi
  echo "Building AMD ANP driver..."
  if ! make -j ${NPROC} ROCM_PATH=/opt/rocm RCCL_HOME=${RCCL_HOME} >/dev/null 2>&1; then
    echo "Error: Failed to build AMD ANP driver."
    exit 1
  fi
fi

# Create symlink librccl-anp.so -> librccl-net.so if needed (RCCL looks for librccl-anp.so)
ANP_BUILD_DIR="${WORKDIR}/${ANP_DIR}/build"
cd ${ANP_BUILD_DIR}
if [ -f "librccl-anp.so" ] && [ ! -f "librccl-net.so" ]; then
  echo "Creating symlink: librccl-net.so -> librccl-anp.so"
  ln -sf librccl-anp.so librccl-net.so
elif [ -f "librccl-net.so" ] && [ ! -f "librccl-anp.so" ]; then
  echo "Creating symlink: librccl-anp.so -> librccl-net.so"
  ln -sf librccl-net.so librccl-anp.so
fi
