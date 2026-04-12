#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

SERVICE_NAME="systemd-resolved"
NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

check_service() {
    ${NSENTER} systemctl is-active "$SERVICE_NAME" 2>/dev/null
}

restart_service() {
    ${NSENTER} systemctl restart "$SERVICE_NAME" 2>/dev/null
}

STATUS=$(check_service)
if [ "$STATUS" = "active" ]; then
    exit 0
fi

echo "$SERVICE_NAME is not active (status: $STATUS), attempting restart..."
restart_service

STATUS=$(check_service)
if [ "$STATUS" = "active" ]; then
    echo "$SERVICE_NAME restarted successfully"
    exit 0
fi

echo "Error: $SERVICE_NAME restart failed (status: $STATUS)"
exit 1