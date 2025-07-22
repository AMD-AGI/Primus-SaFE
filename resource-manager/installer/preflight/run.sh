#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ -z "$GPU_PRODUCT" ]; then
  echo "[ERROR] GPU_PRODUCT is not set" >&2
  exit 1
fi

cd $GPU_PRODUCT || { echo "The $GPU_PRODUCT test is not supported" >&2; exit 1; }

for script in *.sh
do
  echo "running script: $script"
  nsenter --target 1 --mount --uts --ipc --net --pid -- bash $script
  ret=$?
  if [ $ret -ne 0 ]; then
    exit $ret
  fi
done