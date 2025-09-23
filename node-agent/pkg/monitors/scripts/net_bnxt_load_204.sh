#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod |grep '^bnxt_re ' > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: unable to find bnxt_re module"
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod |grep '^bnxt_en ' > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: unable to find bnxt_en module"
  exit 1
fi