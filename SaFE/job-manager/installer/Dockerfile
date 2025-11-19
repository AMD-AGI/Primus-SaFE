ARG REGISTRY=docker.io
ARG GO_VERSION=1.24.7
ARG GOPROXY
FROM ${REGISTRY}/library/golang:${GO_VERSION} AS builder
ARG GOPROXY
ENV LANG="en_US.UTF-8"
ENV GO111MODULE="on"

ENV GOPROXY=${GOPROXY}
ENV GOROOT="/usr/local/go"
ENV GOBIN="/usr/local/go/bin"
ENV CGO_ENABLED=0
ENV GOPATH="/workspace/primus-safe/go"

COPY ./SaFE /workspace/primus-safe
WORKDIR /workspace/primus-safe
RUN mkdir dist && mkdir dist/logs
RUN GOOS=linux GOARCH=amd64  go build -o dist/job_manager ./job-manager/cmd/main.go

FROM ${REGISTRY}/library/ubuntu:22.04
ARG APP_HOME=/opt/primus-safe/job-manager
RUN apt-get update
RUN apt-get install -y sed
COPY --from=builder /workspace/primus-safe/dist ${APP_HOME}
COPY ./SaFE/job-manager/installer/run.sh  ${APP_HOME}

RUN  ls  -l ${APP_HOME} && \
  echo 'umask 0027' | tee -a /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/bashrc && \
  echo "export HISTSIZE=100" >> /etc/profile

RUN rm -rf /workspace/primus-safe

EXPOSE 8083
WORKDIR ${APP_HOME}
CMD ["/bin/sh", "run.sh"]
