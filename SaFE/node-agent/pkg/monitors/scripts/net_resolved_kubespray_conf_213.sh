#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"
CONF_PATH="/etc/systemd/resolved.conf.d/kubespray.conf"

if [ -n "$1" ]; then
  DOMAINS="~$1.primus-safe.amd.com default.svc.cluster.local svc.cluster.local"
else
  DOMAINS="default.svc.cluster.local svc.cluster.local"
fi

if ${NSENTER} test -f "$CONF_PATH" 2>/dev/null; then
  current_domains=$(${NSENTER} grep -E '^Domains=' "$CONF_PATH" 2>/dev/null | sed 's/^Domains=//')
  if [ "$current_domains" = "$DOMAINS" ]; then
    exit 0
  fi
fi

${NSENTER} mkdir -p /etc/systemd/resolved.conf.d/ 2>/dev/null
if [ $? -ne 0 ]; then
  echo "Error: failed to create /etc/systemd/resolved.conf.d/"
  exit 1
fi

${NSENTER} tee "$CONF_PATH" > /dev/null <<EOF
[Resolve]
DNS=169.254.25.10
FallbackDNS=
Domains=${DOMAINS}
DNSSEC=no
Cache=no-negative
EOF

if [ $? -ne 0 ]; then
  echo "Error: failed to write $CONF_PATH"
  exit 1
fi

${NSENTER} systemctl restart systemd-resolved 2>/dev/null
if [ $? -ne 0 ]; then
  echo "Error: failed to restart systemd-resolved"
  exit 1
fi

echo "Created $CONF_PATH and restarted systemd-resolved"
exit 0
