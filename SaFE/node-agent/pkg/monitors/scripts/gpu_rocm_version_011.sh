#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <rocm-version>"
  echo "Example: $0 '6.4.43484'"
  exit 2
fi

host() { nsenter --target 1 --mount --uts --ipc --net --pid -- "$@"; }

expect_version=$1

hipconfig=""
for path in /usr/bin/hipconfig /opt/rocm/bin/hipconfig; do
  if host test -x "$path" > /dev/null 2>&1; then
    hipconfig="$path"
    break
  fi
done
if [ -z "$hipconfig" ]; then
  exit 2
fi

current_version=$(host "$hipconfig" --version | awk -F'.' '{print $1"."$2}')
if [ $? -ne 0 ]; then
  echo "Error: failed to execute $hipconfig --version"
  exit 1
fi

if [ "$expect_version" != "$current_version" ]; then
  echo "Error: current rocm version $current_version, but the expect value is $expect_version"
  exit 1
fi