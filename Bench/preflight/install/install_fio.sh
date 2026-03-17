#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

REPO_URL="https://github.com/axboe/fio.git"
WORKDIR="/opt"
cd ${WORKDIR}

echo "Installing FIO..."
rm -rf fio
git config --global http.postBuffer 524288000
for i in 1 2 3 4 5; do
  if git clone --depth=1 ${REPO_URL} && cd fio && \
     ./configure --prefix="/root" && make && make install; then
    cd ${WORKDIR} && rm -rf fio
    echo "FIO installed successfully"
    exit 0
  fi
  echo "Attempt $i failed, retrying in 15s..." >&2
  cd ${WORKDIR}
  rm -rf fio
  sleep 15
done
echo "Error: Failed to clone/build FIO after 5 attempts" >&2
exit 1
