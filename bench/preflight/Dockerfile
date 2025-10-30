FROM primussafe/pytorch:rocm6.4.3_ubuntu22.04_py3.10
ENV DEBIAN_FRONTEND=noninteractive

ARG APP_HOME=/opt/primus-bench/preflight
RUN mkdir -p ${APP_HOME}
RUN mkdir -p ${APP_HOME}/ssh
RUN mkdir -p ${APP_HOME}/network
RUN mkdir -p ${APP_HOME}/node
RUN mkdir -p ${APP_HOME}/node/config_check
RUN mkdir -p ${APP_HOME}/node/node_check
RUN mkdir -p ${APP_HOME}/install

COPY ./preflight/ssh/* ${APP_HOME}/ssh
COPY ./preflight/network/*.py ${APP_HOME}/network
COPY ./preflight/network/*.sh ${APP_HOME}/network
COPY ./preflight/node/*.sh ${APP_HOME}/node
COPY ./preflight/node/config_check/* ${APP_HOME}/node/config_check
COPY ./preflight/node/node_check/* ${APP_HOME}/node/node_check
COPY ./preflight/node/install/* ${APP_HOME}/install
COPY ./preflight/network/install/* ${APP_HOME}/install

RUN apt update && \
    apt install -y --no-install-recommends \
        net-tools bc \
        perftest libgtest-dev bc \
        openssh-server \
        openmpi-bin openmpi-common libopenmpi-dev \
        rocm-validation-suite gfortran libgfortran-12-dev python3-yaml \
        && \
    apt clean && rm -rf /var/lib/apt/lists/*

RUN bash ${APP_HOME}/install/install_network.sh && bash ${APP_HOME}/install/install_node.sh && rm -rf ${APP_HOME}/install

WORKDIR ${APP_HOME}
