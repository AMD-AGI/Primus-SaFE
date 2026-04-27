# Unified frontend image: serves both SaFE (at /) and Lens (at /lens/)
# from a single nginx container.
#
# Build context MUST be the Web/ directory (the pnpm workspace root):
#   docker build -f Web/Dockerfile -t primus-web:<tag> Web/
#
# Optional build args (forwarded to the SaFE app at build time):
#   --build-arg VITE_MCP_BASE_URL=...
#   --build-arg VITE_MCP_API_KEY=...

ARG NODE_VERSION=22-alpine
ARG NGINX_VERSION=1.25-alpine

# ---------- Stage 1: install workspace dependencies ----------
# Cached as long as lockfile + package.json files are unchanged.
FROM node:${NODE_VERSION} AS deps
WORKDIR /workspace

RUN corepack enable && corepack prepare pnpm@8.15.6 --activate

COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/safe/package.json ./apps/safe/
COPY apps/lens/package.json ./apps/lens/

RUN pnpm install --frozen-lockfile

# ---------- Stage 2: build both apps ----------
FROM deps AS builder

ARG VITE_MCP_BASE_URL
ARG VITE_MCP_API_KEY
ENV VITE_MCP_BASE_URL=${VITE_MCP_BASE_URL}
ENV VITE_MCP_API_KEY=${VITE_MCP_API_KEY}

COPY apps/safe ./apps/safe
COPY apps/lens ./apps/lens

RUN pnpm --filter ./apps/safe build \
 && pnpm --filter ./apps/lens build

# ---------- Stage 3: nginx runtime ----------
FROM nginx:${NGINX_VERSION} AS runtime

# goaccess + bash power the built-in /__internal_analytics dashboard
# (carried over from the previous SaFE-only image).
RUN apk add --no-cache goaccess bash

RUN mkdir -p /usr/share/nginx/html/__internal_analytics \
 && mkdir -p /var/log/nginx

# SaFE goes to the web root; Lens lives under /lens/ to match its
# vite `base: '/lens/'` configuration. The two dist trees never collide
# because Lens emits all asset URLs as /lens/assets/*.
COPY --from=builder /workspace/apps/safe/dist/ /usr/share/nginx/html/
COPY --from=builder /workspace/apps/lens/dist/ /usr/share/nginx/html/lens/

COPY nginx.conf /etc/nginx/nginx.conf
COPY start.sh /start.sh
RUN chmod +x /start.sh

# 80   = HTTP
# 7890 = goaccess real-time WebSocket (proxied via /__internal_analytics/ws)
EXPOSE 80
EXPOSE 7890

CMD ["/start.sh"]
