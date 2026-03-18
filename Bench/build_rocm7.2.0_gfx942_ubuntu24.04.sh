#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e
set -o pipefail

get_input_with_default() {
  local prompt="$1"
  local default_value="$2"
  local input
  read -rp "$prompt" input
  input=$(echo "$input" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  if [ -z "$input" ]; then
      echo "$default_value"
  else
      echo "$input"
  fi
}

# Prompt user for image version (default: current date YYYYMMDD)
DEFAULT_VERSION=$(date +%Y%m%d%H%M)
IMAGE_VERSION=$(get_input_with_default "Enter image version(${DEFAULT_VERSION}): " "${DEFAULT_VERSION}")
GPU_ARCHS=gfx942
ROCM_VERSION=7.2.0
OS_VERSION=24.04
OS_NAME=ubuntu
PY_VERSION=3.12

# Build docker image (--progress=plain shows full output for debugging)
# Build log saved to build.log in current directory
docker buildx build . -f ./Dockerfile \
  --build-arg ROCM_VERSION=${ROCM_VERSION} \
  --build-arg GPU_ARCHS="${GPU_ARCHS}" \
  --build-arg OS_VERSION="${OS_VERSION}" \
  --build-arg OS_NAME="${OS_NAME}" \
  --build-arg PY_VERSION="${PY_VERSION}" \
  -t primussafe/primusbench:rocm${ROCM_VERSION}_${GPU_ARCHS}_${OS_NAME}${OS_VERSION}_${IMAGE_VERSION} 2>&1 | tee build.log

