#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "Starting runner proxy..."

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

python3 "${SCRIPT_DIR}/proxy.py"
exit_code=$?

echo "runner proxy exited with code: ${exit_code}"

exit ${exit_code}
