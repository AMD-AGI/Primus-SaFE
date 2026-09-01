#!/usr/bin/env bash
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Every test for preflight/network. No cluster, no RCCL, no IB -- these are
# pure functions fed recorded output shapes.
#
#   bash preflight/network/tests/run_tests.sh

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

echo "== verdict.sh =="
bash tests/test_verdict.sh

echo
echo "== python =="
python3 -m unittest discover -s tests -t . "$@"
