#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Rebuild bnxt
# Supports two input methods (priority: PATH_TO_BNXT_TAR_PACKAGE > BNXT_DRIVER_VERSION):
#   1. PATH_TO_BNXT_TAR_PACKAGE - direct path to tarball
#   2. BNXT_DRIVER_VERSION - version string to search in /shared-data/drivers/

# Determine the tarball path
BNXT_TARBALL=""

# Priority 1: Use PATH_TO_BNXT_TAR_PACKAGE if specified and exists
if [ -n "${PATH_TO_BNXT_TAR_PACKAGE}" ] && [ -f "${PATH_TO_BNXT_TAR_PACKAGE}" ]; then
  BNXT_TARBALL="${PATH_TO_BNXT_TAR_PACKAGE}"
  echo "Using specified PATH_TO_BNXT_TAR_PACKAGE: ${BNXT_TARBALL}"
# Priority 2: Search by BNXT_DRIVER_VERSION in /shared-data/drivers/
elif [ -n "${BNXT_DRIVER_VERSION}" ]; then
  DRIVERS_DIR="/shared-data/drivers"
  if [ -d "${DRIVERS_DIR}" ]; then
    BNXT_TARBALL=$(ls ${DRIVERS_DIR}/*${BNXT_DRIVER_VERSION}*.tar.gz 2>/dev/null | head -n 1)
    if [ -n "${BNXT_TARBALL}" ] && [ -f "${BNXT_TARBALL}" ]; then
      echo "Found bnxt driver tarball by version ${BNXT_DRIVER_VERSION}: ${BNXT_TARBALL}"
    else
      echo "Error: No bnxt driver tarball found matching version ${BNXT_DRIVER_VERSION} in ${DRIVERS_DIR}"
      echo "Available files:"
      ls -la ${DRIVERS_DIR}/ 2>/dev/null || echo "  (directory empty or not accessible)"
      exit 1
    fi
  else
    echo "Error: Drivers directory ${DRIVERS_DIR} does not exist."
    exit 1
  fi
fi

# Build bnxt if tarball is found
if [ -n "${BNXT_TARBALL}" ] && [ -f "${BNXT_TARBALL}" ]; then
  echo "============== begin to rebuild bnxt from ${BNXT_TARBALL} =============="
  set -e

  . /shared-data/utils.sh
  install_if_not_exists libibverbs-dev ibverbs-utils infiniband-diags rdma-core librdmacm-dev libibverbs-dev libibumad-dev

  tar xzf "${BNXT_TARBALL}" -C /tmp/
  mv /tmp/libbnxt_re-* /tmp/libbnxt
  mv /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so.inbox

  cd /tmp/libbnxt/
  sh ./autogen.sh
  ./configure
  make -C /tmp/libbnxt clean all install

  echo '/usr/local/lib' > /etc/ld.so.conf.d/libbnxt_re.conf
  ldconfig
  cp -f /tmp/libbnxt/bnxt_re.driver /etc/libibverbs.d/

  cd "${PRIMUS_PATH}"
  echo "============== rebuild libbnxt done =============="
else
  echo "Skip bnxt rebuild. Neither PATH_TO_BNXT_TAR_PACKAGE nor BNXT_DRIVER_VERSION specified."
fi