# Perfetto Viewer Container
# A lightweight container that serves Perfetto UI with a pre-loaded trace file

# Stage 1: Download Perfetto UI
FROM alpine:3.19 AS downloader

RUN apk add --no-cache wget ca-certificates

WORKDIR /download

# Download Perfetto UI using wget mirror mode
# This downloads all necessary files for the UI to work
RUN wget --mirror \
    --convert-links \
    --adjust-extension \
    --page-requisites \
    --no-parent \
    --no-host-directories \
    --reject "*.wasm.gz" \
    --timeout=60 \
    --tries=3 \
    -P /download \
    "https://ui.perfetto.dev/" 2>&1 || true

# Stage 2: Runtime image
FROM nginx:alpine

# Install required tools
RUN apk add --no-cache curl

# Create data directory for trace files
RUN mkdir -p /data

# Copy downloaded Perfetto UI
COPY --from=downloader /download /usr/share/nginx/html/perfetto

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
