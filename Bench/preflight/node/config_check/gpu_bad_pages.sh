#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/amd-smi > /dev/null
if [ $? -ne 0 ]; then
  exit 2
fi

msg=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/amd-smi bad-pages |grep "Address:"`
if [ $? -eq 0 ]; then
  echo "amd-smi bad-pages detected, $msg"
  exit 1
fi