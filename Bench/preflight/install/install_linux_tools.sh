#!/bin/bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install linux-tools =============="

apt-get update >/dev/null 2>&1

if [ "${OS_NAME}" = "oci" ]; then
  linux_tools="linux-cloud-tools-oracle"
  echo "Trying to install $linux_tools (OS_NAME=oci)..."
  apt install -y "$linux_tools" >/dev/null 2>&1
else
  KERNEL_VERSION=$(uname -r)
  linux_tools="linux-tools-${KERNEL_VERSION} linux-tools-common"
  echo "Trying to install $linux_tools..."
  apt install -y linux-tools-${KERNEL_VERSION} linux-tools-common >/dev/null 2>&1
fi

if [ $? -ne 0 ]; then
  echo "Failed to install $linux_tools"
  exit 1
fi

echo "============== $linux_tools installation completed =============="