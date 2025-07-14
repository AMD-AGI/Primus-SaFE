#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$#" -lt 3 ]; then
    echo "Usage: $0 <model> <link_speed> <link_width>"
    echo "Example: $0 1002:74a1 32GT/s x16"
    exit 2
fi

PCI_OUTPUT=`nsenter --target 1 --mount --uts --ipc --net --pid -- lspci -d $1 -vvv | grep -e DevSta -e LnkS`

FATAL_ERR_FOUND=0
LINK_BAD=0
EXPECTED_SPEED="$2"
EXPECTED_WIDTH="$3"

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
            print "[ERROR] FatalErr+ found in DevSta"
            exit 1
        }
    }
}

/LnkSta:/ {
    line = $0
    if (line !~ ("Speed " speed)) {
        print "[ERROR] Expected Speed: " speed ", got different value"
        exit 1
    }
    if (line !~ ("Width " width)) {
        print "[ERROR] Expected Width: " width ", got different value"
        exit 1
    }
}
'

RESULT=$?

if [ $RESULT -eq 0 ]; then
    echo "[OK] All checks passed: No FatalErr and Link is ${EXPECTED_SPEED} ${EXPECTED_WIDTH}"
else
    echo "[FAIL] Some PCIe status check failed"
    exit 1
fi
