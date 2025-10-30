#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
switch=`nsenter --target 1 --mount --uts --ipc --net --pid -- /usr/sbin/sysctl -a |grep "kernel.numa_balancing "|awk -F"=" '{print $NF}'`
switch="${switch// /}"
if [ $switch -ne 0 ]; then
   echo "kernel.numa_balancing is not properly configured"
   exit 1
fi