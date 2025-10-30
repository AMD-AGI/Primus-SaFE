ARG REGISTRY=docker.io
ARG GO_VERSION=1.24.2
FROM golang:${GO_VERSION} AS builder

ENV LANG="en_US.UTF-8"
ENV GO111MODULE="on"
ENV GOPROXY="https://goproxy.cn,direct"
ENV GOROOT="/usr/local/go"
ENV GOBIN="/usr/local/go/bin"
ENV CGO_ENABLED=0
ENV GOPATH="/workspace/primus-safe/go"

WORKDIR /workspace/primus-safe
RUN git clone https://github.com/kubernetes/kubernetes.git && \
    cd kubernetes && \
    GOOS=linux GOARCH=amd64  go build -o /workspace/primus-safe/kubemark ./cmd/kubemark/hollow-node.go

FROM ${REGISTRY}/library/ubuntu:22.04
RUN apt-get update
RUN apt-get install -y sed
COPY --from=builder /workspace/primus-safe/kubemark /usr/local/bin/kubemark
ENTRYPOINT ["/usr/local/bin/kubemark"]