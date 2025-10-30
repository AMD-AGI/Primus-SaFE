#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

EXPECTED_SPEED="32GT/s"
EXPECTED_WIDTH="x16"
GPU_PRODUCT=`nsenter --target 1 --mount --uts --ipc --net --pid -- rocm-smi --showproductname |grep "Card Series" |head -1 |awk -F"\t" '{print $NF}'`
if [ $? -ne 0 ] || [ -z "$GPU_PRODUCT" ]; then
  echo "Error: failed to get product"
  exit 1
fi

shopt -s nocasematch
model=""
if [[ "$GPU_PRODUCT" == *"mi300x"* ]]; then
  model="1002:74a1"
elif [[ "$GPU_PRODUCT" == *"mi325x"* ]]; then
  model="1002:74a5"
elif [[ "$GPU_PRODUCT" == *"mi350x"* ]]; then
  model="1002:75a0"
elif [[ "$GPU_PRODUCT" == *"mi355x"* ]]; then
  model="1002:75a3"
else
  echo "${LOG_HEADER}[$(date +'%Y-%m-%d %H:%M:%S')] [WARNING] The $GPU_PRODUCT is not supported" >&2
  exit 0
fi

PCI_OUTPUT=`nsenter --target 1 --mount --uts --ipc --net --pid -- lspci -d $model -vvv | grep -e DevSta -e LnkS`
if [ $? -ne 0 ]; then
  echo "failed to get PCI info"
  exit 1
fi
FATAL_ERR_FOUND=0
LINK_BAD=0

echo "$PCI_OUTPUT" | awk -v speed="$EXPECTED_SPEED" -v width="$EXPECTED_WIDTH" '
BEGIN {
    in_section = 0
}

/DevSta:/ {
    devsta = $0
    split(devsta, fields, ",")
    fatal_found = 0
    for (i in fields) {
        if (index(fields[i], "FatalErr+") != 0) {
            print "FatalErr+ found in DevSta"
            exit 1
        }
    }
}

/LnkSta:/ {
    line = $0
    if (line !~ ("Speed " speed)) {
        print "Expected Speed: " speed ", got different value"
        exit 1
    }
    if (line !~ ("Width " width)) {
        print "Expected Width: " width ", got different value"
        exit 1
    }
}
'

if [ $? -ne 0 ]; then
  exit 1
fi
