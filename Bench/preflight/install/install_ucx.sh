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
git config --global http.postBuffer 524288000

for attempt in 1 2 3 4 5; do
  rm -rf ${UCX_SRC_DIR}
  echo "Cloning UCX repository (attempt $attempt)..."
  if git clone ${UCX_REPO} ${UCX_SRC_DIR} && \
     cd ${UCX_SRC_DIR} && \
     git checkout ${UCX_VERSION} && \
     ./autogen.sh >/dev/null && \
     mkdir -p build && cd build && \
     ../configure --prefix=${INSTALL_PREFIX} --with-rocm=/opt/rocm >/dev/null && \
     make -j $(nproc) >/dev/null && \
     make install >/dev/null; then
    cd ${WORKDIR} && rm -rf ${UCX_SRC_DIR}
    echo "============== install UCX ${UCX_VERSION} successfully =============="
    echo "UCX installed to: ${INSTALL_PREFIX}"
    echo "Add to LD_LIBRARY_PATH: export LD_LIBRARY_PATH=${INSTALL_PREFIX}/lib:\$LD_LIBRARY_PATH"
    exit 0
  fi
  echo "Attempt $attempt failed, retrying in 15s..." >&2
  cd ${WORKDIR}
  sleep 15
done
echo "Error: Failed to clone/build UCX after 5 attempts" >&2
exit 1