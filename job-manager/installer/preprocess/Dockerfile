ARG REGISTRY=docker.io

FROM --platform=linux/amd64 ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/preprocess
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/scripts && chmod -R  750 ${APP_HOME}

# 拷贝文件
COPY ./job-manager/installer/preprocess/*.sh ${APP_HOME}/
COPY ./job-manager/installer/preprocess/scripts/*.sh ${APP_HOME}/scripts/

WORKDIR ${APP_HOME}
