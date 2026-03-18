ARG ROCM_VERSION=6.4.3
ARG GPU_ARCHS=gfx942
ARG OS_VERSION=22.04
ARG OS_NAME=ubuntu
ARG PY_VERSION=3.10
FROM primussafe/pytorch:rocm${ROCM_VERSION}_${GPU_ARCHS}_${OS_NAME}${OS_VERSION}_py${PY_VERSION}

LABEL maintainer="PrimusBench"

ARG ROCM_VERSION=6.4.3
ARG GPU_ARCHS=gfx942
ARG OS_VERSION=22.04
ARG OS_NAME=ubuntu
ARG PY_VERSION=3.10
ARG AINIC_BUNDLE_FILENAME=""
ENV DEBIAN_FRONTEND=noninteractive
ENV ROCM_VERSION=${ROCM_VERSION}
ENV GPU_ARCHS=${GPU_ARCHS}
ENV OS_VERSION=${OS_VERSION}
ENV OS_NAME=${OS_NAME}

RUN LIBTINFO_PKG="libtinfo5" && \
    if [ "${OS_VERSION}" = "24.04" ]; then \
        LIBTINFO_PKG="libtinfo6"; \
    fi && \
    apt-get update && \
    apt-get -q install -y --no-install-recommends  \
    curl bc \
    dmidecode \
    hipify-clang \
    iproute2 \
    libaio-dev \
    libboost-all-dev \
    libcap2 \
    libcurl4-openssl-dev \
    ${LIBTINFO_PKG} \
    lshw \
    net-tools \
    openssh-client openssh-server \
    pciutils \
    python3-mpi4py \
    libnuma-dev libfmt-dev \
    rsync \
    sudo \
    util-linux kmod xz-utils numactl \
    vim \
    dnsutils \
    rocm-cmake \
    dkms \
    perftest libgtest-dev \
    python3-yaml \
    libyaml-cpp-dev \
    initramfs-tools \
    && \
    rm -rf /tmp/*

ARG WORKSPACE=/opt/primusbench

WORKDIR ${WORKSPACE}
#install ior
RUN cd ${WORKSPACE} && git clone https://github.com/hpc/ior.git && cd ior \
    && git checkout remotes/origin/4.0 \
    && ./bootstrap && ./configure --prefix="/opt" && make && make install

#install fio from source
RUN cd ${WORKSPACE} && git clone --depth=1 https://github.com/axboe/fio.git && cd fio \
    && ./configure --prefix="/opt" && make && make install


RUN if [ "${OS_VERSION}" = "24.04" ]; then \
        pip3 install --upgrade pip --break-system-packages --ignore-installed && \
        pip3 install --upgrade wheel setuptools==65.7 ansible --break-system-packages --ignore-installed; \
    else \
        python3 -m pip install --upgrade pip wheel setuptools==65.7 ansible; \
    fi

ARG PREFLIGHT_HOME=${WORKSPACE}/preflight
RUN mkdir -p ${PREFLIGHT_HOME}
RUN mkdir -p ${PREFLIGHT_HOME}/{ssh,network,node,install}
RUN mkdir -p ${PREFLIGHT_HOME}/node/config_check
RUN mkdir -p ${PREFLIGHT_HOME}/node/node_check
RUN mkdir -p ${PREFLIGHT_HOME}/node/model_check


COPY ./preflight/ssh/* ${PREFLIGHT_HOME}/ssh/
COPY ./preflight/network/*.py ${PREFLIGHT_HOME}/network/
COPY ./preflight/network/*.sh ${PREFLIGHT_HOME}/network/
COPY ./preflight/node/*.sh ${PREFLIGHT_HOME}/node/
COPY ./preflight/node/config_check/* ${PREFLIGHT_HOME}/node/config_check/
COPY ./preflight/node/node_check/* ${PREFLIGHT_HOME}/node/node_check/
COPY ./preflight/node/model_check/* ${PREFLIGHT_HOME}/node/model_check/
COPY ./preflight/install/* ${PREFLIGHT_HOME}/install/

# Set AINIC_BUNDLE_PATH if filename was provided (file already copied via COPY above)
RUN if [ -n "${AINIC_BUNDLE_FILENAME}" ]; then \
      export AINIC_BUNDLE_PATH="${PREFLIGHT_HOME}/install/${AINIC_BUNDLE_FILENAME}"; \
      echo "AINIC bundle path: ${AINIC_BUNDLE_PATH}"; \
    fi && \
    bash ${PREFLIGHT_HOME}/install/install.sh && rm -rf ${PREFLIGHT_HOME}/install

ENV ROCM_PATH=/opt/rocm

ENV PATH="/opt/superbench/bin:/usr/local/bin/:/opt/rocm/bin/:${PATH}" \
    ANSIBLE_DEPRECATION_WARNINGS=FALSE \
    ANSIBLE_COLLECTIONS_PATH=/usr/share/ansible/collections

ENV USE_HIP_DATATYPE=1
ENV USE_HIPBLAS_COMPUTETYPE=1

WORKDIR ${WORKSPACE}
COPY playbooks ./playbooks
COPY run.sh .
# COPY default.yaml .
# COPY rules.yaml .
COPY benchmarks ./benchmarks
RUN cd benchmarks && bash build.sh

RUN chmod +x run.sh
CMD ["/opt/primusbench/run.sh"]