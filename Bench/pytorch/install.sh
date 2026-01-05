#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1

if [ $? -ne 0 ]; then
  echo "failed to find primus-safe/pytorch "
  exit 1
fi
export ROCM_VERSION=$ROCM_VERSION
export GPU_ARCHS=$GPU_ARCHS
echo "Using ROCM_VERSION: $ROCM_VERSION"
echo "Using GPU_ARCHS: $GPU_ARCHS"

return_code=0
if [ "$ROCM_VERSION" = "6.4.3" ]; then
  return_code=$(pip3 install torch torchvision --index-url https://download.pytorch.org/whl/rocm6.4)
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  echo "Installing PyTorch for ROCm 7.0..."
  return_code=$(pip3 install torch torchvision --pre --index-url https://download.pytorch.org/whl/nightly/rocm7.0)
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3 and 7.0.3 are supported."
  exit 1
fi

if [ $return_code -ne 0 ]; then
  echo "failed to install torch package"
  exit 1
fi


SCRIPTS_TO_RUN=(
    "install_cmake.sh"
    "install_rocm.sh"
    "install_rccl.sh"
)

for script in "${SCRIPTS_TO_RUN[@]}"; do
  bash "$script"
  if [ $? -ne 0 ]; then
    echo "failed to run $script"
    exit 1
  fi
done
