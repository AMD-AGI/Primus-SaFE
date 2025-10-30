#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod |grep 'amdgpu ' > /dev/null
if [ $? -ne 0 ]; then
  echo "unable to find amdgpu module"
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/rocm-smi > /dev/null
if [ $? -ne 0 ]; then
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi > /tmp/rocm-smi
ret=$?
if [ $ret -ne 0 ]; then
  echo "failed to execute rocm-smi, $ret"
  exit 1
fi
exit 0
