# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Packages ATOM source and atomesh on a published SGLang or vLLM ROCm image.
# Select the base with ENGINE=sglang or ENGINE=vllm.
# The SGLang base rebuilds Mooncake 01d1eb2a with chunked HIP dma-buf registration
# and realizes the scheduler HIP streams before that registration.
# sshd starts only when MN_SSH_AUTHORIZED_KEY is set. With no user command the container runs tail -f /dev/null.
# /tmp/aiter_configs is emptied and mode 1777 so a non-root process can rewrite it.
ARG ENGINE=sglang
ARG SGLANG_BASE=docker.io/lmsysorg/sglang-rocm:v0.5.21-rocm720-mi35x-20261008
ARG VLLM_BASE=docker.io/vllm/vllm-openai-rocm:v0.29.0
ARG BASE_IMAGE=${SGLANG_BASE}

FROM ${BASE_IMAGE}

ARG ENGINE=sglang
ARG ATOM_REPO=https://github.com/ROCm/ATOM.git
# Commit on ROCm/ATOM main. A branch name would move between image builds.
ARG ATOM_REF=53e4fb52403707ab07b69ee909501ea3cc25ca5c
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
        openssh-server openssh-client openssh-sftp-server \
    && rm -rf /var/lib/apt/lists/* \
    && curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \
        | sh -s -- -y --default-toolchain "${RUST_VERSION}" --profile minimal \
    && rustc --version && cargo --version

COPY patch-mooncake.py /tmp/patch-mooncake.py
COPY patch-rdma-allocator.py /tmp/patch-rdma-allocator.py
COPY patch-sglang-streams.py /tmp/patch-sglang-streams.py
COPY build-mooncake.sh /tmp/build-mooncake.sh
# The vLLM base has no /sgl-workspace/Mooncake tree.
RUN if [ "${ENGINE}" = "sglang" ]; then \
        bash /tmp/build-mooncake.sh; \
        python3 /tmp/patch-sglang-streams.py; \
    fi \
    && rm -f /tmp/patch-mooncake.py /tmp/patch-rdma-allocator.py \
        /tmp/patch-sglang-streams.py /tmp/build-mooncake.sh /tmp/mooncake-cmake.log

RUN git init /opt/ATOM \
    && git -C /opt/ATOM remote add origin "${ATOM_REPO}" \
    && git -C /opt/ATOM fetch --depth 1 origin "${ATOM_REF}" \
    && git -C /opt/ATOM checkout --detach FETCH_HEAD \
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
COPY sshd-init.sh /usr/local/bin/sshd-init.sh
COPY sh-wrapper.sh /usr/local/bin/sh-wrapper.sh
# Replace the /usr/bin/sh symlink with a file. Writing through the symlink
# would overwrite dash.
RUN chmod 0755 /usr/local/bin/atom-entrypoint /usr/local/bin/sshd-init.sh /usr/local/bin/sh-wrapper.sh \
    && rm -f /usr/bin/sh \
    && cp /usr/local/bin/sh-wrapper.sh /usr/bin/sh \
    && mkdir -p /root/.ssh /run/sshd \
    && chmod 700 /root/.ssh \
    && rm -rf /run/sshd-init.lock /run/sshd-init.done \
    && rm -rf /tmp/aiter_configs \
    && mkdir -p /tmp/aiter_configs \
    && chmod 1777 /tmp/aiter_configs

WORKDIR /opt/ATOM
ENTRYPOINT ["/usr/local/bin/atom-entrypoint"]
CMD ["tail", "-f", "/dev/null"]
