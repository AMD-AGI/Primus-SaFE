ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/preprocess
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/scripts && chmod -R  750 ${APP_HOME}

COPY ./SaFE/docker/preprocess/*.sh ${APP_HOME}/
COPY ./SaFE/docker/preprocess/scripts/*.sh ${APP_HOME}/scripts/