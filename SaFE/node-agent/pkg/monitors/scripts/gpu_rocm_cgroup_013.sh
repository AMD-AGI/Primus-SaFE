#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Check 1: Is /dev/kfd present?
if [ ! -e /dev/kfd ]; then
    echo "Error: /dev/kfd not found. KFD driver may be disabled or not loaded." >&2
    exit 1
fi

# Check 2: Can we actually open /dev/kfd?
if ! python3 -c "open('/dev/kfd', 'rb').close()" 2>/dev/null; then
    echo "Error: Cannot access /dev/kfd. Permission denied." >&2
    exit 1
fi