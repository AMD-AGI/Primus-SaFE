# Build stage
ARG GO_VERSION=1.24.7
FROM golang:${GO_VERSION}-alpine AS builder

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for caching
COPY Lens/modules/ai-gateway/go.mod Lens/modules/ai-gateway/go.sum ./ai-gateway/
COPY Lens/modules/core/go.mod Lens/modules/core/go.sum ./core/

# Download dependencies
ARG GOPROXY
ENV GOPROXY=${GOPROXY}

WORKDIR /build/ai-gateway
RUN go mod download

# Copy source code
WORKDIR /build
COPY Lens/modules/core ./core
COPY Lens/modules/ai-gateway ./ai-gateway

# Build the binary
WORKDIR /build/ai-gateway
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /ai-gateway \
    ./cmd/ai-gateway

# Final stage
FROM alpine:3.19

# Install ca-certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

# Copy binary from builder
COPY --from=builder /ai-gateway /usr/local/bin/ai-gateway

# Set ownership
RUN chown appuser:appuser /usr/local/bin/ai-gateway

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8003

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/ai-gateway"]

