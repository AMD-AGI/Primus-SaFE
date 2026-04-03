#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

host() { nsenter --target 1 --mount --uts --ipc --net --pid -- "$@"; }

host test -x /usr/bin/amd-smi > /dev/null 2>&1
if [ $? -ne 0 ]; then
  exit 2
fi

data=$(host /usr/bin/amd-smi monitor -e 2>&1)
if [ $? -ne 0 ]; then
  echo "Error: failed to execute amd-smi monitor -e. $data"
  exit 1
fi

# Parse column indices from header line
header=$(echo "$data" | head -1)
col_single=0
col_double=0
col_pcie=0
i=1
for field in $header; do
  case "$field" in
    SINGLE_ECC)  col_single=$i ;;
    DOUBLE_ECC)  col_double=$i ;;
    PCIE_REPLAY) col_pcie=$i ;;
  esac
  i=$((i + 1))
done

if [ $col_single -eq 0 ] || [ $col_double -eq 0 ] || [ $col_pcie -eq 0 ]; then
  echo "Error: failed to parse header columns. header: $header"
  exit 2
fi

total_single_ecc=0
total_double_ecc=0
total_pcie_replay=0

while read -r line; do
  if [[ "$line" =~ ^[0-9] ]]; then
    single_ecc=$(echo "$line" | awk -v c="$col_single" '{print $c}')
    double_ecc=$(echo "$line" | awk -v c="$col_double" '{print $c}')
    pcie_replay=$(echo "$line" | awk -v c="$col_pcie" '{print $c}')
    total_single_ecc=$((total_single_ecc + single_ecc))
    total_double_ecc=$((total_double_ecc + double_ecc))
    total_pcie_replay=$((total_pcie_replay + pcie_replay))
  fi
done <<< "$data"

if [[ $total_single_ecc -gt 10000 || $total_pcie_replay -gt 1000 || $total_double_ecc -gt 0 ]]; then
  echo "ECC error threshold exceeded: total_single_ecc: $total_single_ecc, total_double_errors: $total_double_ecc, total_pcie_replay: $total_pcie_replay"
  exit 1
fi
