ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive

RUN apt update && \
    apt install -y --no-install-recommends \
        python3 \
        python3-pip \
        python3-distutils  \
        python3-setuptools  \
        python3-wheel \
        openssh-server \
        net-tools \
        build-essential automake autoconf \
        libtool \
        git \
        sed \
        bc \
        libssl-dev \
        libelf-dev \
        librdmacm-dev rdmacm-utils infiniband-diags ibverbs-utils perftest ethtool libibverbs-dev rdma-core strace \
        wget \
        && \
    apt clean && \
    rm -rf /var/lib/apt/lists/*

RUN python3 --version && \
    pip3 --version

ARG APP_HOME=/opt/primus-safe/diagnoser
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/depends
COPY ./health-checker/rccl-diagnoser/*.py ${APP_HOME}
COPY ./health-checker/rccl-diagnoser/*.sh ${APP_HOME}
COPY ./health-checker/rccl-diagnoser/depends/* ${APP_HOME}/depends

WORKDIR ${APP_HOME}
