#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail
export PATH="/usr/bin:/bin:${PATH:-}"

# Ensure same-node Pod -> host-IP routing works on this node.
#
# Nodes use per-source policy routing for the RoCE data NIC
# ("from <node-ip> lookup table 212"), but that table has no route for the
# Pod CIDR. Replies from a host-network process to a Pod on the SAME node are
# therefore sent out the fabric NIC and dropped, so same-node Pod -> node-IP
# TCP connections time out. A single high-priority rule fixes it by forcing
# traffic destined to the Pod CIDR through the main table (cni0):
#
#     ip rule add to <POD_CIDR> lookup main pref <PREF>
#
# This monitor re-applies the rule when it is missing (e.g. after a reboot,
# which drops runtime "ip rule" entries). It is idempotent and only mutates
# when the rule is absent.
#
# Arguments:
#   $1  Node info JSON ($Node), e.g. '{"nodeName":"n1","kubePodsSubnet":"172.16.0.0/13"}'.
#       The Pod CIDR is read from its "kubePodsSubnet" field. If the field is
#       missing/empty (or parsing fails) the script is a no-op and returns 0.

# Priority for the rule. Must be below the per-source rules (>= 32755).
PREF=1000

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <node-info>"
  echo "Example: $0 {\"kubePodsSubnet\": \"172.16.0.0/13\"}"
  exit 2
fi

# Parse the Pod CIDR from the node JSON. Any parse failure / empty / "null"
# value means "not configured" -> do nothing and return success.
POD_CIDR=$(echo "$1" | jq -r '.kubePodsSubnet' 2>/dev/null)
if [ -z "$POD_CIDR" ] || [ "$POD_CIDR" == "null" ]; then
  echo "kubePodsSubnet not set, skipping"
  exit 0
fi

# Basic CIDR sanity check: <ipv4>/<prefixlen>. Bad value -> skip (return 0).
if ! echo "$POD_CIDR" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$'; then
  echo "kubePodsSubnet '$POD_CIDR' is not a valid CIDR, skipping"
  exit 0
fi

# Run network commands in the host network namespace (the agent container is
# hostPID + privileged but not hostNetwork), same pattern as other monitors.
NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

# Already present (any rule that sends this CIDR to the main table) -> done.
if ${NSENTER} ip rule show 2>/dev/null | grep -F "$POD_CIDR" | grep -q "lookup main"; then
  echo "OK: rule present (to $POD_CIDR lookup main)"
  exit 0
fi

# Missing -> add it.
if ${NSENTER} ip rule add to "$POD_CIDR" lookup main pref "$PREF" 2>/dev/null; then
  # Confirm it took effect.
  if ${NSENTER} ip rule show 2>/dev/null | grep -F "$POD_CIDR" | grep -q "lookup main"; then
    echo "Re-added rule: to $POD_CIDR lookup main pref $PREF"
    exit 0
  fi
  echo "Error: rule add reported success but is not present for $POD_CIDR"
  exit 1
fi

echo "Error: failed to add rule (to $POD_CIDR lookup main pref $PREF)"
exit 1
