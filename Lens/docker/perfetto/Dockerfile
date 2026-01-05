# Perfetto Viewer Container - Self-hosted Offline Version
# Builds Perfetto UI from official source for complete offline/private deployment
#
# Build time: ~10-15 minutes (depends on network and CPU)
# Final image size: ~60MB

# Stage 1: Build Perfetto UI from official source
FROM debian:bookworm-slim AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    python3 \
    python3-distutils \
    ca-certificates \
    lbzip2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Clone Perfetto repository (shallow clone for faster build)
RUN git clone --depth 1 https://android.googlesource.com/platform/external/perfetto

WORKDIR /build/perfetto

# Install UI build dependencies (downloads Node.js, npm packages, etc.)
RUN tools/install-build-deps --ui

# Generate build configuration
RUN tools/gn gen out/default --args='is_debug=false'

# Build the UI
RUN tools/ninja -C out/default ui

# Stage 2: Runtime image
FROM nginx:alpine

# Install required tools
RUN apk add --no-cache curl

# Create directories
RUN mkdir -p /data /usr/share/nginx/html/perfetto

# Copy built Perfetto UI from builder stage
COPY --from=builder /build/perfetto/out/default/ui /usr/share/nginx/html/perfetto

# Copy custom auto-load script
COPY auto-load.js /usr/share/nginx/html/perfetto/auto-load.js

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
