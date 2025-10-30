#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

nsenter --target 1 --mount --uts --ipc --net --pid --  grep -q "performance" /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
if [ $? -ne 0 ]; then
  echo "cpufreq is not configured to 'performance' mode"
  exit 1
fi
