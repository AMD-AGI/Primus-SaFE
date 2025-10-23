#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

container_count=$(nsenter --target 1 --mount --uts --ipc --net --pid -- docker ps --format '{{.ID}}' | wc -l)
if [ $? -ne 0 ]; then
  exit 2
fi

if [ "$container_count" -gt 0 ]; then
  exit 1
fi