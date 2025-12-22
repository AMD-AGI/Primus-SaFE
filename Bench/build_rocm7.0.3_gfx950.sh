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
DEFAULT_VERSION=$(date +%Y%m%d%H%M)
IMAGE_VERSION=$(get_input_with_default "Enter image version(${DEFAULT_VERSION}): " "${DEFAULT_VERSION}")
AINIC_BUNDLE_PATH=$(get_input_with_default "Enter ainic package path (empty to skip): " "")
GPU_ARCHS=gfx950

# Copy AINIC bundle to build context if provided
AINIC_FILENAME=""
if [ -n "${AINIC_BUNDLE_PATH}" ] && [ -f "${AINIC_BUNDLE_PATH}" ]; then
  AINIC_FILENAME=$(basename "${AINIC_BUNDLE_PATH}")
  cp "${AINIC_BUNDLE_PATH}" "./preflight/install/${AINIC_FILENAME}"
  echo "Copied AINIC bundle to ./preflight/install/${AINIC_FILENAME}"
fi

# Build docker image (--progress=plain shows full output for debugging)
# Build log saved to build.log in current directory
docker buildx build . -f ./Dockerfile \
  --build-arg ROCM_VERSION=7.0.3 \
  --build-arg GPU_ARCHS="${GPU_ARCHS}" \
  --build-arg AINIC_BUNDLE_FILENAME="${AINIC_FILENAME}" \
  -t primussafe/primusbench:rocm7.0.3_${GPU_ARCHS}_ainic_${IMAGE_VERSION} 2>&1 | tee build.log

# Cleanup: remove copied AINIC file after build
if [ -n "${AINIC_FILENAME}" ] && [ -f "./preflight/install/${AINIC_FILENAME}" ]; then
  rm -f "./preflight/install/${AINIC_FILENAME}"
  echo "Cleaned up ./preflight/install/${AINIC_FILENAME}"
fi