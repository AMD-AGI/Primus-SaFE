# Perfetto Viewer Container - Self-hosted Offline Version
# Builds Perfetto UI with proper Emscripten environment
#
# Build time: ~15-20 minutes
# Final image size: ~60MB

# Stage 1: Build Perfetto UI
FROM ubuntu:22.04 AS builder

# Avoid interactive prompts
ENV DEBIAN_FRONTEND=noninteractive

# Install all required build dependencies
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

# Install build dependencies (this downloads Node.js, emsdk, etc.)
RUN tools/install-build-deps --ui

# Source emsdk environment and generate build config
RUN bash -c "source buildtools/linux64/emsdk/emsdk_env.sh && tools/gn gen out/default --args='is_debug=false'"

# Build the UI with emsdk environment
RUN bash -c "source buildtools/linux64/emsdk/emsdk_env.sh && tools/ninja -C out/default ui"

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
