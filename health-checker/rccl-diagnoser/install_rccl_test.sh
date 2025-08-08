#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

REPO_URL="https://github.com/ROCm/rccl-tests.git"
cd "/root" && git clone "$REPO_URL"
if [ $? -ne 0 ]; then
  exit 1
fi
cd "/root/rccl-tests" || exit 1
make MPI=1 MPI_HOME=/usr/lib/x86_64-linux-gnu/openmpi NCCL_HOME=/opt/rocm/
if [ $? -ne 0 ]; then
  exit 1
fi