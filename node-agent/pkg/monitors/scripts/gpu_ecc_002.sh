#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/amd-smi > /dev/null
if [ $? -ne 0 ]; then
    exit 2
fi

data=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/amd-smi monitor -e`
total_single_ecc=0
total_double_ecc=0
total_pcie_replay=0

while read -r line; do
  if [[ "$line" =~ ^[0-9] ]]; then
    single_ecc=$(echo "$line" | awk '{print $2}')
    double_ecc=$(echo "$line" | awk '{print $3}')
    pcie_replay=$(echo "$line" | awk '{print $4}')
    total_single_ecc=$((total_single_ecc + single_ecc))
    total_double_ecc=$((total_double_ecc + double_ecc))
    total_pcie_replay=$((total_pcie_replay + pcie_replay))
    if [[ $total_single_ecc -gt 64 || $total_pcie_replay -gt 64 || $total_double_ecc -gt 1 ]]; then
        echo "ECC error threshold exceeded: total_single_ecc: $total_single_ecc, total_double_errors: $total_double_ecc, total_pcie_replay: $total_pcie_replay"
        exit 1
    fi
  fi
done <<< "$data"