#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/amd-smi > /dev/null
if [ $? -ne 0 ]; then
  exit 2
fi

msg=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/amd-smi bad-pages |grep "Address:"`
if [ $? -eq 0 ]; then
  echo "Error: amd-smi bad-pages detected. $msg"
  exit 1
fi