#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

SERVICE_NAME="systemd-resolved"

check_service() {
    nsenter --target 1 --mount --uts --ipc --net --pid -- systemctl is-active "$SERVICE_NAME" 2>/dev/null
}

STATUS=$(check_service)
if [ "$STATUS" = "active" ]; then
    echo "$SERVICE_NAME Status: ✓ RUNNING"
    exit 0
elif [ "$STATUS" = "inactive" ]; then
    echo "$SERVICE_NAME Status: ✗ STOPPED"
    exit 1
elif [ "$STATUS" = "failed" ]; then
    echo "$SERVICE_NAME Status: ✗ FAILED"
    exit 1
else
    echo "$SERVICE_NAME Status: ? UNKNOWN ($STATUS)"
    exit 2
fi