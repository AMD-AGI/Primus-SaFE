#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# 4-stage multi-target Dockerfile for PrimusBench
#
# Targets (use --target):
#   base     - ROCm + system libs + RCCL (foundation layer)
#   bench    - + lightweight bench tools (rccl-tests, rdma, transfer_bench, ...)
#              no torch, so preflight model_check is NOT available in this target
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
RUN DISTUTILS_PKG="" && \
    if [ "${OS_VERSION}" != "24.04" ]; then DISTUTILS_PKG="python3-distutils"; fi && \
    LIBTINFO_PKG="libtinfo5" && \
    if [ "${OS_VERSION}" = "24.04" ]; then LIBTINFO_PKG="libtinfo6"; fi && \
    for i in 1 2 3; do \
        apt-get update && apt-get install -y --no-install-recommends \
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

# Ubuntu 24.04 marks the system Python as externally managed (PEP 668), so every
# bare `pip3 install` fails with "externally-managed-environment". This image IS
# the environment -- there is no venv to protect -- and pip calls are scattered
# across preflight/*.sh, pytorch/*.sh, Primus' own requirements install and the
# runtime model_benchmark. Setting it once here beats tacking
# --break-system-packages onto each call site and missing some.
RUN if [ "${OS_VERSION}" = "24.04" ]; then \
        printf '[global]\nbreak-system-packages = true\n' > /etc/pip.conf && \
        cat /etc/pip.conf; \
    fi

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
#
# Deliberately torch-free. preflight model_check (and its HF pre-download) is
# installed in the pytorch stage instead -- see the SKIP_INSTALL_SCRIPTS note
# below for why running it here would poison the image with CUDA wheels.
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

# Cache locations must be set before install.sh so install_prepare_dataset.sh
# (run later, in the pytorch stage) and any runtime download land in the same place.
ENV PRIMUSBENCH_CACHE=/opt/primusbench_cache \
    HF_HOME=/opt/primusbench_cache/huggingface \
    TRANSFORMERS_CACHE=/opt/primusbench_cache/huggingface/hub \
    DATASET_CACHE_DIR=/opt/primusbench_cache/datasets \
    HF_HUB_DOWNLOAD_TIMEOUT=300 \
    HF_HUB_ETAG_TIMEOUT=60

# install_model_check.sh installs preflight/node/model_check/requirements.txt,
# which pins torch>=2.6.0. This stage has no torch and no ROCm wheel index, so
# running it here would pull the CUDA build from PyPI (multi-GB of nvidia-*
# wheels baked into an immutable layer, then shadowed by ROCm torch in stage 3).
# install_prepare_dataset.sh imports those same packages. Both are deferred to
# the pytorch stage; the bench target intentionally has no model_check support.
RUN if [ -n "${AINIC_BUNDLE_FILENAME}" ]; then \
      export AINIC_BUNDLE_PATH="${PREFLIGHT_HOME}/install/${AINIC_BUNDLE_FILENAME}"; \
      echo "AINIC bundle path: ${AINIC_BUNDLE_PATH}"; \
    fi && \
    SKIP_INSTALL_SCRIPTS="install_model_check.sh install_prepare_dataset.sh" \
      bash ${PREFLIGHT_HOME}/install/install.sh && \
    find ${PREFLIGHT_HOME}/install -maxdepth 1 -type f \
        \( -name '*.tar.gz' -o -name '*.tgz' -o -name '*.deb' -o -name '*.rpm' \) -delete

COPY preflight/ssh/* ${PREFLIGHT_HOME}/ssh/
COPY preflight/network/*.py ${PREFLIGHT_HOME}/network/
COPY preflight/network/*.sh ${PREFLIGHT_HOME}/network/
COPY preflight/node/*.sh ${PREFLIGHT_HOME}/node/
COPY preflight/node/config_check/* ${PREFLIGHT_HOME}/node/config_check/
COPY preflight/node/node_check/* ${PREFLIGHT_HOME}/node/node_check/
# NOTE: no wildcard here. `COPY dir/* dest/` copies the *contents* of every
# matched subdirectory into dest, flattening the tree -- model_check has
# config/ data/ engine/ models/ utils/, whose four __init__.py would collide
# on one file and break `from data.build_dataset import ...` at runtime.
COPY preflight/node/model_check/ ${PREFLIGHT_HOME}/node/model_check/

COPY benchmarks ./benchmarks
RUN cd benchmarks && bash build.sh

COPY playbooks ./playbooks
COPY run.sh config.sh ./

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
    elif [ "${ROCM_VERSION}" = "7.0.1" ]; then \
        pip3 install torch torchvision --index-url https://download.pytorch.org/whl/rocm7.0 $PIP_EXTRA_ARGS; \
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

# Deferred from the bench stage: model_check needs torch, and its
# requirements.txt carries a bare `torch>=2.6.0` that would happily resolve to
# the CUDA wheel on PyPI. Pin torch to whatever ROCm build was just installed
# via PIP_CONSTRAINT so nothing downstream can swap it out.
ARG WORKSPACE=/opt/primusbench
ARG PREFLIGHT_HOME=${WORKSPACE}/preflight
RUN python3 -c "import torch; print('torch==' + torch.__version__)" > /tmp/torch.constraint && \
    cat /tmp/torch.constraint && \
    PIP_CONSTRAINT=/tmp/torch.constraint bash ${PREFLIGHT_HOME}/install/install_model_check.sh && \
    PIP_CONSTRAINT=/tmp/torch.constraint bash ${PREFLIGHT_HOME}/install/install_prepare_dataset.sh && \
    rm -f /tmp/torch.constraint && \
    rm -rf ${PREFLIGHT_HOME}/install

# Fail the build here rather than shipping an image whose GPUs are invisible.
RUN python3 -c "import torch, transformers, datasets; \
assert torch.version.hip, 'torch is not a ROCm build: ' + torch.__version__; \
print('ROCm torch preserved:', torch.__version__)"

# ============================================================================
# Stage 4: full - Primus training capabilities
#
# Complete image with Transformer Engine, Primus-Turbo, and all Python
# dependencies needed for distributed model training via Primus.
# ============================================================================
FROM pytorch AS full

ARG PRIMUS_TURBO_COMMIT=79373eb781a54fd49aed9430c8718489409d1dd0
ARG TE_BRANCH=v2.8_rocm

RUN ln -sf /usr/bin/python3 /usr/bin/python

# --- Transformer Engine (best-effort: CK headers may not compile on all ROCm versions) ---
RUN cd /opt && \
    git clone --branch ${TE_BRANCH} --depth 1 \
        https://github.com/ROCm/TransformerEngine.git && \
    cd TransformerEngine && \
    git submodule update --init --recursive && \
    sed -i 's/ -Werror//' transformer_engine/common/ck_fused_attn/CMakeLists.txt && \
    PYTORCH_ROCM_ARCH="${GPU_ARCHS}" \
    CMAKE_HIP_ARCHITECTURES="${GPU_ARCHS}" \
    NVTE_FRAMEWORK=pytorch NVTE_BUILD_MAX_JOBS=16 \
        pip3 install --no-build-isolation . \
    && echo "Transformer Engine installed successfully" \
    || echo "WARNING: Transformer Engine build failed (non-fatal); Primus will use fallback kernels"; \
    cd /opt && rm -rf TransformerEngine

RUN python3 -c "import transformer_engine; print(f'TE {transformer_engine.__version__}')" \
    || echo "WARNING: Transformer Engine not available"

# --- Primus-Turbo (best-effort: requires matching PyTorch + CK versions) ---
RUN set +e && \
    cd /opt && \
    git clone --recursive https://github.com/AMD-AGI/Primus-Turbo.git && \
    cd Primus-Turbo && \
    git checkout ${PRIMUS_TURBO_COMMIT} && \
    git submodule update --init --recursive && \
    pip3 install -r requirements.txt && \
    GPU_ARCHS="${GPU_ARCHS}" pip3 install --no-build-isolation .; \
    _rc=$?; cd /opt; rm -rf Primus-Turbo; \
    if [ $_rc -eq 0 ]; then echo "Primus-Turbo installed successfully"; \
    else echo "WARNING: Primus-Turbo build failed (non-fatal)"; fi; \
    exit 0

RUN python3 -c "import primus_turbo; print('Primus-Turbo OK')" \
    || echo "WARNING: Primus-Turbo not available"

# --- Pre-install Primus + Megatron-LM (pinned version for model benchmark) ---
ARG PRIMUS_BRANCH=v0.7.0
ENV PRIMUS_PREINSTALL_DIR=/opt/primus

RUN git clone --depth=1 --branch ${PRIMUS_BRANCH} \
        https://github.com/AMD-AGI/Primus.git ${PRIMUS_PREINSTALL_DIR} && \
    cd ${PRIMUS_PREINSTALL_DIR} && \
    git submodule update --init --depth=1 third_party/Megatron-LM && \
    echo "Primus ${PRIMUS_BRANCH} pre-installed at ${PRIMUS_PREINSTALL_DIR}"

# Primus Python dependencies (from Primus requirements.txt + extras).
# Same hazard as the pytorch stage: Primus' requirements.txt and the torch*
# extras below can resolve torch off PyPI (CUDA build) and silently replace the
# ROCm one. Constrain torch to the installed version for the whole install.
RUN python3 -c "import torch; print('torch==' + torch.__version__)" > /tmp/torch.constraint && \
    export PIP_CONSTRAINT=/tmp/torch.constraint && \
    pip3 install pybind11 && \
    pip3 install -r ${PRIMUS_PREINSTALL_DIR}/requirements.txt && \
    pip3 install \
        loguru wandb expecttest nltk matplotlib markdown2 weasyprint \
        tyro torchao blobfile "torchdata>=0.8.0" "datasets>=3.6.0" \
        "mlflow==3.4.0" pyrsmi plotext && \
    rm -f /tmp/torch.constraint

# Build Megatron C++ dataset helpers
RUN cd ${PRIMUS_PREINSTALL_DIR}/third_party/Megatron-LM/megatron/core/datasets && \
    make -j4 && \
    echo "Megatron helpers_cpp built successfully"

# Nothing upstream sets PYTHONPATH (base is stock ubuntu), so do not append to
# it -- referencing it only earns an UndefinedVar warning and a trailing colon.
ENV PYTHONPATH="${PRIMUS_PREINSTALL_DIR}:${PRIMUS_PREINSTALL_DIR}/third_party/Megatron-LM"

# Last line of defence: nothing above may have swapped in a CUDA torch.
RUN python3 -c "import torch; \
assert torch.version.hip, 'torch is not a ROCm build: ' + torch.__version__; \
print('final torch:', torch.__version__)"

WORKDIR /opt/primusbench
