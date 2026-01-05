#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# check_ibv_device.sh
# Purpose: Check if comma-separated RDMA device names exist in ibv_devices output.
# Exit: 0 if all devices are valid, 1 otherwise.

set -o pipefail

if [ -z "$1" ]; then
  echo "Usage: $0 \"device1,device2,...\""
  echo "Example: $0 \"bnxt_re0,bnxt_re1,bnxt_re2\""
  exit 1
fi

input_devices="$1"

# Get list of available RDMA devices (skip header lines)
available_devices=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/ibv_devices | awk 'NR > 2 {print $1}')
readarray -t avail_array <<< "$available_devices"

# Split input by comma
IFS=',' read -ra dev_array <<< "$input_devices"

# Check each device
for dev in "${dev_array[@]}"; do
  dev=$(echo "$dev" | xargs)  # Trim whitespace
  if [ -z "$dev" ]; then
    continue
  fi

  # Use exact line match to avoid partial/substring match
  if ! printf '%s\n' "${avail_array[@]}" | grep -qx "$dev"; then
    echo "Error: Device '$dev' is not found in ibv_devices."
    exit 1
  fi
done