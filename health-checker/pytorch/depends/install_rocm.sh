#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install rocm-6.4.3 =============="

wget -q https://repo.radeon.com/amdgpu-install/6.4.3/ubuntu/jammy/amdgpu-install_6.4.60403-1_all.deb
if [ $? -ne 0 ]; then
  exit 1
fi

apt update > /dev/null
apt install -y ./amdgpu-install_6.4.60403-1_all.deb > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi

apt install -y rocm > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi

echo "============== install rocm-6.4.3 successfully =============="#