# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Installer image - bundles Helm charts and runs dataplane installation
# Build stage
ARG REGISTRY=docker.io
ARG GO_VERSION=1.24.7
FROM ${REGISTRY}/primussafe/golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /workspace

# Copy source code
# Note: paths are relative to repo root (build context is ".")
COPY Lens/modules/core/ ./modules/core/
COPY Lens/modules/installer/ ./modules/installer/

# Download dependencies and tidy
WORKDIR /workspace/modules/installer
RUN go mod tidy && go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /installer ./cmd/main.go

# Charts packaging stage - package local charts with dependencies for offline installation
FROM ${REGISTRY}/primussafe/alpine:3.19 AS charts-packager

RUN apk add --no-cache ca-certificates curl

# Install helm for packaging charts
RUN curl -fsSL https://get.helm.sh/helm-v3.14.0-linux-amd64.tar.gz | tar xz && \
    mv linux-amd64/helm /usr/local/bin/ && \
    rm -rf linux-amd64

WORKDIR /workspace

# Copy local charts source from repo
# Note: paths are relative to repo root (build context is ".")
COPY Lens/charts/primus-lens-operators/ ./charts/primus-lens-operators/
COPY Lens/charts/primus-lens-infrastructure/ ./charts/primus-lens-infrastructure/
COPY Lens/charts/primus-lens-init/ ./charts/primus-lens-init/
COPY Lens/charts/primus-lens-apps-dataplane/ ./charts/primus-lens-apps-dataplane/

# Create output directory for packaged charts
RUN mkdir -p /charts

# Download dependencies and package charts
# primus-lens-operators has external dependencies (victoria-metrics-operator, fluent-operator, etc.)
RUN cd /workspace/charts/primus-lens-operators && \
    helm dependency update && \
    helm package . -d /charts

# Package charts without dependencies
RUN helm package /workspace/charts/primus-lens-infrastructure -d /charts
RUN helm package /workspace/charts/primus-lens-init -d /charts
RUN helm package /workspace/charts/primus-lens-apps-dataplane -d /charts

# List packaged charts for verification
RUN ls -la /charts/

# Runtime stage
ARG REGISTRY
FROM ${REGISTRY}/primussafe/alpine:3.19

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

# Copy bundled charts for offline installation
COPY --from=charts-packager /charts/ /app/charts/

# Non-root user
RUN addgroup -g 1000 installer && \
    adduser -D -u 1000 -G installer installer && \
    chown -R installer:installer /app

USER installer

ENV MIGRATIONS_PATH=/app/migrations
# Enable local charts by default
ENV HELM_USE_LOCAL_CHARTS=true
ENV HELM_LOCAL_CHARTS_PATH=/app/charts

ENTRYPOINT ["/app/installer"]
