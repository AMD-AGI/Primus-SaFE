#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Use the command "rvs -c gpup_single.conf" to validate GPU properties.

RVS_CONF=/opt/rocm/share/rocm-validation-suite/conf/gpup_single.conf
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
if echo "$OUTPUT" | grep -i "error" > /dev/null; then
  echo "Error detected in \"$OUTPUT\"" >&2
  exit 1
fi

