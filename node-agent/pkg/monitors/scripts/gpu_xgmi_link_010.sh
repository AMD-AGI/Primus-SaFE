#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

msg=`nsenter --target 1 --mount --uts --ipc --net --pid -- dmesg | grep -i xgmi |grep "link error"`
if [ $? -eq 0 ]; then
  echo "Error: $msg"
  exit 1
fi