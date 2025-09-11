#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <rocm-version>"
  echo "Example: $0 '6.4.43484'"
  exit 2
fi

expect_version=$1
current_version=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/hipconfig --version |awk -F'.' '{print $1"."$2}'`
if [ $? -ne 0 ]; then
  echo "Error: failed to execute hipconfig --version"
  exit 1
fi

if [ "$expect_version" != "$current_version" ]; then
  echo "current rocm version $current_version, but the expect value is $expect_version"
  exit 1
fi