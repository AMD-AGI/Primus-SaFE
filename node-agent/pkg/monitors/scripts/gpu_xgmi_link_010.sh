#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

msg=`nsenter --target 1 --mount --uts --ipc --net --pid -- dmesg | grep -i xgmi |grep "link error"`
if [ $? -eq 0 ]; then
    echo "Error: $msg"
    exit 1
fi