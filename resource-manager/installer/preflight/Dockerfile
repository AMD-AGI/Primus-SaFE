ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/opt/primus-safe/preflight
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/mi300x && chmod -R  750 ${APP_HOME}

COPY ./resource-manager/installer/preflight/*.sh ${APP_HOME}/
COPY ./resource-manager/installer/preflight/mi300x/*.sh ${APP_HOME}/mi300x/

WORKDIR ${APP_HOME}
