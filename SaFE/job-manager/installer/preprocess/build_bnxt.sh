#!/bin/sh

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Function to check and install packages if not already installed
install_if_not_exists() {
    missing_packages=""

    # Check each package if it's installed
    for package in "$@"; do
        if ! dpkg -l | grep -q "^ii  $package "; then
            missing_packages="$missing_packages $package"
        fi
    done

    # Install only missing packages
    if [ -n "$missing_packages" ]; then
        echo "Installing missing packages:$missing_packages"
        apt update >/dev/null
        apt-get install -y $missing_packages >/dev/null
    fi
}

# Rebuild bnxt
if [ -f "${PATH_TO_BNXT_TAR_PACKAGE}" ]; then
  install_if_not_exists libibverbs-dev ibverbs-utils infiniband-diags rdma-core librdmacm-dev libibverbs-dev libibumad-dev
  echo "Rebuild bnxt from $PATH_TO_BNXT_TAR_PACKAGE ..." && \
  tar xzf "${PATH_TO_BNXT_TAR_PACKAGE}" -C /tmp/ && \
  mv /tmp/libbnxt_re-* /tmp/libbnxt && \
  mv /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so.inbox && \
  cd /tmp/libbnxt/ && sh ./autogen.sh && ./configure && \
  make -C /tmp/libbnxt clean all install && \
  echo '/usr/local/lib' > /etc/ld.so.conf.d/libbnxt_re.conf && \
  ldconfig && \
  cp -f /tmp/libbnxt/bnxt_re.driver /etc/libibverbs.d/ && \
  cd "${PRIMUS_PATH}" && \
  echo "Rebuild libbnxt done."
else
  echo "Skip bnxt rebuild. PATH_TO_BNXT_TAR_PACKAGE=$PATH_TO_BNXT_TAR_PACKAGE"
fi