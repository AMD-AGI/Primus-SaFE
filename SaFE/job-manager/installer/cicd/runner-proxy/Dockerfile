ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/cicd/runner-proxy
RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME} && chmod -R  750 ${APP_HOME}

COPY ./SaFE/job-manager/installer/cicd/runner-proxy/*.sh ${APP_HOME}/
