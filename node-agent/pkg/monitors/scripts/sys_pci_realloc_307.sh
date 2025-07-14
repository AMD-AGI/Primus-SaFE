#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

CMDLINE=`nsenter --target 1 --mount --uts --ipc --net --pid -- cat /proc/cmdline`
for cmd in `echo $CMDLINE`
do
    if [[ "$cmd" == "pci=realloc=off" ]]; then
        exit 0
    fi
done

echo "pci=realloc=on"
exit 1
