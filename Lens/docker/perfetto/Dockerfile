# Perfetto Viewer Container
# A lightweight container that serves Perfetto UI with a pre-loaded trace file

FROM nginx:alpine

# Install required tools
RUN apk add --no-cache curl jq unzip

# Download and extract Perfetto UI
# Using a standalone build from nicpottier/nicperfetto (simplified Perfetto build)
RUN mkdir -p /usr/share/nginx/html/perfetto && \
    cd /tmp && \
    curl -L https://github.com/nicpottier/nicperfetto/releases/download/v32.0/nicperfetto-v32.0.zip -o perfetto.zip && \
    unzip -o perfetto.zip -d /usr/share/nginx/html/perfetto && \
    rm perfetto.zip

# Create data directory for trace files
RUN mkdir -p /data

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

