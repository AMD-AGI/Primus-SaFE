#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Detect WekaFS client processes stuck in uninterruptible sleep (D state) and
# restart the local Weka client. Always exits 0 — this monitor is a recovery
# action, never a failure signal.

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

# Skip when nfs_type argument is missing or not exactly "wekafs".
nfs_type="${1:-}"
if [ "$nfs_type" != "wekafs" ]; then
    exit 0
fi

# Skip when the host has no Weka client binary.
if ! $NSENTER test -x /usr/bin/weka; then
    exit 0
fi

# Scan for D-state PIDs whose kernel stack contains a wekafs_ frame; restart
# the local Weka client on the first match. Errors are swallowed deliberately.
$NSENTER bash -c '
    ps -eo pid,stat,cmd | awk '"'"'$2 ~ /^D/'"'"' | while read pid rest; do
        if sudo cat /proc/$pid/stack 2>/dev/null | grep -q "wekafs_"; then
            /usr/bin/weka local restart client
            break
        fi
    done
' >/dev/null 2>&1

exit 0
