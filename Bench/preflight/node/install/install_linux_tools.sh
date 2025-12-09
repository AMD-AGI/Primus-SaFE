
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
current_kernel=$(uname -r)
linux_tools="linux-tools-$current_kernel"
dpkg -l | grep -q "$linux_tools"
if [ $? -ne 0 ]; then
  apt-get update >/dev/null && apt install -y "$linux_tools"  linux-tools-common linux-cloud-tools-$current_kernel >/dev/null
  if [ $? -ne 0 ]; then
    exit 1
  fi
fi
