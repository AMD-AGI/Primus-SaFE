# Perfetto Viewer Container
# Uses iframe + postMessage API to embed official Perfetto UI
# This is the recommended integration method by Perfetto team

FROM nginx:alpine

# Install required tools
RUN apk add --no-cache curl

# Create data directory for trace files
RUN mkdir -p /data /usr/share/nginx/html/perfetto

# Copy static HTML files
COPY index.html /usr/share/nginx/html/perfetto/index.html

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
