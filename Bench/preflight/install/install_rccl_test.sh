#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

if [ -d "/opt/rccl-tests" ]; then
  exit 0
fi

echo "==============  begin to install rccl-tests =============="

REPO_URL="https://github.com/ROCm/rccl-tests.git"
cd /opt && git clone "$REPO_URL" >/dev/null

cd "./rccl-tests" || exit 1
make -j 16 MPI=1 MPI_HOME=/opt/mpich NCCL_HOME=/opt/rccl/build > /dev/null

echo "============== install rccl-tests successfully =============="