#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install AMD AINIC Driver =============="
set -e

WORKDIR="/opt"

cd ${WORKDIR}

# Check if AINIC_BUNDLE_PATH is set
if [ -z "${AINIC_BUNDLE_PATH}" ]; then
  echo "Error: AINIC_BUNDLE_PATH environment variable is not set"
  exit 1
fi

# Check if source tarball exists
if [ ! -f "${AINIC_BUNDLE_PATH}" ]; then
  echo "Error: AINIC bundle not found at ${AINIC_BUNDLE_PATH}"
  exit 1
fi

# Extract tarball name and directory name from full path
AINIC_TARBALL=$(basename "${AINIC_BUNDLE_PATH}")
AINIC_DIR="${AINIC_TARBALL%.tar.gz}"

cp ${AINIC_BUNDLE_PATH} ${WORKDIR}/
if [ $? -ne 0 ]; then
  echo "Error: Failed to copy AINIC bundle"
  exit 1
fi

# Extract AINIC bundle
tar zxf ${AINIC_TARBALL}
if [ $? -ne 0 ]; then
  echo "Error: Failed to extract ${AINIC_TARBALL}"
  exit 1
fi

# Extract host software package
cd ${AINIC_DIR}
tar zxf host_sw_pkg.tar.gz
if [ $? -ne 0 ]; then
  echo "Error: Failed to extract host_sw_pkg.tar.gz"
  exit 1
fi

# Run installation script
echo "Running AINIC installation script..."
cd host_sw_pkg

./install.sh --domain=user -y
if [ $? -ne 0 ]; then
  echo "Error: Failed to install AINIC driver."
  exit 1
fi

# Verify ionic_rdma module is available (only on host, not in Docker)
if [ ! -f /.dockerenv ] && ! grep -q docker /proc/1/cgroup 2>/dev/null; then
  echo "Verifying ionic_rdma kernel module..."
  if modinfo ionic_rdma &>/dev/null; then
    echo "ionic_rdma module installed successfully"
    # Load the module
    modprobe ionic_rdma || true
  else
    echo "Warning: ionic_rdma module not found after installation"
  fi
fi

# Cleanup
echo "Cleaning up temporary files..."
cd ${WORKDIR}
rm -f ${AINIC_TARBALL}
rm -rf ${AINIC_DIR}

echo "============== install AMD AINIC Driver ${AINIC_DIR} successfully =============="
