#!/bin/bash
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install linux-tools =============="

KERNEL_VERSION=$(uname -r)
linux_tools="linux-tools-${KERNEL_VERSION}"

# Check if already installed
if dpkg -l | grep -q "$linux_tools"; then
  echo "Linux tools already installed for kernel $KERNEL_VERSION"
  exit 0
fi

echo "Trying to install $linux_tools..."
apt-get update >/dev/null 2>&1

# Try to install exact version first
if apt-cache show "$linux_tools" >/dev/null 2>&1; then
  echo "Installing $linux_tools..."
  apt install -y "$linux_tools" linux-tools-common >/dev/null 2>&1
  if [ $? -eq 0 ]; then
    echo "Linux tools installed successfully"
    exit 0
  fi
fi  

echo "============== linux-tools installation completed =============="
