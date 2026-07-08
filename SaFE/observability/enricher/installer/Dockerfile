ARG REGISTRY=docker.io
ARG GO_VERSION=1.25.0
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

# Build inside the multi-module workspace so go.work resolves the in-repo
# apis module.
COPY ./SaFE /workspace/primus-safe
WORKDIR /workspace/primus-safe
RUN mkdir -p dist
RUN GOOS=linux GOARCH=amd64 go build -o dist/observability-enricher ./observability/enricher/cmd/main.go

FROM ${REGISTRY}/library/ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
ARG APP_HOME=/opt/primus-safe/observability-enricher
COPY --from=builder /workspace/primus-safe/dist/observability-enricher ${APP_HOME}/observability-enricher

RUN echo 'umask 0027' | tee -a /etc/bashrc

EXPOSE 8080
WORKDIR ${APP_HOME}
CMD ["./observability-enricher"]
