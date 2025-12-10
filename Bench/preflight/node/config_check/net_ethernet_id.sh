#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

ifname=$NCCL_SOCKET_IFNAME

# Check if ifconfig exists, if not, use ip command
if nsenter --target 1 --mount --uts --ipc --net --pid -- which ifconfig > /dev/null 2>&1; then
  # Use ifconfig if available
  nsenter --target 1 --mount --uts --ipc --net --pid -- ifconfig | grep "$ifname" > /dev/null
  if [ $? -ne 0 ]; then
    echo "no network interface found containing \"$ifname\""
    exit 1
  fi
else
  # Fall back to ip command
  nsenter --target 1 --mount --uts --ipc --net --pid -- ip link show | grep "$ifname" > /dev/null
  if [ $? -ne 0 ]; then
    echo "no network interface found containing \"$ifname\""
    exit 1
  fi
fi