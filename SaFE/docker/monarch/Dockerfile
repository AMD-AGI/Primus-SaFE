# Build from latest pytorch stable image; should be relatively in sync with torchmonarch and pytorch.
FROM tasimage/primus:pr-563

# usage:
#    docker buildx build \
#    --build-arg GPU_ARCH=gfx950 \
#    --build-context ainic=/shared_nfs/weilei/ainic \
#    --build-context torchtitan=/shared_nfs/weilei/torchtitan \
#    --build-context torchft=/shared_nfs/weilei/torchft \
#    -t primussafe/monarch:202604071400 .
SHELL ["/bin/bash", "-c"]

# Ensure /opt/venv/bin is on PATH for ALL shells (bash, sh, subprocess).
# Without this, bash sources .bashrc and sees the venv, but sh and
# Monarch's Rust-spawned subprocesses (/usr/bin/python3.10) do not.
ENV PATH="/opt/venv/bin:${PATH}"
ENV VIRTUAL_ENV=/opt/venv

# Path
ENV LD_LIBRARY_PATH=/usr/local/lib/python3.10/dist-packages/torch/lib:$LD_LIBRARY_PATH
ENV PYTHONPATH=/workspace/monarch:/workspace/torchft:/workspace/torchtitan:/workspace/monarch/python:/opt/venv/lib/python3.10/site-packages
ENV XDG_RUNTIME_DIR=/tmp/root

# Ensure 'python' command is available.
RUN ln -sf "$(which python3)" /usr/local/bin/python

# System dependencies.
RUN apt-get update -y && \
    apt-get install -y curl clang cmake liblzma-dev libunwind-dev \
    libibverbs-dev librdmacm-dev libmlx5-1 ibverbs-providers protobuf-compiler vim && \
    rm -f /opt/venv/bin/cmake && \
    ln -sf /usr/lib/x86_64-linux-gnu/libefa.so.1 /usr/lib/x86_64-linux-gnu/libefa.so
RUN pip install --upgrade pip setuptools wheel setuptools-rust --break-system-packages


# Install AMD AINIC driver (set AINIC_DRIVER_VERSION to skip/change version)
# GPU_ARCH: target GPU architecture (e.g., gfx942, gfx950) - required for Docker builds without GPU access
# Use --build-context to override source dirs:
#   docker buildx build --build-context ainic=/path/to/ainic --build-context torchtitan=/path/to/torchtitan --build-context torchft=/path/to/torchft .
COPY --from=ainic . /workspace/ainic
#ARG AINIC_DRIVER_VERSION=1.117.5-a-56
ARG GPU_ARCH=gfx942
RUN AINIC_DRIVER_VERSION=${AINIC_DRIVER_VERSION} GPU_ARCH=${GPU_ARCH} /bin/sh /workspace/ainic/build_ainic.sh && \
    rm -rf /workspace/ainic

# PyTorch is already included in the base image (2.10.0+rocm7.1).
# Install Rust toolchain.
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y && \
    . "$HOME/.cargo/env" && \
    rustup toolchain install nightly-2025-12-05 && \
    rustup default nightly-2025-12-05
ENV PATH="/root/.cargo/bin:${PATH}"

# Uninstall existing torchtitan and install from local source
RUN pip uninstall -y torchtitan || true && \
    rm -rf /workspace/torchtitan /opt/venv/lib/python*/site-packages/torchtitan* && \
    pip cache purge

COPY --from=torchtitan . /workspace/torchtitan
RUN cd /workspace/torchtitan && \
    python -m pip install -r requirements.txt --break-system-packages && \
    python -m pip install -e . --break-system-packages --force-reinstall --no-deps

# Install torchft from source (with hostname fix for Kubernetes)
COPY --from=torchft . /workspace/torchft
RUN rm -f /workspace/torchft/torchft/_torchft*.so && \
    cd /workspace/torchft && \
    pip install -e . --break-system-packages

# Copy monarch source code.
# COPY . /workspace/monarch

# Install monarch from source w/ kubernetes.
# MONARCH_RDMA_GPU_PLATFORM=rocm: build nccl-sys/rdmaxcel-sys with HIP/RCCL instead of CUDA/NCCL.
# MONARCH_RDMA_USE_SYSTEM=1: use system-installed rdma-core instead of building from source.

# Pre-install dependencies to avoid resolution issues
RUN python3 -m pip install pyzmq opentelemetry-api opentelemetry-sdk cloudpickle --break-system-packages

RUN ulimit -n 65536
RUN cd /workspace && git clone https://github.com/zstreet87/monarch.git && \ 
    cd monarch && git checkout hipify && git submodule update --init --recursive && \
    MONARCH_RDMA_GPU_PLATFORM=rocm \
    MONARCH_RDMA_USE_SYSTEM=1 CARGO_BUILD_JOBS=8 pip install -e ".[kubernetes]" --no-build-isolation --break-system-packages

# Copy Monarch client, mesh startup, and utilities
COPY client.py /workspace/monarch/client.py
COPY start_mesh.sh /workspace/monarch/start_mesh.sh
COPY utils/ /workspace/monarch/utils/

# Apply Monarch patches at build time:
# 1. PyTorch inductor duplicate registration assertions (upstream fix: Sep 2025)
# 2. socket.gethostname -> POD_IP for Kubernetes hostNetwork mode
# Set POD_IP=build so the hostname patch hook is installed (activates at runtime when real POD_IP is set)
COPY patch.py /tmp/patch.py
RUN POD_IP=build python3 /tmp/patch.py && rm -f /tmp/patch.py

WORKDIR /workspace/monarch