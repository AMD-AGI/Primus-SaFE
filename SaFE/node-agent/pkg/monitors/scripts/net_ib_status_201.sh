#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ -z "$1" ]; then
  echo "Usage: $0 \"device1,device2,...\""
  echo "Example: $0 \"bnxt_re0,bnxt_re1,bnxt_re2\""
  exit 1
fi

IFS=',' read -ra DEV_ARRAY <<< "$1"
for dev in "${DEV_ARRAY[@]}"; do
  dev=$(echo "$dev" | xargs)

  OUTPUT=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/ibstatus "$dev" 2>&1)
  ret=$?
  if [ $ret -ne 0 ]; then
    exit 1
  fi

  STATE_LINE=$(echo "$OUTPUT" | grep "state:" | head -n1)
  if [ -z "$STATE_LINE" ]; then
    exit 1
  fi

  STATE=$(echo "$STATE_LINE" | awk -F':' '{print $3}' | xargs)
  if [ "$STATE" != "ACTIVE" ]; then
    echo "Error: Device '$dev' is DOWN!"
    exit 1
  fi
done