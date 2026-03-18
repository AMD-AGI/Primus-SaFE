#!/bin/bash

# NIC configuration for oci environments only.

version="1.0"
prog=$(realpath $0)

echo "Script ${prog} version:${version}"

sysctl -w net.ipv4.neigh.default.gc_thresh1=262144
sysctl -w net.ipv4.neigh.default.gc_thresh2=262144
sysctl -w net.ipv4.neigh.default.gc_thresh3=262144

nicctl debug update pipeline internal rdma --skip-data-copy disable

sudo <<'EOF'
#echo 1 > /proc/sys/net/ipv4/conf/all/arp_filter
echo 0 > /proc/sys/net/ipv4/conf/all/rp_filter
echo 1 > /proc/sys/net/ipv4/conf/all/accept_local
echo 1 > /proc/sys/net/ipv4/conf/all/arp_filter
echo 1 > /proc/sys/net/ipv4/conf/all/arp_ignore
echo 2 > /proc/sys/net/ipv4/conf/all/arp_announce
EOF

echo "Reading arp settings after modification"
grep -r  . /proc/sys/net/ipv4/conf/all/ | grep 'arp_\|accept_local\|_filter'

#time-sync Host-clock to NIC-clock
nicctl update card time

echo "Script $0 version:${version} done"
