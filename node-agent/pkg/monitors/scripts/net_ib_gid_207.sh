#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ -z "$1" ]; then
  echo "Usage: $0 \"device1,device2,...\""
  echo "Example: $0 \"bnxt_re0,bnxt_re1,bnxt_re2\""
  exit 1
fi

input_devices="$1"
first_device=$(echo "$input_devices" | cut -d',' -f1)
nsenter --target 1 --mount --uts --ipc --net --pid -- cat "/sys/class/infiniband/${first_device}/ports/1/gid_attrs/types/1" |grep -q "RoCE v2"
if [[ $? -ne 0 ]]; then
  echo "Error: $first_device is not RoCE v2 device"
  exit 2
fi

check_gid() {
    local gid_data=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/ibv_devinfo -vv | grep GID)
    while IFS= read -r line; do
        if [[ $line =~ GID\[([0-9]+)\] ]]; then
            gid_index=${BASH_REMATCH[1]}
            if (( gid_index < 0 || gid_index > 3 )); then
                echo "Error: Invalid GID index found: $gid_index (must be 0-3), line: $line" >&2
                return 1
            fi
        fi
    done <<< "$gid_data"
    return 0
}

# first check
if ! check_gid; then
    nsenter --target 1 --mount --uts --ipc --net --pid -- rmmod bnxt_re && sleep 5 && rmmod bnxt_en && sleep 2 && modprobe bnxt_re

    # second check
    if ! check_gid; then
        echo "Error: GID index still invalid after repair attempt" >&2
        exit 1
    fi
fi