#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ ! -f "/tmp/rocm-smi" ]; then
  exit 0
fi

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <driver-version>"
  echo "Example: $0 '6.12.12'"
  exit 2
fi

IFS='.' read -ra parts <<< "$1"
length=${#parts[@]}

version=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi --showdriverversion |grep "^Driver version:"`
if [ $? -ne 0 ]; then
  echo "Error: failed to execute rocm-smi --showdriverversion"
  exit 1
fi
major_version=$(echo "$version" | cut -d ' ' -f 3 | cut -d '.' -f 1)
minor_version=$(echo "$version" | cut -d ' ' -f 3 | cut -d '.' -f 2)

if [ $length -ge 1 ]; then
  if [ -n "${parts[0]}" ] && [ "$major_version" != "${parts[0]}" ]; then
    echo "current gpu driver major version is $major_version, but the expect value is ${parts[0]}"
    exit 1
  fi
fi

if [ $length -ge 2 ]; then
  if [ -n "${parts[1]}" ] && [ "$minor_version" != "${parts[1]}" ]; then
    echo "current gpu driver minor version is $minor_version, but the expect value is ${parts[1]}"
    exit 1
  fi
fi