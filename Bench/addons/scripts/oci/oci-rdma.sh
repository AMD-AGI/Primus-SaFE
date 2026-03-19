#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -euo pipefail

eth_mtu=9000
vf_mtu=9000

nicctl update qos dscp-to-purpose --dscp 46 --purpose rdma-ack >/dev/null 2>&1
nicctl update qos scheduling --priority 0,1,6 --dwrr 99,1,0 --rate-limit 0,0,10 >/dev/null 2>&1

while read -r vf_ndev; do
    ip link set mtu "${vf_mtu}" "${vf_ndev}" 2>/dev/null
done < <(rdma link 2>/dev/null | grep '_vf' | awk '{print $NF}')

while read -r ndev; do
    ip link set mtu "${eth_mtu}" "${ndev}" 2>/dev/null
done < <(rdma link 2>/dev/null | grep 'enp' | awk '{print $NF}')

nicctl debug update pipeline internal rdma --skip-data-copy disable >/dev/null 2>&1
