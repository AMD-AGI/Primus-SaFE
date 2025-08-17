#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

cd depends

pip3 install -r requirements.txt > /dev/null
if [ $? -ne 0 ]; then
  echo "failed to install python package"
  exit 1
fi

SCRIPTS_TO_RUN=(
    "install_cmake.sh"
    "install_rocm.sh"
    "install_rccl.sh"
    "install_mpich.sh"
    "install_rccl_test.sh"
    "install_bnxt_driver.sh"
)

for script in "${SCRIPTS_TO_RUN[@]}"; do
  bash "$script"
  if [ $? -ne 0 ]; then
    echo "failed to run $script"
    exit 1
  fi
done
