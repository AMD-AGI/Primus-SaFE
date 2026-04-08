#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

host() { nsenter --target 1 --mount --uts --ipc --net --pid -- "$@"; }

host test -x /usr/bin/amd-smi > /dev/null 2>&1
if [ $? -ne 0 ]; then
  exit 2
fi

output=$(host /usr/bin/amd-smi bad-pages 2>&1)
if [ $? -ne 0 ]; then
  echo "Error: amd-smi bad-pages command failed. $output"
  exit 1
fi

msg=$(echo "$output" | grep "Address:")
if [ -n "$msg" ]; then
  count=$(echo "$msg" | wc -l)
  echo "Error: $count bad page(s) detected. $(echo "$msg" | head -5)"
  exit 1
fi