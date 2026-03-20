FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y \
    clang-15 llvm-15 libbpf-dev linux-headers-generic \
    && ln -sf /usr/bin/clang-15 /usr/bin/clang \
    && ln -sf /usr/bin/llc-15 /usr/bin/llc \
    && ln -sf /usr/bin/llvm-strip-15 /usr/bin/llvm-strip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy all source (bpf/ has vmlinux.h bundled, cmd/ has Go source)
COPY Lens/modules/exporters/rccl-socket-tracer/bpf/ ./bpf/
COPY Lens/modules/exporters/rccl-socket-tracer/cmd/ ./cmd/
WORKDIR /build/cmd

# bpf2go compiles BPF and generates Go bindings in one step
RUN go mod tidy
RUN go generate ./...

# Build Go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /rccl-socket-tracer .

# Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /rccl-socket-tracer /usr/local/bin/rccl-socket-tracer
ENTRYPOINT ["/usr/local/bin/rccl-socket-tracer"]
