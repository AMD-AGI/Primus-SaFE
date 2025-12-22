#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -e

# Configuration
# Default to v54.0 to match libionic driver version 54.3
RDMA_CORE_VERSION="${RDMA_CORE_VERSION:-v54.3}"
INSTALL_DIR="/usr"
BUILD_DIR="/tmp/rdma-core-build"

echo "========================================"
echo "Installing rdma-core ${RDMA_CORE_VERSION}"
echo "========================================"

# Clean up previous build directory
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"
cd "${BUILD_DIR}"

# Clone rdma-core
echo "[INFO] Cloning rdma-core ${RDMA_CORE_VERSION}..."
git clone --depth 1 --branch "${RDMA_CORE_VERSION}" https://github.com/linux-rdma/rdma-core.git
cd rdma-core

# Build
echo "[INFO] Building rdma-core..."
mkdir build && cd build
cmake -GNinja \
    -DCMAKE_INSTALL_PREFIX="${INSTALL_DIR}" \
    -DCMAKE_BUILD_TYPE=Release \
    -DNO_MAN_PAGES=1 \
    ..

ninja -j$(nproc)

# Install
echo "[INFO] Installing rdma-core..."
ninja install

# Update library cache
ldconfig

# Show installed version
echo "[INFO] Installed libibverbs version:"
pkg-config --modversion libibverbs 2>/dev/null || echo "pkg-config info not available"

# Cleanup
echo "[INFO] Cleaning up build directory..."
rm -rf "${BUILD_DIR}"

echo "========================================"
echo "rdma-core installation complete!"
echo "========================================"
