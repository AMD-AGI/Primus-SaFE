#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

nsenter --target 1 --mount --uts --ipc --net --pid -- date | grep ' UTC ' > /dev/null
if [ $? -ne 0 ]; then
    echo "The time zone of node is not UTC zone"
    exit 1
fi