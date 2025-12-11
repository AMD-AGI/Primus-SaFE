#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Try to get cmdline from nsenter first (for container environments)
if command -v nsenter &> /dev/null; then
  CMDLINE=`nsenter --target 1 --mount --uts --ipc --net --pid -- cat /proc/cmdline 2>/dev/null`
  if [ $? -ne 0 ]; then
    # Fall back to direct read if nsenter fails
    CMDLINE=`cat /proc/cmdline 2>/dev/null`
    if [ $? -ne 0 ]; then
      exit 2
    fi
  fi
else
  # Direct read if nsenter is not available
  CMDLINE=`cat /proc/cmdline`
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
