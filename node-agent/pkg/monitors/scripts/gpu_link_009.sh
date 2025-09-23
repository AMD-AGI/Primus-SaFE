#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ ! -f "/tmp/rocm-smi" ]; then
  exit 0
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi --showtopoaccess |grep -i false >/dev/null
if [ $? -eq 0 ]; then
  echo "Error: There is a link error between two GPUs"
  exit 1
fi