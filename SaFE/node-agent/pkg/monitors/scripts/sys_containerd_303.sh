#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

nsenter --target 1 --mount --uts --ipc --net --pid -- ps aux | grep -q /usr/local/bin/containerd
if [ $? -ne 0 ]; then
  echo "Error: /usr/local/bin/containerd is not running"
  exit 1
fi