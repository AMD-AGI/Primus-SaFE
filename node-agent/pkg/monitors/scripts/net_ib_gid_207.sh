#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

nsenter --target 1 --mount --uts --ipc --net --pid -- ibv_devinfo -vv | grep -q "RoCE v2"
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