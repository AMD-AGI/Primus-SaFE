ARG REGISTRY=docker.io
FROM ${REGISTRY}/library/ubuntu:22.04

ARG ROCM_VERSION=6.4.3
ARG GPU_ARCHS=gfx942
ENV DEBIAN_FRONTEND=noninteractive
ENV ROCM_VERSION=${ROCM_VERSION}
ENV GPU_ARCHS=${GPU_ARCHS}

ARG TMP_HOME=/opt/primus-bench/pytorch
RUN mkdir -p ${TMP_HOME}
COPY ./pytorch/*.sh ${TMP_HOME}/

RUN apt update && \
    apt install -y --no-install-recommends \
        python3 python3-pip python3-distutils python3-setuptools python3-wheel python3-docutils \
        build-essential automake autoconf libtool dpkg-dev \
        apt-utils sed wget git jq ca-certificates pkg-config \
        libssl-dev libelf-dev libudev-dev libnl-3-dev libnl-route-3-dev libsystemd-dev  \
        librdmacm-dev rdma-core rdmacm-utils infiniband-diags ibverbs-utils ethtool libibverbs-dev libibumad-dev libpci-dev strace \
        && \
    apt clean && rm -rf /var/lib/apt/lists/*

RUN python3 -m pip install --upgrade pip

RUN bash ${TMP_HOME}/install.sh && rm -rf /opt/primus-bench

RUN python3 --version && pip3 --version

WORKDIR /opt
