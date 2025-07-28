#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

ps aux | grep /usr/local/bin/containerd | grep -v grep > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: /usr/local/bin/containerd is not running"
  exit 1
fi