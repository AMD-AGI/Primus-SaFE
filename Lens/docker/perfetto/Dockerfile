# Perfetto Viewer Container
# A lightweight container that serves Perfetto UI with a pre-loaded trace file

# Stage 1: Build Perfetto UI from official Google source
FROM python:3.11-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    nodejs \
    npm \
    ninja-build \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Clone official Google Perfetto repository
# Using a specific version tag for reproducibility
ARG PERFETTO_VERSION=v49.0
RUN git clone --depth 1 --branch ${PERFETTO_VERSION} https://github.com/google/perfetto.git perfetto

WORKDIR /build/perfetto

# Install build dependencies and build the UI
# Perfetto uses its own build system
RUN tools/install-build-deps --ui && \
    tools/gn gen out/dist --args='is_debug=false' && \
    tools/ninja -C out/dist ui

# Stage 2: Runtime image
FROM nginx:alpine

# Install required tools
RUN apk add --no-cache curl

# Create data directory for trace files
RUN mkdir -p /data

# Copy built Perfetto UI from builder stage
COPY --from=builder /build/perfetto/out/dist/ui /usr/share/nginx/html/perfetto

# Copy nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
