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
    && wget https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz --no-check-certificate \
    && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz \
    && rm go${GO_VERSION}.linux-amd64.tar.gz
RUN go version

RUN apt-get install -y sed
RUN apt-get install -y jq
RUN apt-get install -y bc

ENV LANG="en_US.UTF-8"
ENV GOOS="linux"
ENV GOPROXY=$GOPROXY
ENV GO111MODULE="on"
ENV GOROOT="/usr/local/go"
ENV GOBIN="/usr/local/go/bin"
ENV CGO_ENABLED=0
ENV GOPATH="/workspace/safe/go"

ENV BASE_HOME=/opt/safe
ENV APP_HOME=/opt/safe/node-agent
RUN mkdir -p ${BASE_HOME} && chmod 750 ${BASE_HOME}
RUN mkdir ${APP_HOME} && chmod 750 ${APP_HOME}
RUN mkdir ${APP_HOME}/logs && chmod 750 ${APP_HOME}/logs
RUN cd ./node-agent/cmd && go build -o ${APP_HOME}/node_agent main.go \
  && go clean --modcache --cache && cd ../..

COPY ./node-agent/run.sh ${APP_HOME}/
COPY ./node-agent/config/config.toml  ${APP_HOME}/config/config.toml

RUN  \
  echo 'umask 0027' | tee -a /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/profile

WORKDIR ${APP_HOME}
CMD ["/bin/sh", "run.sh"]
