#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

cd /opt/primus-safe/pytorch
if [ $? -ne 0 ]; then
  echo "failed to find primus-safe/pytorch "
  exit 1
fi

pip3 install torch torchvision --index-url https://download.pytorch.org/whl/rocm6.4
if [ $? -ne 0 ]; then
  echo "failed to install torch package"
  exit 1
fi

SCRIPTS_TO_RUN=(
    "install_cmake.sh"
    "install_rocm.sh"
    "install_rccl.sh"
    "install_mpich.sh"
    "install_rccl_test.sh"
    "install_rdma_test.sh"
)

for script in "${SCRIPTS_TO_RUN[@]}"; do
  bash "$script"
  if [ $? -ne 0 ]; then
    echo "failed to run $script"
    exit 1
  fi
done