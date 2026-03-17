#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

REPO_URL="https://github.com/hpc/ior.git"
WORKDIR="/opt"
cd ${WORKDIR}

echo "Installing IOR..."
rm -rf ior
git config --global http.postBuffer 524288000
for i in 1 2 3 4 5; do
  if git clone ${REPO_URL} && cd ior && \
     git checkout remotes/origin/4.0 && \
     ./bootstrap && ./configure --prefix="/opt" && make && make install; then
    cd ${WORKDIR} && rm -rf ior
    echo "IOR installed successfully"
    exit 0
  fi
  echo "Attempt $i failed, retrying in 15s..." >&2
  cd ${WORKDIR}
  rm -rf ior
  sleep 15
done
echo "Error: Failed to clone/build IOR after 5 attempts" >&2
exit 1
