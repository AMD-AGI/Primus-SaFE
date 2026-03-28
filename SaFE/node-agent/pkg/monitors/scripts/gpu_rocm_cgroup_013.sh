#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Check 1: Is /dev/kfd a valid character device?
if [ ! -c /dev/kfd ]; then
    echo "Error: /dev/kfd not found or not a character device. KFD driver may be disabled or not loaded."
    exit 1
fi

# Check 2: Can we actually open /dev/kfd?
if ! cat /dev/kfd >/dev/null 2>&1 && ! test -r /dev/kfd; then
    echo "Error: Cannot access /dev/kfd. Permission denied."
    exit 1
fi