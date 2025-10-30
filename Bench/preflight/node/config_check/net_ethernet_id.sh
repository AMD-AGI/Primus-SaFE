#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

ifname=$NCCL_SOCKET_IFNAME

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/sbin/ifconfig > /dev/null
if [ $? -ne 0 ]; then
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/ifconfig |grep "$ifname" > /dev/null
if [ $? -ne 0 ]; then
  echo "no network interface found containing \"$ifname\""
  exit 1
fi