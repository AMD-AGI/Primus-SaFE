#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid --  grep -q "[always]" /sys/kernel/mm/transparent_hugepage/enabled
if [ $? -ne 0 ]; then
  echo "transparent_hugepage is not enabled"
  exit 1
fi
