FROM primussafe/pytorch:rocm6.4.3_ubuntu22.04_py3.10
ENV DEBIAN_FRONTEND=noninteractive

ARG APP_HOME=/opt/primus-bench/preflight
RUN mkdir -p ${APP_HOME}/network
RUN mkdir -p ${APP_HOME}/ssh
RUN mkdir -p ${APP_HOME}/install

COPY ./preflight/network/*.py ${APP_HOME}/network
COPY ./preflight/network/*.sh ${APP_HOME}/network
COPY ./preflight/ssh/* ${APP_HOME}/ssh
COPY ./preflight/network/install/* ${APP_HOME}/install

RUN apt update && \
    apt install -y --no-install-recommends \
        net-tools bc \
        openssh-server \
        && \
    apt clean && rm -rf /var/lib/apt/lists/*

RUN bash ${APP_HOME}/install/install_network.sh && rm -rf ${APP_HOME}/install

WORKDIR ${APP_HOME}/network
