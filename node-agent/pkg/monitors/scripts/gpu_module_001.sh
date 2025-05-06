#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod |grep 'amdgpu ' > /dev/null
if [ $? -ne 0 ]; then
    echo "Error: unable to find amdgpu module"
    exit 1
fi