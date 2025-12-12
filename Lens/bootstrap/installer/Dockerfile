# Primus Lens Installer Container
# This container includes the installer binary, all Helm charts, and required tools.
#
# Build with: make docker-build (from the installer directory)
# This will create a build context with all charts included.

# Build stage - compile the installer
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (excluding charts which are added separately)
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY internal/ internal/

# Build the installer binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o primus-lens-installer ./cmd/installer

# Final stage - minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    curl \
    bash

# Install kubectl
ARG KUBECTL_VERSION=v1.29.0
RUN curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

# Install helm
ARG HELM_VERSION=v3.14.0
RUN curl -fsSL https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz | tar xz && \
    mv linux-amd64/helm /usr/local/bin/ && \
    rm -rf linux-amd64

# Copy installer binary from builder
COPY --from=builder /build/primus-lens-installer /primus-lens-installer

# Copy all Helm charts (added to build context by Makefile)
COPY charts /charts

# Set charts directory environment variable
ENV CHARTS_DIR=/charts

WORKDIR /

# Default command
ENTRYPOINT ["/primus-lens-installer"]
CMD ["--help"]

