#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# 4-stage multi-target Dockerfile for PrimusBench
#
# Targets (use --target):
#   base     - ROCm + system libs + RCCL (foundation layer)
#   bench    - + lightweight bench tools (rccl-tests, rdma, transfer_bench, ...)
#   pytorch  - + PyTorch (model inference, micro_benchmarks)
#   full     - + Primus training stack (Transformer Engine, Primus-Turbo)
#
# Build context: Bench/ directory (run from Bench/ with -f build/Dockerfile)
#
# Examples:
#   docker buildx build . -f build/Dockerfile --target bench  ...
#   docker buildx build . -f build/Dockerfile --target full   ...
#

# ============================================================================
# Global ARGs (available to all stages via re-declaration)
# ============================================================================
ARG REGISTRY=docker.io
ARG OS_VERSION=22.04

# ============================================================================
# Stage 1: base - System + ROCm SDK + cmake + RCCL
# ============================================================================
FROM ${REGISTRY}/library/ubuntu:${OS_VERSION} AS base

ARG ROCM_VERSION=7.0.3
ARG GPU_ARCHS=gfx950
ARG OS_VERSION=22.04
ARG OS_NAME=ubuntu
ARG PY_VERSION=3.10

ENV DEBIAN_FRONTEND=noninteractive
ENV ROCM_VERSION=${ROCM_VERSION}
ENV GPU_ARCHS=${GPU_ARCHS}
ENV OS_VERSION=${OS_VERSION}
ENV OS_NAME=${OS_NAME}
ENV PY_VERSION=${PY_VERSION}

# Pre-ROCm packages: only stock Ubuntu packages that do NOT conflict with ROCm.
# rocm-cmake, dkms, hipify-clang must be installed AFTER ROCm repo is configured.
RUN apt-get update && \
    DISTUTILS_PKG="" && \
    if [ "${OS_VERSION}" != "24.04" ]; then DISTUTILS_PKG="python3-distutils"; fi && \
    LIBTINFO_PKG="libtinfo5" && \
    if [ "${OS_VERSION}" = "24.04" ]; then LIBTINFO_PKG="libtinfo6"; fi && \
    for i in 1 2 3; do \
        apt-get install -y --no-install-recommends \
            python3 python3-pip ${DISTUTILS_PKG} python3-setuptools python3-wheel python3-docutils \
            build-essential automake autoconf libtool dpkg-dev \
            apt-utils sed wget git jq ca-certificates pkg-config \
            libssl-dev libelf-dev libudev-dev libnl-3-dev libnl-route-3-dev libsystemd-dev \
            librdmacm-dev rdma-core rdmacm-utils infiniband-diags ibverbs-utils ethtool \
            libibverbs-dev libibumad-dev libpci-dev strace \
            curl bc dmidecode iproute2 libaio-dev libboost-all-dev libcap2 \
            libcurl4-openssl-dev ${LIBTINFO_PKG} \
            lshw net-tools openssh-client openssh-server pciutils \
            python3-mpi4py libnuma-dev libfmt-dev \
            rsync sudo util-linux kmod xz-utils numactl vim dnsutils \
            perftest libgtest-dev python3-yaml \
            libyaml-cpp-dev initramfs-tools \
        && break || { echo "Attempt $i failed, retrying in 15s..."; sleep 15; }; \
    done && \
    if [ "${OS_VERSION}" = "24.04" ]; then \
        pip3 install --upgrade pip --break-system-packages --ignore-installed; \
    else \
        python3 -m pip install --upgrade pip; \
    fi && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/*

ARG _BUILD=/opt/_build
RUN mkdir -p ${_BUILD}
COPY pytorch/install_cmake.sh pytorch/install_rocm.sh pytorch/install_rccl.sh ${_BUILD}/
RUN cd ${_BUILD} && bash install_cmake.sh
RUN cd ${_BUILD} && bash install_rocm.sh

# Post-ROCm packages: these come from the ROCm apt repo or depend on it
RUN apt-get update && apt-get install -y --no-install-recommends \
        rocm-cmake dkms hipify-clang \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN cd ${_BUILD} && bash install_rccl.sh
RUN rm -rf ${_BUILD}

ENV ROCM_PATH=/opt/rocm
ENV PATH="/opt/rocm/bin:${PATH}"

RUN python3 --version && pip3 --version && cmake --version

WORKDIR /opt

# ============================================================================
# Stage 2: bench - Lightweight bench tools (no PyTorch)
#
# Usable image for quick hardware checks: rccl-tests, rdma, transfer_bench,
# rocBLAS, babel_stream, ROCm validation suite, fio, ior, etc.
# ============================================================================
FROM base AS bench

LABEL maintainer="PrimusBench"

ARG AINIC_BUNDLE_FILENAME=""
ARG WORKSPACE=/opt/primusbench

WORKDIR ${WORKSPACE}

RUN if [ "${OS_VERSION}" = "24.04" ]; then \
        pip3 install --upgrade wheel setuptools==65.7 ansible --break-system-packages --ignore-installed; \
    else \
        python3 -m pip install --upgrade pip wheel setuptools==65.7 ansible; \
    fi

ARG PREFLIGHT_HOME=${WORKSPACE}/preflight
RUN mkdir -p ${PREFLIGHT_HOME}/{ssh,network,node,install} && \
    mkdir -p ${PREFLIGHT_HOME}/node/{config_check,node_check,model_check}

COPY preflight/install/* ${PREFLIGHT_HOME}/install/

RUN if [ -n "${AINIC_BUNDLE_FILENAME}" ]; then \
      export AINIC_BUNDLE_PATH="${PREFLIGHT_HOME}/install/${AINIC_BUNDLE_FILENAME}"; \
      echo "AINIC bundle path: ${AINIC_BUNDLE_PATH}"; \
    fi && \
    bash ${PREFLIGHT_HOME}/install/install.sh && rm -rf ${PREFLIGHT_HOME}/install

COPY preflight/ssh/* ${PREFLIGHT_HOME}/ssh/
COPY preflight/network/*.py ${PREFLIGHT_HOME}/network/
COPY preflight/network/*.sh ${PREFLIGHT_HOME}/network/
COPY preflight/node/*.sh ${PREFLIGHT_HOME}/node/
COPY preflight/node/config_check/* ${PREFLIGHT_HOME}/node/config_check/
COPY preflight/node/node_check/* ${PREFLIGHT_HOME}/node/node_check/
COPY preflight/node/model_check/* ${PREFLIGHT_HOME}/node/model_check/

COPY benchmarks ./benchmarks
RUN cd benchmarks && bash build.sh

COPY playbooks ./playbooks
COPY run.sh config.sh ./

ENV PRIMUSBENCH_CACHE=/opt/primusbench_cache \
    HF_HOME=/opt/primusbench_cache/huggingface \
    TRANSFORMERS_CACHE=/opt/primusbench_cache/huggingface/hub \
    DATASET_CACHE_DIR=/opt/primusbench_cache/datasets \
    HF_HUB_DOWNLOAD_TIMEOUT=300 \
    HF_HUB_ETAG_TIMEOUT=60

ENV PATH="/opt/superbench/bin:/opt/bin:/usr/local/bin:/opt/rocm/bin:${PATH}" \
    ANSIBLE_DEPRECATION_WARNINGS=FALSE \
    ANSIBLE_COLLECTIONS_PATH=/usr/share/ansible/collections

ENV USE_HIP_DATATYPE=1 \
    USE_HIPBLAS_COMPUTETYPE=1

RUN chmod +x run.sh
CMD ["/opt/primusbench/run.sh"]

# ============================================================================
# Stage 3: pytorch - Adds PyTorch
#
# Extends bench with PyTorch for model inference checks, dataset preparation,
# and any workload that needs torch but not full Primus training.
# ============================================================================
FROM bench AS pytorch

RUN PIP_EXTRA_ARGS="" && \
    if [ "${OS_VERSION}" = "24.04" ]; then PIP_EXTRA_ARGS="--break-system-packages"; fi && \
    if [ "${ROCM_VERSION}" = "6.4.3" ]; then \
        pip3 install torch torchvision --index-url https://download.pytorch.org/whl/rocm6.4 $PIP_EXTRA_ARGS; \
    elif [ "${ROCM_VERSION}" = "7.0.3" ]; then \
        pip3 install torch torchvision --pre --index-url https://download.pytorch.org/whl/nightly/rocm7.0 $PIP_EXTRA_ARGS; \
    elif [ "${ROCM_VERSION}" = "7.2.0" ]; then \
        RADEON_REPO="https://repo.radeon.com/rocm/manylinux/rocm-rel-7.2" && \
        pip3 install torch torchvision --no-deps --no-index --find-links ${RADEON_REPO} $PIP_EXTRA_ARGS && \
        pip3 install torch torchvision --find-links ${RADEON_REPO} $PIP_EXTRA_ARGS; \
    else \
        echo "Error: Unsupported ROCM_VERSION" && exit 1; \
    fi

RUN python3 -c "import torch; print(f'PyTorch {torch.__version__}, ROCm {torch.version.hip}')"

# ============================================================================
# Stage 4: full - Primus training capabilities
#
# Complete image with Transformer Engine, Primus-Turbo, and all Python
# dependencies needed for distributed model training via Primus.
# ============================================================================
FROM pytorch AS full

ARG PRIMUS_TURBO_COMMIT=79373eb781a54fd49aed9430c8718489409d1dd0
ARG TE_BRANCH=stable

RUN cd /opt && \
    git clone --branch ${TE_BRANCH} --depth 1 \
        https://github.com/ROCm/TransformerEngine.git && \
    cd TransformerEngine && \
    git submodule update --init --recursive && \
    NVTE_FRAMEWORK=pytorch pip3 install --no-build-isolation . && \
    cd /opt && rm -rf TransformerEngine

RUN python3 -c "import transformer_engine; print(f'TE {transformer_engine.__version__}')"

RUN cd /opt && \
    git clone --recursive https://github.com/AMD-AGI/Primus-Turbo.git && \
    cd Primus-Turbo && \
    git checkout ${PRIMUS_TURBO_COMMIT} && \
    git submodule update --init --recursive && \
    pip3 install -r requirements.txt && \
    GPU_ARCHS="${GPU_ARCHS}" pip3 install --no-build-isolation . && \
    cd /opt && rm -rf Primus-Turbo

RUN python3 -c "import primus_turbo; print('Primus-Turbo OK')" || true

RUN pip3 install \
    loguru wandb expecttest nltk matplotlib markdown2 weasyprint \
    tyro torchao blobfile "torchdata>=0.8.0" "datasets>=3.6.0" \
    "mlflow==3.4.0" pyrsmi plotext

WORKDIR /opt/primusbench
