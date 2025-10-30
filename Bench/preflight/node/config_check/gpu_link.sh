#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ ! -f "/tmp/rocm-smi" ]; then
  exit 0
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi --showtopoaccess |grep -i false >/dev/null
if [ $? -eq 0 ]; then
  echo "ink error between GPUs is found"
  exit 1
fi