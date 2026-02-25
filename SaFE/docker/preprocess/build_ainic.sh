#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Check if AINIC_DRIVER_VERSION is specified
if [ -z "${AINIC_DRIVER_VERSION}" ]; then
  echo "AINIC_DRIVER_VERSION not specified, skipping AINIC installation."
  exit 0
fi

echo "============== begin to install AMD AINIC (version: ${AINIC_DRIVER_VERSION}) =============="
set -e

# ---------------------------------------------------------------------------
# Version mapping: AINIC_DRIVER_VERSION -> (AMD_ANP_VERSION, ROCM_VERSION)
# ---------------------------------------------------------------------------
set_versions_from_driver() {
  _driver_version="$1"
  case "${_driver_version}" in
    1.117.5-a-56)
      AMD_ANP_VERSION="v1.3.0"
      ROCM_VERSION="7.1.0"
      LIBIONIC_VERSION="54.0-184"
      ;;
    *)
      echo "Error: Unknown AINIC driver version ${_driver_version}."
      echo "Please add version mapping in build_ainic.sh"
      exit 1
      ;;
  esac
  unset _driver_version
}

set_versions_from_driver "${AINIC_DRIVER_VERSION}"
echo "Mapped AINIC driver version ${AINIC_DRIVER_VERSION} -> ANP: ${AMD_ANP_VERSION}, ROCM: ${ROCM_VERSION}, LIBIONIC: ${LIBIONIC_VERSION}"

# Search for matching driver file in /shared-data/drivers/
DRIVERS_DIR="/shared-data/drivers"
if [ ! -d "${DRIVERS_DIR}" ]; then
  echo "Error: Drivers directory ${DRIVERS_DIR} does not exist."
  exit 1
fi

# Find tarball matching the driver version
PATH_TO_AINIC_TAR_PACKAGE=$(ls ${DRIVERS_DIR}/*${AINIC_DRIVER_VERSION}*.tar.gz 2>/dev/null | head -n 1)
if [ -z "${PATH_TO_AINIC_TAR_PACKAGE}" ] || [ ! -f "${PATH_TO_AINIC_TAR_PACKAGE}" ]; then
  echo "Error: No AINIC driver tarball found matching version ${AINIC_DRIVER_VERSION} in ${DRIVERS_DIR}"
  echo "Available files:"
  ls -la ${DRIVERS_DIR}/ 2>/dev/null || echo "  (directory empty or not accessible)"
  exit 1
fi
echo "Found AINIC driver tarball: ${PATH_TO_AINIC_TAR_PACKAGE}"

. /shared-data/utils.sh
install_if_not_exists dpkg-dev kmod xz-utils libfmt-dev libboost-all-dev ibibverbs-dev ibverbs-utils infiniband-diags

# Call build_anp.sh with required parameters
export AMD_ANP_VERSION=${AMD_ANP_VERSION}
export ROCM_VERSION=${ROCM_VERSION}
export AINIC_DRIVER_VERSION=${AINIC_DRIVER_VERSION}
export LIBIONIC_VERSION=${LIBIONIC_VERSION}
/bin/sh /shared-data/build_anp.sh

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

echo "============== install AMD AINIC ${AINIC_DRIVER_VERSION} successfully =============="
