ARG REGISTRY=docker.io
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG GOPROXY='https://goproxy.cn,direct'

FROM ${REGISTRY}/library/ubuntu:22.04

ENV http_proxy=${HTTP_PROXY}
ENV https_proxy=${HTTPS_PROXY}
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV GO_VERSION=1.24.2
ENV PATH=$PATH:/usr/local/go/bin

RUN apt-get update && apt-get install -y wget \
    && wget https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz \
    && rm go${GO_VERSION}.linux-amd64.tar.gz
RUN go version

RUN apt-get install -y sed

ENV LANG="en_US.UTF-8"
ENV GOOS="linux"
ENV GO111MODULE="on"
ENV GOPROXY=$GOPROXY
ENV GOROOT="/usr/local/go"
ENV GOBIN="/usr/local/go/bin"
ENV CGO_ENABLED=0
ENV GOPATH="/workspace/primus-safe/go"

COPY . /workspace/primus-safe
WORKDIR /workspace/primus-safe

ENV BASE_HOME=/opt/primus-safe
ENV APP_HOME=/opt/primus-safe/webhooks
RUN mkdir -p ${BASE_HOME} && chmod 750 ${BASE_HOME}
RUN mkdir ${APP_HOME} && chmod 750 ${APP_HOME}
RUN mkdir ${APP_HOME}/logs && chmod 750 ${APP_HOME}/logs
RUN mkdir ${APP_HOME}/config && chmod 750 ${APP_HOME}/config
RUN cd ./webhooks/cmd && go build -o ${APP_HOME}/webhooks main.go \
  && go clean --modcache --cache && cd ../..

COPY ./webhooks/installer/dockerfile/run.sh ${APP_HOME}/
COPY ./webhooks/config/config.yaml  ${APP_HOME}/config/config.yaml

RUN  ls  -l ${APP_HOME} && \
  echo 'umask 0027' | tee -a /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/profile

RUN rm -rf /workspace/primus-safe

EXPOSE 8084
WORKDIR ${APP_HOME}
CMD ["/bin/sh", "run.sh"]
