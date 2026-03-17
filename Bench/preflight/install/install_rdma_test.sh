#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

if [ -d "/opt/rdma-perftest" ]; then
  exit 0
fi

echo "==============  begin to install rdma-tests =============="
REPO_URL="https://github.com/ROCm/rdma-perftest.git"
cd /tmp
rm -rf rdma-perftest
git config --global http.postBuffer 524288000
for i in 1 2 3 4 5; do
  if git clone "$REPO_URL" >/dev/null; then
    break
  fi
  echo "Attempt $i failed, retrying in 15s..." >&2
  rm -rf rdma-perftest
  sleep 15
done
if [ ! -d "rdma-perftest" ]; then
  echo "Error: Failed to clone rdma-perftest after 5 attempts" >&2
  exit 1
fi

cd "./rdma-perftest" || exit 1
./autogen.sh && ./configure --prefix=/opt/rdma-perftest && make && make install > /dev/null
if [ $? -ne 0 ]; then
  exit 1
fi
rm -rf /tmp/rdma-perftest
echo "============== install rdma-tests successfully =============="