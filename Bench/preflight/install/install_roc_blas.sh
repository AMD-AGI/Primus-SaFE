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
git clone --branch $ROC_TAG --depth 1 "$REPO_URL" >/dev/null


cd "./rocBLAS" || exit 1

# Check if GPU_ARCHS is set
if [ -z "$GPU_ARCHS" ]; then
  echo "Error: GPU_ARCHS environment variable is not set"
  exit 1
fi

echo "Building rocBLAS clients for GPU_ARCHS=$GPU_ARCHS, ROCM_VERSION=$ROCM_VERSION"

# Diagnostics: verify key paths and tools
echo "cmake version: $(cmake --version | head -1)"
echo "rocm path contents:"
ls /opt/rocm/lib/librocblas* 2>/dev/null || echo "  Warning: librocblas not found in /opt/rocm/lib/"
ls /opt/rocm/lib/libhipblaslt* 2>/dev/null || echo "  Warning: libhipblaslt not found in /opt/rocm/lib/"
ls -ld /opt/rocm-* 2>/dev/null || echo "  Warning: no /opt/rocm-* directories found"

chmod +x ./install.sh
./install.sh --clients-only --clients_no_fortran --library-path /opt/rocm --architecture "$GPU_ARCHS" 2>&1 || {
  echo "==== rocBLAS build FAILED - dumping cmake error logs ===="
  for log in /opt/rocBLAS/build/release/CMakeFiles/CMakeError.log \
             /opt/rocBLAS/build/release/CMakeFiles/CMakeOutput.log; do
    if [ -f "$log" ]; then
      echo "==== $(basename "$log") (last 80 lines) ===="
      tail -80 "$log"
    fi
  done
  exit 1
}