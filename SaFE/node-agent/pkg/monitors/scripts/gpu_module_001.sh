#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"

host() { nsenter --target 1 --mount --uts --ipc --net --pid -- "$@"; }

json_tmpfile="/tmp/rocm-smi.json.tmp"
json_outfile="/tmp/rocm-smi.json"

host lsmod | grep 'amdgpu ' > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: unable to find amdgpu module"
  exit 1
fi

host test -x /usr/bin/rocm-smi > /dev/null 2>&1
if [ $? -ne 0 ]; then
  exit 2
fi

# Cache JSON output with all-info, driver version, and topology
host /usr/bin/rocm-smi -a --showdriverversion --showtopoaccess --json > "${json_tmpfile}" 2>/dev/null
ret=$?
if [ $ret -ne 0 ]; then
  echo "Error: failed to execute rocm-smi --json. ret=$ret"
  rm -f "${json_tmpfile}" 2>/dev/null || true
  exit 1
fi
mv -f "${json_tmpfile}" "${json_outfile}" 2>/dev/null || true

exit 0
