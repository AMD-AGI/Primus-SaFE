#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Check if source tarball exists
if [ ! -f "${PATH_TO_AINIC_TAR_PACKAGE}" ]; then
  exit 0
fi

echo "============== begin to install AMD AINIC Driver =============="
set -e

. /shared-data/utils.sh
# install_if_not_exists libibverbs-dev ibverbs-utils infiniband-diags rdma-core librdmacm-dev libibverbs-dev libibumad-dev
#export AMD_ANP_VERSION=$AMD_ANP_VERSION
#bash /shared-data/build_anp.sh

WORKDIR="/opt"
cd ${WORKDIR}

# Extract tarball name and directory name from full path
AINIC_TARBALL=$(basename "${PATH_TO_AINIC_TAR_PACKAGE}")
AINIC_DIR="${AINIC_TARBALL%.tar.gz}"

cp ${PATH_TO_AINIC_TAR_PACKAGE} ${WORKDIR}/
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

# Cleanup
echo "Cleaning up temporary files..."
cd ${WORKDIR}
rm -f ${AINIC_TARBALL}
rm -rf ${AINIC_DIR}

echo "============== install AMD AINIC Driver ${AINIC_DIR} successfully =============="
