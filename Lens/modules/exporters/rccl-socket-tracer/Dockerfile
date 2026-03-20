FROM golang:1.22-bookworm AS builder

RUN apt-get update && apt-get install -y \
    clang-15 llvm-15 libbpf-dev linux-headers-generic \
    && ln -s /usr/bin/clang-15 /usr/bin/clang \
    && ln -s /usr/bin/llc-15 /usr/bin/llc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Generate vmlinux.h from libbpf-bootstrap (try multiple sources)
RUN curl -sfL "https://raw.githubusercontent.com/libbpf/libbpf-bootstrap/refs/heads/master/vmlinux/vmlinux_612.h" -o /build/vmlinux.h || \
    curl -sfL "https://cdn.jsdelivr.net/gh/libbpf/libbpf-bootstrap@master/vmlinux/vmlinux_612.h" -o /build/vmlinux.h || \
    (apt-get update && apt-get install -y linux-tools-common bpftool && \
     bpftool btf dump file /sys/kernel/btf/vmlinux format c > /build/vmlinux.h)
RUN head -1 /build/vmlinux.h && wc -l /build/vmlinux.h

# Copy BPF source and compile
COPY Lens/modules/exporters/rccl-socket-tracer/bpf/ ./bpf/
RUN cp /build/vmlinux.h ./bpf/vmlinux.h
RUN clang -O2 -g -Wall -target bpf \
    -D__TARGET_ARCH_x86 \
    -I./bpf \
    -c ./bpf/tracer.bpf.c \
    -o ./bpf/tracer.bpf.o

# Copy Go source
COPY Lens/modules/exporters/rccl-socket-tracer/cmd/ ./cmd/
WORKDIR /build/cmd

# Generate Go bindings from compiled BPF object
RUN go install github.com/cilium/ebpf/cmd/bpf2go@latest
RUN go generate ./...

# Build Go binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /rccl-socket-tracer .

# Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /rccl-socket-tracer /usr/local/bin/rccl-socket-tracer
ENTRYPOINT ["/usr/local/bin/rccl-socket-tracer"]
