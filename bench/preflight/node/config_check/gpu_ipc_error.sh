#!/bin/bash

# Search for the specific error in dmesg
if nsenter --target 1 --mount --uts --ipc --net --pid -- dmesg -T | grep "amdgpu: Failed to import IPC handle"; then
  exit 1
fi