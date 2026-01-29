FROM golang:1.24.7-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go.mod files for dependency caching
COPY modules/core/go.mod modules/core/go.sum ./modules/core/
COPY modules/jobs/go.mod modules/jobs/go.sum ./modules/jobs/
COPY modules/control-plane-controller/go.mod ./modules/control-plane-controller/

# Download dependencies
WORKDIR /app/modules/control-plane-controller
RUN go mod download

# Copy source code
WORKDIR /app
COPY modules/core/ ./modules/core/
COPY modules/jobs/ ./modules/jobs/
COPY modules/control-plane-controller/ ./modules/control-plane-controller/

# Build
WORKDIR /app/modules/control-plane-controller
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /control-plane-controller ./cmd/control-plane-controller

# Final image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /control-plane-controller /usr/local/bin/control-plane-controller

# Create non-root user
RUN adduser -D -u 1000 appuser
USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/control-plane-controller"]
