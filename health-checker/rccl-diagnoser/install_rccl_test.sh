#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
echo "begin to install rccl-tests"

REPO_URL="https://github.com/ROCm/rccl-tests.git"
cd "/opt" && git clone "$REPO_URL" > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi
cd "/opt/rccl-tests" || exit 1
make -j8 MPI=1 MPI_HOME=/opt/openmpi-4.1.8 NCCL_HOME=/opt/rocm > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi

echo "install rccl-tests successfully"