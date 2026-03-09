#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

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
  # Check if already immutable
  attrs=$(${NSENTER} lsattr ${TARGET_FILE} 2>/dev/null)
  if echo "$attrs" | grep -q "^....i"; then
    exit 0
  fi

  # Check if already read-only (444)
  perms=$(${NSENTER} stat -c "%a" ${TARGET_FILE} 2>/dev/null)
  if [ "$perms" = "444" ]; then
    exit 0
  fi

  # Try to set immutable first
  ${NSENTER} chattr +i ${TARGET_FILE} 2>/dev/null
  if [ $? -eq 0 ]; then
    echo "Set ${TARGET_FILE} to immutable (contains nameserver 127.0.0.53)"
  else
    ${NSENTER} chmod 444 ${TARGET_FILE}
    if [ $? -eq 0 ]; then
      echo "Set ${TARGET_FILE} to read-only (contains nameserver 127.0.0.53)"
    else
      echo "Error: failed to set ${TARGET_FILE} to read-only"
      exit 1
    fi
  fi
fi
