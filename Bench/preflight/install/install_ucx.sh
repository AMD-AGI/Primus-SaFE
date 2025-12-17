#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install UCX 1.18.0 =============="
set -e

UCX_VERSION="1.18.0"
UCX_URL="https://github.com/openucx/ucx/releases/download/v${UCX_VERSION}/ucx-${UCX_VERSION}.tar.gz"
UCX_DIR="ucx-${UCX_VERSION}"
UCX_TARBALL="ucx-${UCX_VERSION}.tar.gz"
WORKDIR="/opt"

cd ${WORKDIR}

# Download UCX
echo "Downloading UCX ${UCX_VERSION}..."
wget -q ${UCX_URL}
if [ $? -ne 0 ]; then
  echo "Error: Failed to download UCX from ${UCX_URL}"
  exit 1
fi

# Extract tarball
echo "Extracting ${UCX_TARBALL}..."
mkdir -p ${UCX_DIR}
tar -zxf ${UCX_TARBALL} -C ${UCX_DIR} --strip-components=1
if [ $? -ne 0 ]; then
  echo "Error: Failed to extract ${UCX_TARBALL}"
  exit 1
fi

# Configure
echo "Configuring UCX..."
cd ${UCX_DIR}
mkdir -p build
cd build
../configure --prefix=${WORKDIR}/ucx --with-rocm=/opt/rocm 2>&1 | tee log_ucx_configure.txt
if [ ${PIPESTATUS[0]} -ne 0 ]; then
  echo "Error: Failed to configure UCX. See log_ucx_configure.txt for details."
  exit 1
fi

# Build
echo "Building UCX (this may take a while)..."
make -j 16 2>&1 | tee log_ucx_build.txt
if [ ${PIPESTATUS[0]} -ne 0 ]; then
  echo "Error: Failed to build UCX. See log_ucx_build.txt for details."
  exit 1
fi

# Install
echo "Installing UCX..."
make install
if [ $? -ne 0 ]; then
  echo "Error: Failed to install UCX"
  exit 1
fi


echo "============== install UCX ${UCX_VERSION} successfully =============="