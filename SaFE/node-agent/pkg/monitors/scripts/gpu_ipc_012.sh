#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

# Search for the specific error in dmesg
if nsenter --target 1 --mount --uts --ipc --net --pid -- dmesg | grep -q "amdgpu: Failed to import IPC handle"; then
    echo "Error: 'amdgpu: Failed to import IPC handle' found in dmesg."
    exit 1
fi
