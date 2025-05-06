#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/rocm-smi > /dev/null
if [ $? -ne 0 ]; then
    exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi --showtopoaccess |grep -i false >/dev/null
if [ $? -eq 0 ]; then
    echo "Error: There is a link error between two GPUs"
    exit 1
fi