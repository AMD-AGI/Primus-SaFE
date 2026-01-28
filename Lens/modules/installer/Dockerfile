# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Build stage
ARG GO_VERSION=1.24.7
FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /workspace

# Copy go.mod files first for better caching
# Note: paths are relative to repo root (build context is ".")
COPY Lens/modules/core/go.mod Lens/modules/core/go.sum ./modules/core/
COPY Lens/modules/installer/go.mod Lens/modules/installer/go.sum ./modules/installer/

# Download dependencies
WORKDIR /workspace/modules/installer
RUN go mod download

# Copy source code
WORKDIR /workspace
COPY Lens/modules/core/ ./modules/core/
COPY Lens/modules/installer/ ./modules/installer/

# Build
WORKDIR /workspace/modules/installer
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /installer ./cmd/main.go

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl

# Install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

# Install helm
RUN curl -fsSL https://get.helm.sh/helm-v3.14.0-linux-amd64.tar.gz | tar xz && \
    mv linux-amd64/helm /usr/local/bin/ && \
    rm -rf linux-amd64

WORKDIR /app

COPY --from=builder /installer /app/installer

# Copy database migrations
COPY Lens/modules/core/pkg/database/migrations/ /app/migrations/

# Non-root user
RUN addgroup -g 1000 installer && \
    adduser -D -u 1000 -G installer installer && \
    chown -R installer:installer /app

USER installer

ENV MIGRATIONS_PATH=/app/migrations

ENTRYPOINT ["/app/installer"]
