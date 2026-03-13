#!/bin/bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install linux-tools =============="

apt-get update >/dev/null 2>&1

KERNEL_VERSION=$(uname -r)

if [ "${OS_NAME}" = "oci" ]; then
  # e.g. 6.8.0-1039-oracle -> 6.8 for linux-tools-oracle-6.8
  VERSION_PREFIX="${KERNEL_VERSION%%-*}"
  VERSION_MAJOR_MINOR=$(echo "$VERSION_PREFIX" | cut -d'.' -f1-2)
  linux_tools="linux-tools-oracle-${VERSION_MAJOR_MINOR} linux-tools-common"
  echo "Trying to install $linux_tools (OS_NAME=oci, kernel ${KERNEL_VERSION})..."
  apt install -y linux-tools-oracle-${VERSION_MAJOR_MINOR} linux-tools-common >/dev/null 2>&1
else
  linux_tools="linux-tools-${KERNEL_VERSION} linux-tools-common"
  echo "Trying to install $linux_tools..."
  apt install -y linux-tools-${KERNEL_VERSION} linux-tools-common >/dev/null 2>&1
fi

if [ $? -ne 0 ]; then
  echo "Failed to install $linux_tools"
  exit 1
fi

echo "============== $linux_tools installation completed =============="