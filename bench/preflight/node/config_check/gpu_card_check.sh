#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ ! -f "/tmp/rocm-smi" ]; then
  exit 0
fi

expectedCount="8"
actualCount=`cat "/tmp/rocm-smi" | grep '^[0-9]' |wc -l`
ret=$?
if [ $ret -ne 0 ]; then
  echo "failed to execute rocm-smi"
  exit 2
fi

if [ $actualCount -ne $expectedCount ]; then
  echo "GPU count is $actualCount, less than expected $expectedCount"
  exit 1
fi