#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if ! nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/rocminfo >/dev/null 2>&1; then
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocminfo > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "Error: failed to execute rocminfo"
  exit 1
fi