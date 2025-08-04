#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 driver-version"
  echo "Example: $0 231.0.162.0"
  exit 2
fi

EXPECT_VERSION=$1

version=`nsenter --target 1 --mount --uts --ipc --net --pid -- modinfo bnxt_re|grep ^version|awk -F" " '{print $2}'`
if [ $? -ne 0 ]; then
  echo "Error: unable to find bnxt_re driver version"
  exit 2
fi
if [ -n "$EXPECT_VERSION" ] && [ "$version" != "$EXPECT_VERSION" ]; then
  echo "current bnxt_re driver version is $version, but the expect value is $EXPECT_VERSION"
  exit 1
fi