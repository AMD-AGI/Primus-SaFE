#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"
CONF_PATH="/etc/systemd/resolved.conf.d/kubespray.conf"

${NSENTER} test -f "$CONF_PATH" 2>/dev/null
if [ $? -eq 0 ]; then
  exit 0
fi

${NSENTER} mkdir -p /etc/systemd/resolved.conf.d/ 2>/dev/null
if [ $? -ne 0 ]; then
  echo "Error: failed to create /etc/systemd/resolved.conf.d/"
  exit 1
fi

${NSENTER} tee "$CONF_PATH" > /dev/null <<'EOF'
[Resolve]
DNS=169.254.25.10
FallbackDNS=
Domains=default.svc.cluster.local svc.cluster.local
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
