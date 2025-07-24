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
  # The /var/log/ directory is a hostPath volume, and the host's identically named directory has already been mounted into the container.
  cp $script /var/log
  nsenter --target 1 --mount --uts --ipc --net --pid -- bash /var/log/$script
  ret=$?
  rm -f /var/log/$script
  if [ $ret -ne 0 ]; then
    exit 127
  fi
done