#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c gst_single.conf" to stress the GPU FLOPS performance.

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf/$GPU_PRODUCT/gst_single.conf
if [ ! -f "${RVS_CONF}" ]; then
  echo "${RVS_CONF} does not exist" >&2
  exit 1
fi

OUTPUT=$(/opt/rocm/bin/rvs -c "${RVS_CONF}")
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "rvs failed with exit code: $EXIT_CODE" >&2
  exit 1
fi

tmpfile="/tmp/match_lines.txt"
echo "$OUTPUT" | grep "met: FALSE" > $tmpfile
if [ -s /tmp/match_lines.txt ]; then
  cat $tmpfile && rm -f $tmpfile
  echo "'met: FALSE' is found in result" >&2
  exit 1
fi

rm -f $tmpfile
