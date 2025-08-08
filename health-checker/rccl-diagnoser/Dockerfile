FROM primussafe/megatron-base:0.0.1
ENV DEBIAN_FRONTEND=noninteractive

RUN apt update && \
    apt install -y --no-install-recommends \
        python3 \
        python3-pip \
        python3-distutils \
        git \
        make \
        openmpi-bin \
        openmpi-common \
        libopenmpi-dev \
        && \
    apt clean && \
    rm -rf /var/lib/apt/lists/*

RUN python3 --version && \
    pip3 --version \

ARG APP_HOME=/opt/primus-safe/diagnoser
RUN mkdir -p ${APP_HOME} && chmod -R  750 ${APP_HOME}
COPY ./health-checker/rccl-diagnoser/*.py ${APP_HOME}/
COPY ./health-checker/rccl-diagnoser/*.sh ${APP_HOME}/
COPY ./health-checker/rccl-diagnoser/requirements.txt ${APP_HOME}/

WORKDIR ${APP_HOME}
