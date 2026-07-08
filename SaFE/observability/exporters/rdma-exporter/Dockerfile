# Build trigger: amd64-only eBPF + quay.io registry
ARG GO_VERSION=1.24

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS builder

ARG TARGETOS
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
    clang llvm libbpf-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN ln -s /app/include /include
RUN go generate ./pkg/bpf/...
RUN rm -f pkg/bpf/rdmactl/bpf_stub.go pkg/bpf/rdmaflow/bpf_stub.go
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o rdma-exporter ./cmd

FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    iproute2 \
    rdma-core \
    infiniband-diags \
    util-linux \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/rdma-exporter /usr/local/bin/

EXPOSE 9401

ENTRYPOINT ["rdma-exporter"]
