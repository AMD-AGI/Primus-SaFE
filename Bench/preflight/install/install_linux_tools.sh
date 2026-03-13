#!/bin/bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install linux-tools =============="

apt-get update >/dev/null 2>&1

KERNEL_VERSION=$(uname -r)

if [ "${OS_NAME}" = "oci" ]; then
  # e.g. 5.15.0-1074 or 6.8.0-1039-oracle -> linux-tools-5.15.0-1074-oracle
  KERNEL_SUFFIX="${KERNEL_VERSION}"
  [[ "$KERNEL_VERSION" != *-oracle ]] && KERNEL_SUFFIX="${KERNEL_VERSION}-oracle"
  linux_tools="linux-tools-${KERNEL_SUFFIX} linux-cloud-tools-${KERNEL_SUFFIX} linux-tools-common"
  echo "Trying to install $linux_tools (OS_NAME=oci, kernel ${KERNEL_VERSION})..."
  if ! apt install -y linux-tools-${KERNEL_SUFFIX} linux-cloud-tools-${KERNEL_SUFFIX} linux-tools-common >/dev/null 2>&1; then
    echo "Kernel-specific package failed, trying generic linux-tools-oracle..."
    linux_tools="linux-tools-oracle linux-cloud-tools-oracle linux-tools-common"
    apt install -y linux-tools-oracle linux-cloud-tools-oracle linux-tools-common >/dev/null 2>&1
  fi
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