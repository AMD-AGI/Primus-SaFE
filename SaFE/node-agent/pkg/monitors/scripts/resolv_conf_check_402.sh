#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 {\"clusterId\": \"cluster1\"}"
  exit 2
fi

clusterId=$(echo "$1" | jq -r '.clusterId // empty')
if [ -z "$clusterId" ]; then
  echo "Error: failed to get clusterId from input: $1"
  exit 2
fi

RESOLV_CONF="/etc/resolv.conf"
NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

${NSENTER} test -e ${RESOLV_CONF}
if [ $? -ne 0 ]; then
  echo "Warning: ${RESOLV_CONF} does not exist"
  exit 2
fi

TARGET_FILE=${RESOLV_CONF}
${NSENTER} test -L ${RESOLV_CONF}
if [ $? -eq 0 ]; then
  TARGET_FILE=$(${NSENTER} readlink -f ${RESOLV_CONF})
  if [ $? -ne 0 ] || [ -z "${TARGET_FILE}" ]; then
    echo "Warning: failed to resolve symlink ${RESOLV_CONF}"
    exit 2
  fi
  echo "${RESOLV_CONF} is a symlink to ${TARGET_FILE}"
fi

content=$(${NSENTER} cat ${TARGET_FILE})
if [ $? -ne 0 ]; then
  echo "Warning: failed to read ${TARGET_FILE}"
  exit 2
fi

if echo "$content" | grep -q "nameserver 127.0.0.53"; then
  # Already has 127.0.0.53: check if immutable, set chattr +i if not
  attrs=$(${NSENTER} lsattr ${TARGET_FILE} 2>/dev/null)
  if [ -n "$attrs" ] && echo "$attrs" | grep -qE '^[ -]{4}i'; then
    exit 0
  fi

  ${NSENTER} chattr +i ${TARGET_FILE} 2>/dev/null
  if [ $? -eq 0 ]; then
    echo "Set ${TARGET_FILE} immutable (contains nameserver 127.0.0.53)"
  else
    echo "failed to chattr +i ${TARGET_FILE}"
  fi
else
  # Does not have 127.0.0.53: add it before first nameserver, then set immutable
  ${NSENTER} chattr -i ${TARGET_FILE} 2>/dev/null
  ${NSENTER} sed -i '0,/^nameserver/{s/^nameserver/nameserver 127.0.0.53\nnameserver/}' ${TARGET_FILE} 2>/dev/null
  if [ $? -ne 0 ]; then
    echo "Error: failed to add nameserver 127.0.0.53 to ${TARGET_FILE}"
    exit 1
  fi
  ${NSENTER} chattr +i ${TARGET_FILE} 2>/dev/null
  if [ $? -eq 0 ]; then
    echo "Added nameserver 127.0.0.53 to ${TARGET_FILE} and set immutable"
  else
    echo "Added nameserver 127.0.0.53 to ${TARGET_FILE} (chattr +i not supported)"
  fi
fi
