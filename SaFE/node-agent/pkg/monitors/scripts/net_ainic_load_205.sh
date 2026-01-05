#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod |grep '^ionic_rdma ' > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: unable to find ionic_rdma module"
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod |grep '^ionic ' > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: unable to find ionic module"
  exit 1
fi
