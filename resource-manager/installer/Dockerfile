ARG REGISTRY=docker.io
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG GOPROXY='https://goproxy.cn,direct'
ARG GO_VERSION=1.24.2

FROM --platform=linux/amd64 golang:${GO_VERSION} as builder

RUN echo ${HTTP_PROXY}
RUN echo ${HTTPS_PROXY}
ENV http_proxy=${HTTP_PROXY}
ENV https_proxy=${HTTPS_PROXY}
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}

ENV LANG="en_US.UTF-8"
ENV GOOS="linux"
ENV GOARCH="amd64"
ENV GO111MODULE="on"
ENV GOPROXY=$GOPROXY
ENV GOROOT="/usr/local/go"
ENV GOBIN="/usr/local/go/bin"
ENV CGO_ENABLED=0
ENV GOPATH="/workspace/safe/go"

COPY . /workspace/primus-safe
WORKDIR /workspace/primus-safe
RUN mkdir dist && mkdir dist/config && mkdir dist/logss
RUN GOOS=linux GOARCH=amd64  go build -o dist/resource_manager ./resource-manager/cmd/main.go

FROM --platform=linux/amd64 ${REGISTRY}/library/ubuntu:22.04

RUN apt-get install -y sed

ENV BASE_HOME=/opt/primus-safe
ENV APP_HOME=/opt/primus-safe/resource-manager

COPY --from=builder /workspace/primus-safe/dist ${APP_HOME}
COPY ./resource-manager/installer/dockerfile/run.sh  ${APP_HOME}
COPY ./resource-manager/config/config.yaml  ${APP_HOME}/config

RUN  ls  -l ${APP_HOME} && \
  echo 'umask 0027' | tee -a /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/profile

EXPOSE 8081
WORKDIR ${APP_HOME}
CMD ["/bin/sh", "run.sh"]
