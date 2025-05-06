#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod |grep 'amdgpu ' > /dev/null
if [ $? -ne 0 ]; then
    echo "Error: unable to find amdgpu module"
    exit 1
fi