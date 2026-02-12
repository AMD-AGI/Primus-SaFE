ARG REGISTRY=docker.io
ARG OS_VERSION=22.04
ARG PY_VERSION=3.10
FROM ${REGISTRY}/library/ubuntu:${OS_VERSION}

ARG ROCM_VERSION=6.4.3
ARG GPU_ARCHS=gfx942
ARG OS_VERSION=22.04
ARG PY_VERSION=3.10
ENV DEBIAN_FRONTEND=noninteractive
ENV ROCM_VERSION=${ROCM_VERSION}
ENV GPU_ARCHS=${GPU_ARCHS}
ENV OS_VERSION=${OS_VERSION}
ENV PY_VERSION=${PY_VERSION}

ARG TMP_HOME=/opt/primus-bench/pytorch
RUN mkdir -p ${TMP_HOME}
COPY ./pytorch/*.sh ${TMP_HOME}/

RUN apt update && \
    DISTUTILS_PKG="" && \
    if [ "${OS_VERSION}" != "24.04" ]; then DISTUTILS_PKG="python3-distutils"; fi && \
    apt install -y --no-install-recommends \
        python3 python3-pip ${DISTUTILS_PKG} python3-setuptools python3-wheel python3-docutils \
        build-essential automake autoconf libtool dpkg-dev \
        apt-utils sed wget git jq ca-certificates pkg-config \
        libssl-dev libelf-dev libudev-dev libnl-3-dev libnl-route-3-dev libsystemd-dev  \
        librdmacm-dev rdma-core rdmacm-utils infiniband-diags ibverbs-utils ethtool libibverbs-dev ibverbs-utils libibumad-dev libpci-dev strace \
        && \
    apt clean && rm -rf /var/lib/apt/lists/*

RUN if [ "${OS_VERSION}" = "24.04" ]; then \
        pip3 install --upgrade pip --break-system-packages --ignore-installed; \
    else \
        python3 -m pip install --upgrade pip; \
    fi

RUN bash ${TMP_HOME}/install.sh && rm -rf /opt/primus-bench

RUN python3 --version && pip3 --version

WORKDIR /opt
