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

# Pre-download blis to avoid GitHub release asset JWT expiry during rocBLAS install.sh
# (rocBLAS install.sh downloads blis; the redirect URL's JWT can expire on slow builds)
if [ ! -e "build/deps/blis/lib/libblis.a" ] && [ ! -e "/usr/local/lib/libblis.a" ]; then
  echo "Pre-downloading AOCL BLIS for rocBLAS clients..."
  mkdir -p build/deps && cd build/deps
  BLIS_URL="https://github.com/amd/blis/releases/download/2.0/aocl-blis-mt-ubuntu-2.0.tar.gz"
  for i in 1 2 3 4 5; do
    rm -rf blis blis.tar.gz amd-blis-mt
    if wget -nv -O blis.tar.gz "$BLIS_URL" 2>/dev/null && [ -s blis.tar.gz ]; then
      tar -xvf blis.tar.gz
      BLIS_DIR=$(tar -tf blis.tar.gz | head -1 | cut -d/ -f1)
      mv "$BLIS_DIR" blis
      rm -f blis.tar.gz
      cd blis/lib && ln -sf libblis-mt.a libblis.a && cd ../..
      echo "BLIS pre-downloaded successfully"
      break
    fi
    echo "Attempt $i failed, retrying in 15s..." >&2
    sleep 15
  done
  if [ ! -e "blis/lib/libblis.a" ]; then
    echo "Error: Failed to pre-download BLIS after 5 attempts (GitHub JWT may have expired)" >&2
    exit 1
  fi
  cd /opt/rocBLAS
fi

echo "Building rocBLAS clients for GPU_ARCHS=$GPU_ARCHS, ROCM_VERSION=$ROCM_VERSION"
chmod +x ./install.sh && ./install.sh --clients-only --clients_no_fortran --library-path /opt/rocm --architecture "$GPU_ARCHS" >/dev/null