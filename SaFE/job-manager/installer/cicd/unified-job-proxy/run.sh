#!/bin/sh

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "Starting unified job proxy..."

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

pip install -r "${SCRIPT_DIR}/requirements.txt" > /dev/null

exec python3 "${SCRIPT_DIR}/proxy.py"
exit_code=$?

echo "unified job proxy exited with code: ${exit_code}"

exit ${exit_code}
