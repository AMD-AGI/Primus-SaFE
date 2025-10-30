#!/bin/bash

# check_ibv_device.sh
# Purpose: Check if comma-separated RDMA device names exist in ibv_devices output.
# Exit: 0 if all devices are valid, 1 otherwise.

input_devices=$NCCL_IB_HCA

# Get list of available RDMA devices (skip header lines)
available_devices=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/ibv_devices 2>/dev/null | awk 'NR > 2 {print $1}')
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
    echo "device '$dev' is not found in ibv_devices"
    exit 1
  fi
done