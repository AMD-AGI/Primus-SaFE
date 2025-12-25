#!/bin/bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Validate GPU_PRODUCT
case "$GPU_PRODUCT" in
  MI300X|MI325X|MI355X)
    ;;
  *)
    echo "Error: Unsupported GPU_PRODUCT '$GPU_PRODUCT'. Only MI300X, MI325X, and MI355X are supported." >&2
    exit 1
    ;;
esac