# ---------- Build stage ----------
FROM node:22-alpine AS builder

WORKDIR /app

# Copy project files, install dependencies, and build
COPY . .
ARG VITE_MCP_BASE_URL
ARG VITE_MCP_API_KEY
ENV VITE_MCP_BASE_URL=${VITE_MCP_BASE_URL}
ENV VITE_MCP_API_KEY=${VITE_MCP_API_KEY}
RUN npm config set registry https://registry.npmjs.org/ \
 && npm install \
 && npm run build

# ---------- Runtime stage ----------
    FROM nginx:1.25-alpine AS runtime

    # 1. Install goaccess and bash
    RUN apk add --no-cache goaccess bash

    # 2. Create required directories
    #    - /usr/share/nginx/html         = Nginx default static directory
    #    - /usr/share/nginx/html/__internal_analytics = Analytics report page
    #    - /var/log/nginx                = Nginx access logs
    RUN mkdir -p /usr/share/nginx/html/__internal_analytics \
        && mkdir -p /var/log/nginx

    # 3. Copy build output to Nginx static directory
    COPY --from=builder /app/dist /usr/share/nginx/html/

    # 4. Copy custom nginx.conf
    COPY nginx.conf /etc/nginx/nginx.conf

    # 5. Copy startup script
    #    start.sh runs goaccess + nginx together
    COPY start.sh /start.sh
    RUN chmod +x /start.sh

    # 6. Expose ports
    #    - 80: Frontend app
    #    - 7890: goaccess real-time WebSocket
    EXPOSE 80
    EXPOSE 7890

    # 7. Use startup script as container entrypoint
    CMD ["/start.sh"]
