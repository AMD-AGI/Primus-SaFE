#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
echo "==============  begin to install rdma-tests =============="
set -e
REPO_URL="https://github.com/ROCm/rdma-perftest.git"
cd "/tmp" && git clone "$REPO_URL" > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi

cd "./rdma-perftest" || exit 1
./autogen.sh && ./configure --prefix=/opt/rdma-perftest && make && make install
if [ $? -ne 0 ]; then
  exit 1
fi
rm -rf /tmp/rdma-perftest
echo "============== install rdma-tests successfully =============="