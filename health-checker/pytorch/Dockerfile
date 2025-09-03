ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive

ARG TMP_HOME=/opt/primus-safe/pytorch
RUN mkdir -p ${TMP_HOME}
COPY ./health-checker/pytorch/*.sh ${TMP_HOME}
COPY ./health-checker/pytorch/*.txt ${TMP_HOME}
COPY ./health-checker/pytorch/*.gz ${TMP_HOME}

RUN apt update && \
    apt install -y --no-install-recommends \
        python3 python3-pip python3-distutils python3-setuptools python3-wheel \
        net-tools \
        build-essential automake autoconf libtool \
        git \
        apt-utils \
        sed \
        libssl-dev \
        libelf-dev \
        librdmacm-dev rdmacm-utils infiniband-diags ibverbs-utils perftest ethtool libibverbs-dev rdma-core libibumad-dev libpci-dev strace \
        wget \
        && \
    apt clean && rm -rf /var/lib/apt/lists/*

RUN bash ${TMP_HOME}/install.sh && rm -rf ${TMP_HOME}

RUN python3 --version && pip3 --version

WORKDIR /opt
