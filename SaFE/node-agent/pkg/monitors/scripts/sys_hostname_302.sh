#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

nsenter --target 1 --mount --uts --ipc --net --pid -- hostname > /dev/null
if [ $? -ne 0 ]; then
  echo "hostname is abnormal"
  exit 1
fi