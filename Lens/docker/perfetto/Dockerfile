# Perfetto Viewer Container - trace_processor HTTP Mode
# Uses server-side trace processing for instant trace viewing
#
# Architecture:
#   - trace_processor_shell runs as HTTP server on port 9001
#   - nginx serves Perfetto UI and proxies API requests
#   - Browser connects to trace_processor via HTTP API (no WASM needed)
#
# Build time: ~15-20 minutes (includes Perfetto UI build)
# Final image size: ~80MB

# Stage 1: Build Perfetto UI and get trace_processor_shell
FROM ubuntu:22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

# Install build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    python3 \
    python3-pip \
    python3-distutils \
    git \
    curl \
    lbzip2 \
    xz-utils \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Clone Perfetto repository
RUN git clone --depth 1 https://android.googlesource.com/platform/external/perfetto

WORKDIR /build/perfetto

# Install build dependencies
RUN tools/install-build-deps --ui

# Build UI
RUN ui/build && \
    mkdir -p /perfetto-dist && \
    cp -r out/ui/ui/dist/* /perfetto-dist/

# Build trace_processor_shell (native binary for server-side processing)
RUN tools/gn gen out/default --args='is_debug=false' && \
    tools/ninja -C out/default trace_processor_shell && \
    cp out/default/trace_processor_shell /trace_processor_shell

# Stage 2: Runtime image (use debian-based nginx for glibc compatibility)
FROM nginx:latest

# Install required tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    sed \
    && rm -rf /var/lib/apt/lists/*

# Create directories
RUN mkdir -p /data /usr/share/nginx/html/perfetto

# Copy trace_processor_shell binary
COPY --from=builder /trace_processor_shell /usr/bin/trace_processor_shell
RUN chmod +x /usr/bin/trace_processor_shell

# Copy built Perfetto UI and fix permissions
COPY --from=builder /perfetto-dist /usr/share/nginx/html/perfetto
RUN chmod -R 755 /usr/share/nginx/html/perfetto && \
    find /usr/share/nginx/html/perfetto -type f -exec chmod 644 {} \;

# Copy custom scripts
COPY url-fix.js /usr/share/nginx/html/perfetto/url-fix.js
COPY httpd-connect.js /usr/share/nginx/html/perfetto/httpd-connect.js

# Copy nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
