FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y \
    clang-15 llvm-15 libbpf-dev linux-headers-generic \
    && ln -s /usr/bin/clang-15 /usr/bin/clang \
    && ln -s /usr/bin/llc-15 /usr/bin/llc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy BPF source (vmlinux.h is bundled in the repo)
COPY Lens/modules/exporters/rccl-socket-tracer/bpf/ ./bpf/
RUN clang -O2 -g -Wall -target bpf \
    -D__TARGET_ARCH_x86 \
    -I./bpf \
    -c ./bpf/tracer.bpf.c \
    -o ./bpf/tracer.bpf.o

# Copy Go source
COPY Lens/modules/exporters/rccl-socket-tracer/cmd/ ./cmd/
WORKDIR /build/cmd

# Generate Go bindings from compiled BPF object
RUN go install github.com/cilium/ebpf/cmd/bpf2go@v0.17.3
RUN go generate ./...

# Build Go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /rccl-socket-tracer .

# Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /rccl-socket-tracer /usr/local/bin/rccl-socket-tracer
ENTRYPOINT ["/usr/local/bin/rccl-socket-tracer"]
