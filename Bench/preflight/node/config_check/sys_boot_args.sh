#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
# Required flags
FOUND_PCI_REALLOC=0
FOUND_IOMMU_PT=0
# Optional flags (at least one required)
FOUND_AMD_IOMMU=0
FOUND_INTEL_IOMMU=0

for cmd in `echo $CMDLINE`
do
  if [[ "$cmd" == "pci=realloc=off" ]]; then
    FOUND_PCI_REALLOC=1
  fi
  if [[ "$cmd" == "iommu=pt" ]]; then
    FOUND_IOMMU_PT=1
  fi
  if [[ "$cmd" == "amd_iommu=on" ]]; then
    FOUND_AMD_IOMMU=1
  fi
  if [[ "$cmd" == "intel_iommu=on" ]]; then
    FOUND_INTEL_IOMMU=1
  fi
done

# Check required: pci=realloc=off
if [ $FOUND_PCI_REALLOC -ne 1 ]; then
  echo "pci=realloc=off is not configured in kernel boot args" >&2
  exit 1
fi

# Check optional: amd_iommu=on OR intel_iommu=on OR iommu=pt (at least one)
if [ $FOUND_AMD_IOMMU -ne 1 ] && [ $FOUND_INTEL_IOMMU -ne 1 ] && [ $FOUND_IOMMU_PT -ne 1 ]; then
  echo "Neither amd_iommu=on nor intel_iommu=on nor iommu=pt is configured in kernel boot args" >&2
  exit 1
fi

exit 0
