#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

count=`nsenter --target 1 --mount --uts --ipc --net --pid --  /usr/bin/lspci -vvv |grep ACSCtl |grep + |wc -l`
if [ $? -ne 0 ]; then
    echo "Error: failed to execute lspci"
    exit 2
fi

if [ $count -ne 0 ]; then
    echo 'acsctl is enabled, but the expectation is disabled'
    exit 1
fi