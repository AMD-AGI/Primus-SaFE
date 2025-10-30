#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1

SCRIPTS_TO_RUN=(
    "install_mpich.sh"
    "install_rccl_test.sh"
    "install_rdma_test.sh"
    "install_bnxt_driver.sh"
)

for script in "${SCRIPTS_TO_RUN[@]}"; do
  bash "$script"
  if [ $? -ne 0 ]; then
    echo "failed to run $script"
    exit 1
  fi
done