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

# socat backs the Pod-side listener for `ssh -R`: the apiserver execs a
# `socat TCP-LISTEN:<port>,bind=127.0.0.1,fork` in this container and refuses the
# forward outright when it is absent. The workload image belongs to the user - it is
# usually an upstream vLLM/SGLang/PyTorch image - so the platform cannot assume socat
# is in it, any more than it assumes sshd is. Report on what is actually on PATH
# rather than on the install's exit status: install_if_not_exists sends apt's own
# output to /dev/null, and PATH is what the apiserver's listener script tests.
install_if_not_exists socat
if command -v socat >/dev/null 2>&1; then
  echo "socat is available, ssh -R remote forwarding will work"
else
  echo "WARN: socat is not available, ssh -R remote forwarding will be refused"
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