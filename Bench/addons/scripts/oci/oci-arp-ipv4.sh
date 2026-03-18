#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

sysctl -w net.ipv4.neigh.default.gc_thresh1=262144
sysctl -w net.ipv4.neigh.default.gc_thresh2=262144
sysctl -w net.ipv4.neigh.default.gc_thresh3=262144
nicctl debug update pipeline internal rdma --skip-data-copy disable

echo 0 > /proc/sys/net/ipv4/conf/all/rp_filter
echo 1 > /proc/sys/net/ipv4/conf/all/accept_local
echo 1 > /proc/sys/net/ipv4/conf/all/arp_filter
echo 1 > /proc/sys/net/ipv4/conf/all/arp_ignore
echo 2 > /proc/sys/net/ipv4/conf/all/arp_announce

grep -r  . /proc/sys/net/ipv4/conf/all/ | grep 'arp_\|accept_local\|_filter'
nicctl update card time