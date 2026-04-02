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

if command -v bash >/dev/null 2>&1; then
  if curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.sh | bash >/dev/null; then
    echo "INFO: AMD certificates installed successfully"
  else
    echo "WARN: setup-certs failed, AMD certificates may not be installed"
  fi
else
  echo "WARN: bash not found, skipping AMD certificate installation"
fi