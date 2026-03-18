# ---------- Build stage ----------
FROM node:22-alpine AS builder

WORKDIR /app

# Copy project files, install dependencies, and build
COPY . .
RUN npm install && npm run build

# ---------- Runtime stage ----------
FROM nginx:alpine

# Copy built files to Nginx static directory
WORKDIR /app
COPY --from=builder /app/dist /app/dist

# Optional: custom Nginx config (requires nginx.conf)
COPY nginx.conf /app/nginx.conf

EXPOSE 80
CMD ["nginx", "-c", "/app/nginx.conf"]
    