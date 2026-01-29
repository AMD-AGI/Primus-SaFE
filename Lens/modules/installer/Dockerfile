# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Build stage
ARG REGISTRY=docker.io
ARG GO_VERSION=1.24.7
FROM ${REGISTRY}/primussafe/golang:${GO_VERSION}-alpine AS builder

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

# Charts download stage - pull all required charts for offline installation
FROM ${REGISTRY}/primussafe/alpine:3.19 AS charts-downloader

RUN apk add --no-cache ca-certificates curl

# Install helm for pulling charts
RUN curl -fsSL https://get.helm.sh/helm-v3.14.0-linux-amd64.tar.gz | tar xz && \
    mv linux-amd64/helm /usr/local/bin/ && \
    rm -rf linux-amd64

# Build arguments for chart version and registry credentials
ARG CHART_VERSION=latest
ARG CHART_REGISTRY=oci://docker.io/primussafe
ARG REGISTRY_USERNAME
ARG REGISTRY_PASSWORD

WORKDIR /charts

# Login to registry if credentials provided
RUN if [ -n "$REGISTRY_USERNAME" ] && [ -n "$REGISTRY_PASSWORD" ]; then \
        echo "$REGISTRY_PASSWORD" | helm registry login docker.io -u "$REGISTRY_USERNAME" --password-stdin; \
    fi

# Pull all required charts
# Charts are downloaded as .tgz files to /charts directory
RUN helm pull ${CHART_REGISTRY}/primus-lens-operators --version ${CHART_VERSION} --destination /charts || \
    helm pull ${CHART_REGISTRY}/primus-lens-operators --destination /charts

RUN helm pull ${CHART_REGISTRY}/primus-lens-infrastructure --version ${CHART_VERSION} --destination /charts || \
    helm pull ${CHART_REGISTRY}/primus-lens-infrastructure --destination /charts

RUN helm pull ${CHART_REGISTRY}/primus-lens-init --version ${CHART_VERSION} --destination /charts || \
    helm pull ${CHART_REGISTRY}/primus-lens-init --destination /charts

RUN helm pull ${CHART_REGISTRY}/primus-lens-apps-dataplane --version ${CHART_VERSION} --destination /charts || \
    helm pull ${CHART_REGISTRY}/primus-lens-apps-dataplane --destination /charts

# List downloaded charts for verification
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
COPY --from=charts-downloader /charts/ /app/charts/

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
