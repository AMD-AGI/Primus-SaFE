#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

CMDLINE=`nsenter --target 1 --mount --uts --ipc --net --pid -- cat /proc/cmdline`
if [ $? -ne 0 ]; then
  exit 2
fi
for cmd in `echo $CMDLINE`
do
  if [[ "$cmd" == "pci=realloc=off" ]]; then
    exit 0
  fi
  if [[ "$cmd" == "iommu=pt" ]]; then
    exit 0
  fi
  if [[ "$cmd" == "amd_iommu=on" ]]; then
    exit 0
  fi
  if [[ "$cmd" == "intel_iommu=on" ]]; then
    exit 0
  fi
done

echo "iommu is not properly configured"
exit 1
