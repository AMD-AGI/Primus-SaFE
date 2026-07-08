#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ "$WORKLOAD_KIND" != "Authoring" ]; then
  exit 0
fi
. /shared-data/utils.sh
install_if_not_exists openssh-server
if [ $? -eq 0 ]; then
  echo "openssh-server installation succeeded"
else
  echo "openssh-server installation failed"
fi

# curl is required below to fetch the AMD certificate setup script.
install_if_not_exists curl

if command -v bash >/dev/null 2>&1 && command -v curl >/dev/null 2>&1; then
  # Download and execute separately so curl failures are detected (a piped
  # `curl | bash` returns bash's exit code, masking a missing/failed curl).
  if curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.sh -o /tmp/setup-certs.sh \
     && bash /tmp/setup-certs.sh >/dev/null; then
    echo "INFO: AMD certificates installed successfully"
  else
    echo "WARN: setup-certs failed, AMD certificates may not be installed"
  fi
  rm -f /tmp/setup-certs.sh
else
  echo "WARN: bash or curl not found, skipping AMD certificate installation"
fi