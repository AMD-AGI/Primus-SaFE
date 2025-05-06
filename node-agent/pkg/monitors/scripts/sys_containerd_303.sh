#!/bin/bash

#
# Copyright © AMD. 2025-2026. All rights reserved.
#

ps aux | grep /usr/local/bin/containerd | grep -v grep > /dev/null
if [ $? -ne 0 ]; then
    echo "Error: /usr/local/bin/containerd is not running"
    exit 1
fi