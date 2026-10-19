#!/bin/bash
# Patch Mooncake 01d1eb2a in the SGLang image and install the transfer engine.
# The source tree must still contain the original 2GB shrink.

set -euo pipefail

root="/sgl-workspace/Mooncake"
python3 /tmp/patch-mooncake.py

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
rm -rf "${root}/build"

so="$(/opt/venv/bin/python -c 'import mooncake.engine as engine; print(engine.__file__)')"
strings "${so}" | grep -q "ibv_reg_dmabuf_mr"
strings "${so}" | grep -q "Auto-splitting buffer"
strings "${so}" | grep -q "MC_DISABLE_HIP"
strings "${so}" | grep -q "MOONCAKE_DISABLE_HIP_DMABUF"
if strings "${so}" | grep -q "shrink it to"; then
    echo "installed engine.so still shrinks oversized buffers" >&2
    exit 1
fi
