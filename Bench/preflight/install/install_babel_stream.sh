#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

REPO_URL="https://github.com/UoB-HPC/BabelStream.git"
cd /opt
rm -rf BabelStream
# Retry git clone on transient network errors (e.g. GnuTLS recv error, early EOF)
git config --global http.postBuffer 524288000
for i in 1 2 3 4 5; do
  if git clone --branch v5.0 "$REPO_URL"; then
    break
  fi
  echo "Attempt $i failed, retrying in 15s..." >&2
  rm -rf BabelStream
  sleep 15
done
if [ ! -d "BabelStream" ]; then
  echo "failed to clone babel_stream " >&2
  exit 1
fi