#!/bin/bash
# Patch Mooncake 01d1eb2a in the SGLang image and install the transfer engine.
# The source tree must still contain the original 2GB shrink.

set -euo pipefail

root="/sgl-workspace/Mooncake"
python3 /tmp/patch-mooncake.py
python3 /tmp/patch-rdma-allocator.py

rm -rf "${root}/build"
cmake -S "${root}" -B "${root}/build" \
    -DUSE_HIP=ON \
    -DUSE_HIP_DMABUF=ON \
    -DENABLE_MULTI_PROTOCOL=ON \
    -DWITH_TE=ON \
    -DWITH_STORE=OFF \
    -DWITH_STORE_RUST=OFF \
    -DBUILD_UNIT_TESTS=OFF \
    -DBUILD_EXAMPLES=OFF \
    -DCMAKE_PREFIX_PATH=/opt/rocm \
    | tee /tmp/mooncake-cmake.log
grep -q "HIP dmabuf MR registration enabled (hsa-runtime64 found)" /tmp/mooncake-cmake.log

cmake --build "${root}/build" -j "$(nproc)"
cmake --install "${root}/build"
# The install directory can land as mode 767. Members of group 1000 then
# cannot search it, and mooncake.engine fails to import.
chmod 0755 /opt/venv/lib/python3.10/site-packages/mooncake
rm -rf "${root}/build"

so="$(/opt/venv/bin/python -c 'import mooncake.engine as engine; print(engine.__file__)')"
# Search the binary directly. grep -q closes a pipe early, and pipefail then
# treats the resulting SIGPIPE as failure.
grep -aq "ibv_reg_dmabuf_mr" "${so}"
grep -aq "Selected HIP memory allocator for protocol" "${so}"
grep -aq "Auto-splitting buffer" "${so}"
grep -aq "MC_DISABLE_HIP" "${so}"
grep -aq "MOONCAKE_DISABLE_HIP_DMABUF" "${so}"
grep -aq "hipDevicePrimaryCtxRelease" "${so}"
if grep -aq "shrink it to" "${so}"; then
    echo "installed engine.so still shrinks oversized buffers" >&2
    exit 1
fi
