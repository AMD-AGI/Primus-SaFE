#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- hostname > /dev/null
if [ $? -ne 0 ]; then
    echo "hostname is abnormal"
    exit 1
fi