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

DEFAULT_VERSION=$(date +%Y%m%d)
IMAGE_VERSION=$(get_input_with_default "Enter image version(${DEFAULT_VERSION}): " "${DEFAULT_VERSION}")

docker buildx build . -f ./Dockerfile \
  --progress=plain \
  --build-arg ROCM_VERSION=6.4.3 \
  -t primussafe/primusbench:${IMAGE_VERSION} | tee build.log
