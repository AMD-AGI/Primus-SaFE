#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

nsenter --target 1 --mount --uts --ipc --net --pid -- date | grep ' UTC ' > /dev/null
if [ $? -ne 0 ]; then
  echo "The time zone of node is not UTC zone"
  exit 1
fi