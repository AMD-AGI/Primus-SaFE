ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/opt/primus-safe/preflight
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/mi300x && chmod -R  750 ${APP_HOME}

RUN apt-get update
RUN apt-get install -y sed
RUN apt-get install -y jq
RUN apt-get install -y bc

COPY ./health-checker/preflight/*.sh ${APP_HOME}/
COPY ./health-checker/preflight/mi300x/*.sh ${APP_HOME}/mi300x/

WORKDIR ${APP_HOME}
