#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

cd "$(dirname "${BASH_SOURCE[0]}")" || { echo "failed to find primus-safe/pytorch"; exit 1; }
echo "Using ROCM_VERSION: $ROCM_VERSION"
echo "Using GPU_ARCHS: $GPU_ARCHS"

PIP_EXTRA_ARGS=""
if [ "$OS_VERSION" = "24.04" ]; then
  PIP_EXTRA_ARGS="--break-system-packages"
fi

if [ "$ROCM_VERSION" = "6.4.3" ]; then
  pip3 install torch torchvision --index-url https://download.pytorch.org/whl/rocm6.4 $PIP_EXTRA_ARGS
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  pip3 install torch torchvision --pre --index-url https://download.pytorch.org/whl/nightly/rocm7.0 $PIP_EXTRA_ARGS
elif [ "$ROCM_VERSION" = "7.2.0" ]; then
  RADEON_REPO="https://repo.radeon.com/rocm/manylinux/rocm-rel-7.2"
  pip3 install torch torchvision \
    --no-deps --no-index --find-links ${RADEON_REPO} $PIP_EXTRA_ARGS && \
  pip3 install torch torchvision \
    --find-links ${RADEON_REPO} $PIP_EXTRA_ARGS
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3, 7.0.3 and 7.2.0 are supported."
  exit 1
fi

if [ $? -ne 0 ]; then
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
