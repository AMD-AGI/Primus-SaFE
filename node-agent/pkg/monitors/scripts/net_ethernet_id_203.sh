#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ -z "$1" ]; then
  echo "Usage: $0 <search_id>"
  exit 2
fi

SEARCH_ID="$1"

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/sbin/ifconfig > /dev/null
if [ $? -ne 0 ]; then
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/ifconfig |grep "$SEARCH_ID" > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: No network interface found containing \"$SEARCH_ID\""
  exit 1
fi