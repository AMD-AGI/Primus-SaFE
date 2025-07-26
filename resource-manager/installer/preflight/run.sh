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

has_error=0
for script in *.sh; do
  echo "Running script: $script"
  bash "$script"
  if [ $? -ne 0 ]; then
    echo "Error occurred in script: $script" >&2
    has_error=1
  fi
done

if [ "$has_error" -eq 1 ]; then
  exit 127
fi