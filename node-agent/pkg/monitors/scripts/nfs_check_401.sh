#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <nfs_path>"
  echo "Example: $0 /nfs"
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- df -h | grep -q "$1" > /dev/null
if [ $? -ne 0 ]; then
  echo "nfs($1) is not mount"
  exit 1
fi