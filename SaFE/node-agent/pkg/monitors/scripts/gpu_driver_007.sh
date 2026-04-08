#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

JSON_FILE="/tmp/rocm-smi.json"

if [ ! -f "${JSON_FILE}" ]; then
  exit 0
fi

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <driver-version>"
  echo "Example: $0 '6.12.12'"
  exit 2
fi

IFS='.' read -ra parts <<< "$1"
length=${#parts[@]}

version=$(jq -r '.system["Driver version"] // empty' "${JSON_FILE}" 2>/dev/null)
if [ -z "$version" ]; then
  echo "Error: failed to parse driver version from ${JSON_FILE}"
  exit 1
fi

major_version=$(echo "$version" | cut -d '.' -f 1)

if [ $length -ge 1 ]; then
  if [ -n "${parts[0]}" ] && [ "$major_version" != "${parts[0]}" ]; then
    echo "current gpu driver major version is $major_version, but the expect value is ${parts[0]}"
    exit 1
  fi
fi
