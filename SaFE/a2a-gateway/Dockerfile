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
RUN GOOS=linux GOARCH=amd64 go build -o dist/a2a-gateway ./a2a-gateway/cmd/main.go

FROM ${REGISTRY}/library/alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /workspace/primus-safe/dist/a2a-gateway /usr/local/bin/a2a-gateway

EXPOSE 8089 9090
ENTRYPOINT ["a2a-gateway"]
