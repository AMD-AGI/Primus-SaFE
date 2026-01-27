#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

CONTAINER_NAME="csi-wekafs-node"
output=$(nsenter --target 1 --mount --uts --ipc --net --pid -- crictl ps 2>/dev/null | grep "$CONTAINER_NAME")

if [ -z "$output" ]; then
    echo "No $CONTAINER_NAME containers found, skipping check"
    exit 1
fi

total=$(echo "$output" | wc -l)
if [ "$total" -ne 3 ]; then
    echo "Error: Expected 3 $CONTAINER_NAME containers, but found $total"
    exit 1
fi

running=$(echo "$output" | grep -c '\bRunning\b')

if [ "$running" -ne "$total" ]; then
    echo "Error: Not all $CONTAINER_NAME containers are running, Total: $total, Running: $running"
    exit 1
fi

echo "All $total $CONTAINER_NAME containers are running"
exit 0