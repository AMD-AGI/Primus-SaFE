ARG REGISTRY=docker.io
FROM ${REGISTRY}/library/ubuntu:22.04

ARG ROCM_VERSION=6.4.3
ENV DEBIAN_FRONTEND=noninteractive
ENV ROCM_VERSION=${ROCM_VERSION}

ARG TMP_HOME=/opt/primus-bench/pytorch
RUN mkdir -p ${TMP_HOME}
COPY ./pytorch/*.sh ${TMP_HOME}/

RUN apt update && \
    apt install -y --no-install-recommends \
        python3 python3-pip python3-distutils python3-setuptools python3-wheel \
        build-essential automake autoconf libtool dpkg-dev \
        apt-utils sed wget git jq \
        libssl-dev libelf-dev \
        librdmacm-dev rdmacm-utils infiniband-diags ibverbs-utils ethtool libibverbs-dev rdma-core libibumad-dev libpci-dev strace \
        && \
    apt clean && rm -rf /var/lib/apt/lists/*

RUN python3 -m pip install --upgrade pip

RUN bash ${TMP_HOME}/install.sh && rm -rf /opt/primus-bench

RUN python3 --version && pip3 --version

WORKDIR /opt
