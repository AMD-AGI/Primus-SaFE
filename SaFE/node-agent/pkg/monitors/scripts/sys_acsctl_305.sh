#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

count=`nsenter --target 1 --mount --uts --ipc --net --pid --  /usr/bin/lspci -vvv |grep ACSCtl |grep "SrcValid+" |wc -l`
if [ $? -ne 0 ]; then
  echo "Error: failed to execute lspci"
  exit 2
fi

if [ $count -gt 0 ]; then
  echo "Warning: ACS (Access Control Services) is enabled."
  exit 1
fi