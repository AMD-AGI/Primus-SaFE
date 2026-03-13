#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e 

if [ -d "/opt/rocBLAS" ]; then
  exit 0
fi

ROC_TAG=""
if [ "$ROCM_VERSION" = "6.4.3" ]; then
  ROC_TAG="rocm-6.4.3"
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  ROC_TAG="rocm-7.0.2"
elif [ "$ROCM_VERSION" = "7.2.0" ]; then
  ROC_TAG="rocm-7.2.0"
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3, 7.0.3 and 7.2.0 are supported."
  exit 1
fi

REPO_URL="https://github.com/ROCm/rocBLAS.git"
cd /opt
rm -rf rocBLAS
git config --global http.postBuffer 524288000
for i in 1 2 3 4 5; do
  if git clone --branch $ROC_TAG --depth 1 "$REPO_URL" >/dev/null; then
    break
  fi
  echo "Attempt $i failed, retrying in 15s..." >&2
  rm -rf rocBLAS
  sleep 15
done
if [ ! -d "rocBLAS" ]; then
  echo "Error: Failed to clone rocBLAS after 5 attempts" >&2
  exit 1
fi

cd "./rocBLAS" || exit 1

# Check if GPU_ARCHS is set
if [ -z "$GPU_ARCHS" ]; then
  echo "Error: GPU_ARCHS environment variable is not set"
  exit 1
fi

echo "Building rocBLAS clients for GPU_ARCHS=$GPU_ARCHS, ROCM_VERSION=$ROCM_VERSION"
chmod +x ./install.sh && ./install.sh --clients-only --clients_no_fortran --library-path /opt/rocm --architecture "$GPU_ARCHS" >/dev/null