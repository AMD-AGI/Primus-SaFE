#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

REPO_URL="https://github.com/UoB-HPC/BabelStream.git"
cd /opt
git clone --branch v5.0 "$REPO_URL" >/dev/null
if [ $? -ne 0 ]; then
  echo "failed to clone babel_stream " >&2
  exit 1
fi