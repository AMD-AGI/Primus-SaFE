#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "begin to install rccl"

git clone https://github.com/ROCm/rccl
if [ $? -ne 0 ]; then
  exit 1
fi

cd rccl
bash ./install.sh -l
if [ $? -ne 0 ]; then
  exit 1
fi
echo "install rccl successfully"
