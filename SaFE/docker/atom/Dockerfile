# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Packages ATOM source and atomesh on a published SGLang or vLLM ROCm image.
# Select the base with ENGINE=sglang or ENGINE=vllm.
ARG ENGINE=sglang
ARG SGLANG_BASE=docker.io/lmsysorg/sglang-rocm:v0.5.20-rocm10-mi35x-20260920
ARG VLLM_BASE=docker.io/vllm/vllm-openai-rocm:v0.29.0
ARG BASE_IMAGE=${SGLANG_BASE}

FROM ${BASE_IMAGE}

ARG ENGINE=sglang
ARG ATOM_REPO=https://github.com/ROCm/ATOM.git
ARG ATOM_REF=main
ARG RUST_VERSION=1.94.0
ARG ATOM_MESH_FEATURES=""

ENV ATOM_ENGINE=${ENGINE}
ENV ATOM_MESH_BACKEND=${ENGINE}
ENV ATOM_ROOT=/opt/ATOM
ENV PATH="/root/.cargo/bin:${PATH}"
ENV PYTHONPATH="/opt/ATOM"

# A symlink to /opt/venv/bin/python resolves to the system interpreter and
# drops the venv. Invoke the venv binary directly so pip stays in that env.
RUN printf '%s\n' \
        '#!/bin/sh' \
        'if [ -x /opt/venv/bin/python ]; then exec /opt/venv/bin/python "$@"; fi' \
        'exec python3 "$@"' \
        > /usr/local/bin/atom-python \
    && chmod 0755 /usr/local/bin/atom-python
ENV PYO3_PYTHON=/opt/venv/bin/python

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        build-essential ca-certificates curl git pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/* \
    && curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
        | sh -s -- -y --default-toolchain "${RUST_VERSION}" --profile minimal \
    && rustc --version && cargo --version

RUN git clone --depth 1 --branch "${ATOM_REF}" "${ATOM_REPO}" /opt/ATOM \
    && cd /opt/ATOM \
    && ATOM_MESH_BUILD=1 ATOM_MESH_FEATURES="${ATOM_MESH_FEATURES}" \
        /usr/local/bin/atom-python -m pip install --no-cache-dir -e . --no-deps \
    && /usr/local/bin/atom-python -m pip install --no-cache-dir \
        pybind11 pyzmq msgspec xxhash "fastapi>=0.115,<0.137" psutil \
        "prometheus_client>=0.25,<0.26" protobuf uvicorn uvloop aiohttp \
        datasets openpyxl tqdm setproctitle "transformers==5.16.1" \
    && strip /opt/ATOM/atom/mesh/target/release/atomesh \
    && cp /opt/ATOM/atom/mesh/target/release/atomesh /usr/local/bin/atomesh \
    && atomesh --version \
    && /usr/local/bin/atom-python -c "import atom; print(atom.__file__)"

COPY entrypoint.sh /usr/local/bin/atom-entrypoint
RUN chmod 0755 /usr/local/bin/atom-entrypoint

WORKDIR /opt/ATOM
ENTRYPOINT ["/usr/local/bin/atom-entrypoint"]
