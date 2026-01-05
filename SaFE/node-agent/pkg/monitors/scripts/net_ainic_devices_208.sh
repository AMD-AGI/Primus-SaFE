#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ -z "$1" ]; then
  echo "Usage: $0 \"device1,device2,...\""
  echo "Example: $0 \"bnxt_re0,bnxt_re1,bnxt_re2\""
  exit 2
fi

NIC_COUNT=$(nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/nicctl show environment | grep ^NIC | wc -l)
IFS=',' read -ra DEV_ARRAY <<< "$1"
if [ "${#DEV_ARRAY[@]}" -ne "$NIC_COUNT" ]; then
  echo "Error: Device count mismatch, ${#DEV_ARRAY[@]} IB devices configured but $NIC_COUNT AI NIC cards found"
  exit 1
fi