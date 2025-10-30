FROM primussafe/pytorch:rocm6.4.3_ubuntu22.04_py3.10
ENV DEBIAN_FRONTEND=noninteractive

ARG APP_HOME=/opt/primus-bench/preflight/node
RUN mkdir -p ${APP_HOME}
RUN mkdir -p ${APP_HOME}/install
RUN mkdir -p ${APP_HOME}/config_check
RUN mkdir -p ${APP_HOME}/node_check

COPY ./preflight/node/*.sh ${APP_HOME}
COPY ./preflight/node/install/* ${APP_HOME}/install
COPY ./preflight/node/config_check/* ${APP_HOME}/config_check
COPY ./preflight/node/node_check/* ${APP_HOME}/node_check

RUN apt update && \
    apt install -y --no-install-recommends \
        perftest libgtest-dev bc \
        openmpi-bin openmpi-common libopenmpi-dev \
        rocm-validation-suite gfortran libgfortran-12-dev python3-yaml \
        && \
    apt clean && rm -rf /var/lib/apt/lists/*

RUN bash ${APP_HOME}/install/install_node.sh && rm -rf ${APP_HOME}/install

WORKDIR ${APP_HOME}
