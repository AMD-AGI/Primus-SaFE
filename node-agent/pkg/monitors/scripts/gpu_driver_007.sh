#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ ! -f "/tmp/rocm-smi" ]; then
    exit 0
fi

EXPECT_MAJOR=$1
EXPECT_MINOR=$2

version=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi --showdriverversion |grep "^Driver version:"`
if [ $? -ne 0 ]; then
    echo "Error: failed to execute rocm-smi --showdriverversion"
    exit 1
fi
major_version=$(echo "$version" | cut -d ' ' -f 3 | cut -d '.' -f 1)
minor_version=$(echo "$version" | cut -d ' ' -f 3 | cut -d '.' -f 2)

if [ -n "$EXPECT_MAJOR" ] && [ "$major_version" != "$EXPECT_MAJOR" ]; then
  echo "current gpu driver major version is $major_version, but the expect value is $EXPECT_MAJOR"
  exit 1
fi

if [ -n "$EXPECT_MINOR" ] && [ "$minor_version" != "$EXPECT_MINOR" ]; then
  echo "current gpu driver minor version is $minor_version, but the expect value is $EXPECT_MINOR"
  exit 1
fi