#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

echo "============== begin to install Open MPI 4.1.6 =============="
set -e

OPENMPI_VERSION="4.1.6"
OPENMPI_URL="https://download.open-mpi.org/release/open-mpi/v4.1/openmpi-${OPENMPI_VERSION}.tar.gz"
OPENMPI_DIR="ompi-${OPENMPI_VERSION}"
OPENMPI_TARBALL="openmpi-${OPENMPI_VERSION}.tar.gz"
WORKDIR="/opt"

cd ${WORKDIR}

# Download Open MPI
echo "Downloading Open MPI ${OPENMPI_VERSION}..."
wget -q ${OPENMPI_URL} >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to download Open MPI from ${OPENMPI_URL}"
  exit 1
fi

# Extract tarball
echo "Extracting ${OPENMPI_TARBALL}..."
mkdir -p ${OPENMPI_DIR}
tar -zxf ${OPENMPI_TARBALL} -C ${OPENMPI_DIR} --strip-components=1 >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to extract ${OPENMPI_TARBALL}"
  exit 1
fi

# Configure
echo "Configuring Open MPI..."
cd ${OPENMPI_DIR}
mkdir -p build
cd build
../configure --with-ucx=/opt/ucx --prefix=/opt/openmpi \
    --disable-oshmem --disable-mpi-fortran >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to configure Open MPI."
  exit 1
fi

# Build
echo "Building Open MPI (this may take a while)..."
make -j 16 >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to build Open MPI."
  exit 1
fi

# Install
echo "Installing Open MPI..."
make install >/dev/null
if [ $? -ne 0 ]; then
  echo "Error: Failed to install Open MPI"
  exit 1
fi

# Cleanup
echo "Cleaning up temporary files..."
cd ${WORKDIR}
rm -f ${OPENMPI_TARBALL}
rm -rf ${OPENMPI_DIR}

echo "============== install Open MPI ${OPENMPI_VERSION} successfully =============="