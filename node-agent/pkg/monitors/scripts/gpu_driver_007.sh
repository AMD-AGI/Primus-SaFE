#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

EXPECT_MAJOR=$1
EXPECT_MINOR=$2

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/rocm-smi > /dev/null
if [ $? -ne 0 ]; then
    exit 2
fi

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