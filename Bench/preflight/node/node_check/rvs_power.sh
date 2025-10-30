#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c iet_single.conf" to stress the GPU power

host=$(hostname)
RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf/$GPU_PRODUCT/iet_single.conf
if [ ! -f "${RVS_CONF}" ]; then
  echo "${RVS_CONF} does not exist" >&2
  exit 0
fi

OUTPUT=$(/opt/rocm/bin/rvs -c "${RVS_CONF}")
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

tmpfile="/tmp/match_lines.txt"
echo "$OUTPUT" | grep "pass: FALSE" > $tmpfile
if [ -s /tmp/match_lines.txt ]; then
  cat $tmpfile && rm -f $tmpfile
  echo "'pass: FALSE' is found in result" >&2
  exit 1
fi

rm -f $tmpfile

