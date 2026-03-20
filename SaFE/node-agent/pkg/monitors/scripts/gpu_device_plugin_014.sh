#!/bin/bash

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -o pipefail

# Check device plugin sockets on host node via nsenter.
# kubelet.sock: kubelet device plugin registration socket (created by kubelet)
# amd.com_gpu: AMD GPU device plugin socket (created when AMD device plugin registers)
DEVICE_PLUGINS_DIR="/var/lib/kubelet/device-plugins"
KUBELET_SOCK="${DEVICE_PLUGINS_DIR}/kubelet.sock"
AMD_GPU_SOCK="${DEVICE_PLUGINS_DIR}/amd.com_gpu"

NSENTER="nsenter --target 1 --mount --uts --ipc --net --pid --"

# Check if device-plugins directory exists and is accessible
if ! $NSENTER ls "$DEVICE_PLUGINS_DIR" >/dev/null 2>&1; then
  echo "Error: device-plugins directory $DEVICE_PLUGINS_DIR does not exist or is not accessible"
  exit 1
fi

# Check kubelet.sock (required for device plugin registration)
if ! $NSENTER ls "$KUBELET_SOCK" >/dev/null 2>&1; then
  echo "Error: kubelet device plugin socket $KUBELET_SOCK does not exist"
  exit 1
fi

# Check AMD GPU device plugin socket
if ! $NSENTER ls "$AMD_GPU_SOCK" >/dev/null 2>&1; then
  echo "Error: AMD GPU device plugin socket $AMD_GPU_SOCK does not exist"
  exit 1
fi

exit 0
