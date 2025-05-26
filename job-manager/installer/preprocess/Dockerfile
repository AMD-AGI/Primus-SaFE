ARG REGISTRY=docker.io
ARG HTTP_PROXY
ARG HTTPS_PROXY

FROM ${REGISTRY}/library/ubuntu:22.04

ENV APP_HOME=/preprocess
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/scripts && chmod -R  750 ${APP_HOME}

# 拷贝文件
COPY ./build/job-manager/preprocess/*.sh ${APP_HOME}/
COPY ./build/job-manager/preprocess/scripts/*.sh ${APP_HOME}/scripts/

WORKDIR ${APP_HOME}
