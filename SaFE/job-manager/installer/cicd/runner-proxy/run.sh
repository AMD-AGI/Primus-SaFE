#!/bin/sh

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

pip install -r "${SCRIPT_DIR}/requirements.txt" >/dev/null

exec python3 "${SCRIPT_DIR}/workload_client.py"