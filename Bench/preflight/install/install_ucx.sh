#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

UCX_VERSION="${UCX_VERSION:-v1.18.0}"
UCX_REPO="https://github.com/openucx/ucx.git"
UCX_SRC_DIR="ucx-src"  # Source directory (will be deleted after install)
WORKDIR="/opt"
INSTALL_PREFIX="${WORKDIR}/ucx"  # Install directory (will be kept)

echo "============== begin to install UCX ${UCX_VERSION} =============="

cd ${WORKDIR}

# Clone UCX repository
echo "Cloning UCX repository..."
if [ -d "${UCX_SRC_DIR}" ]; then
  echo "Removing existing UCX source directory..."
  rm -rf ${UCX_SRC_DIR}
fi

git clone ${UCX_REPO} ${UCX_SRC_DIR}
if [ $? -ne 0 ]; then
  echo "Error: Failed to clone UCX repository from ${UCX_REPO}"
  exit 1
fi

# Checkout specific version
echo "Checking out version ${UCX_VERSION}..."
cd ${UCX_SRC_DIR}
git checkout ${UCX_VERSION}
if [ $? -ne 0 ]; then
  echo "Error: Failed to checkout version ${UCX_VERSION}"
  exit 1
fi

# Generate configure script
echo "Running autogen.sh..."
./autogen.sh  >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to run autogen.sh"
  exit 1
fi

# Configure
echo "Configuring UCX..."
mkdir -p build && cd build
../configure --prefix=${INSTALL_PREFIX} --with-rocm=/opt/rocm >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to configure UCX."
  exit 1
fi

# Build
echo "Building UCX (this may take a while)..."
make -j $(nproc)  >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to build UCX."
  exit 1
fi

# Install
echo "Installing UCX..."
make install  >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to install UCX"
  exit 1
fi

# Cleanup source directory (keep install directory)
cd ${WORKDIR}
rm -rf ${UCX_SRC_DIR}

echo "============== install UCX ${UCX_VERSION} successfully =============="
echo "UCX installed to: ${INSTALL_PREFIX}"
echo "Add to LD_LIBRARY_PATH: export LD_LIBRARY_PATH=${INSTALL_PREFIX}/lib:\$LD_LIBRARY_PATH"