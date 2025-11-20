ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/cicd/runner-proxy
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME} && chmod -R  750 ${APP_HOME}

RUN apt update && \
    apt install -y --no-install-recommends \
        python3 python3-pip python3-distutils python3-setuptools python3-wheel && \
    apt clean && rm -rf /var/lib/apt/lists/*

COPY ./SaFE/job-manager/installer/cicd/runner-proxy/*.sh ${APP_HOME}/
COPY ./SaFE/job-manager/installer/cicd/runner-proxy/*.py ${APP_HOME}/
