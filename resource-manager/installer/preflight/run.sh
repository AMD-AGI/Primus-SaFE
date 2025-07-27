#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ -z "$GPU_PRODUCT" ]; then
  echo "[ERROR] GPU_PRODUCT is not set" >&2
  exit 1
fi

found=0
target_dir=""
while IFS= read -r -d $'\0' dir; do
  DIR_NAME=$(basename "$dir")
  if [[ "$GPU_PRODUCT" == *"$DIR_NAME"* ]]; then
    target_dir=$dir
    found=1
    break
  fi
done < <(find . -type d -print0)

if [ "$found" -eq 0 ]; then
  echo "The $GPU_PRODUCT test is not supported" >&2
  exit 1
fi

cd $target_dir
has_error=0
for script in *.sh; do
  echo "Running script: $script"
  bash "$script"
  if [ $? -ne 0 ]; then
    has_error=1
  fi
done

if [ "$has_error" -eq 1 ]; then
  exit 127
fi