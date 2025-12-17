#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

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
DEFAULT_VERSION=$(date +%Y%m%d)
IMAGE_VERSION=$(get_input_with_default "Enter image version(${DEFAULT_VERSION}): " "${DEFAULT_VERSION}")
AINIC_BUNDLE_PATH=$(get_input_with_default "Enter ainic package path (empty to disable ainic): " "")

docker buildx build . -f ./Dockerfile \
  --build-arg ROCM_VERSION=7.0.3 \
  --build-arg AINIC_BUNDLE_PATH="${AINIC_BUNDLE_PATH}" \
  -t primussafe/primusbench:${IMAGE_VERSION}