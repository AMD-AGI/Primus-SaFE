#!/bin/bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install linux-tools =============="

if [ "${OS_NAME}" = "oci" ]; then
  echo "Skipping linux-tools installation (OS_NAME=oci)"
  echo "============== linux-tools skipped (OCI) =============="
  exit 0
fi

apt-get update >/dev/null 2>&1

KERNEL_VERSION=$(uname -r)
linux_tools="linux-tools-${KERNEL_VERSION} linux-tools-common"
echo "Trying to install $linux_tools..."
apt install -y $linux_tools >/dev/null 2>&1

if [ $? -ne 0 ]; then
  echo "Failed to install $linux_tools"
  exit 1
fi

echo "============== $linux_tools installation completed =============="