#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install rocm-$ROCM_VERSION =============="
set -e

# Set the download URL and filename based on ROCM_VERSION
if [ "$ROCM_VERSION" = "6.4.3" ]; then
  AMDGPU_INSTALL_FILE="amdgpu-install_6.4.60403-1_all.deb"
elif [ "$ROCM_VERSION" = "7.0.3" ]; then
  AMDGPU_INSTALL_FILE="amdgpu-install_7.0.3.70003-1_all.deb"
else
  echo "Error: Unsupported ROCM_VERSION '$ROCM_VERSION'. Only 6.4.3 and 7.0.3 are supported."
  exit 1
fi

AMDGPU_INSTALL_URL="https://repo.radeon.com/amdgpu-install/$ROCM_VERSION/ubuntu/jammy/$AMDGPU_INSTALL_FILE"

echo "Downloading $AMDGPU_INSTALL_FILE..."
wget -q $AMDGPU_INSTALL_URL
if [ $? -ne 0 ]; then
  echo "Error: Failed to download $AMDGPU_INSTALL_URL"
  exit 1
fi

echo "Installing $AMDGPU_INSTALL_FILE..."
apt update > /dev/null
apt install -y ./$AMDGPU_INSTALL_FILE > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to install $AMDGPU_INSTALL_FILE"
  exit 1
fi

echo "Installing ROCm $ROCM_VERSION..."
apt update > /dev/null
apt install -y rocm > /dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to install ROCm"
  exit 1
fi

echo "Cleaning up..."
rm -f ./$AMDGPU_INSTALL_FILE
echo "============== install rocm-$ROCM_VERSION successfully =============="#