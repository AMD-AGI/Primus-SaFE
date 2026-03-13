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
cd /opt
rm -rf rccl-tests
git config --global http.postBuffer 524288000
for i in 1 2 3 4 5; do
  if git clone "$REPO_URL" >/dev/null; then
    break
  fi
  echo "Attempt $i failed, retrying in 15s..." >&2
  rm -rf rccl-tests
  sleep 15
done
if [ ! -d "rccl-tests" ]; then
  echo "Error: Failed to clone rccl-tests after 5 attempts" >&2
  exit 1
fi

cd "./rccl-tests" || exit 1
make -j 16 MPI=1 MPI_HOME=/opt/mpich NCCL_HOME=/opt/rccl/build > /dev/null

echo "============== install rccl-tests successfully =============="