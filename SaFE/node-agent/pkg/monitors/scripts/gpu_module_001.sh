#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

# Stage capture under the app dir, then move to /tmp/rocm-smi (other gpu_*.sh scripts read that path).
STATE_DIR="/opt/primus-safe/node-agent/tmp"
if ! mkdir -p "${STATE_DIR}" "/tmp"; then
  echo "Error: cannot create ${STATE_DIR} or /tmp"
  exit 1
fi
tmpfile="${STATE_DIR}/rocm-smi.tmp"
outfile="/tmp/rocm-smi"

nsenter --target 1 --mount --uts --ipc --net --pid -- lsmod | grep 'amdgpu ' > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: unable to find amdgpu module"
  exit 1
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- ls /usr/bin/rocm-smi > /dev/null
if [ $? -ne 0 ]; then
  exit 2
fi

nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/bin/rocm-smi > "${tmpfile}"
ret=$?
if [ $ret -ne 0 ]; then
  echo "Error: failed to execute rocm-smi. ret=$ret"
  /bin/rm -f "${tmpfile}" 2>/dev/null || > "${tmpfile}" 2>/dev/null || true
  exit 1
fi
/bin/mv -f "${tmpfile}" "${outfile}"
exit 0
