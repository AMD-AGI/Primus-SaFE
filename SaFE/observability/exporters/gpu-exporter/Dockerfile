ARG GO_VERSION=1.24

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o gpu-exporter ./cmd

FROM ubuntu:22.04

ARG TARGETARCH

# Install base dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    util-linux \
    pciutils \
    kmod \
    && rm -rf /var/lib/apt/lists/*

# Install ROCm SMI tools for AMD GPU monitoring (amd64 only)
RUN if [ "${TARGETARCH}" = "amd64" ]; then \
        apt-get update && apt-get install -y --no-install-recommends \
            wget gnupg2 python3 python3-pip \
            libdrm2 libdrm-amdgpu1 \
        && mkdir -p /etc/apt/keyrings \
        && wget -qO - https://repo.radeon.com/rocm/rocm.gpg.key \
            | gpg --dearmor -o /etc/apt/keyrings/rocm.gpg \
        && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/rocm.gpg] https://repo.radeon.com/rocm/apt/6.3 jammy main" \
            > /etc/apt/sources.list.d/rocm.list \
        && apt-get update \
        && apt-get install -y --no-install-recommends amd-smi-lib \
        && apt-get purge -y wget gnupg2 \
        && apt-get autoremove -y \
        && rm -rf /var/lib/apt/lists/*; \
    fi

ENV PATH="/opt/rocm/bin:${PATH}"
ENV LD_LIBRARY_PATH="/opt/rocm/lib:/opt/rocm/lib64:${LD_LIBRARY_PATH}"

COPY --from=builder /app/gpu-exporter /usr/local/bin/

EXPOSE 9400

ENTRYPOINT ["gpu-exporter"]
