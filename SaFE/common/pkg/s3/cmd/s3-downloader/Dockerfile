# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy all required modules (common, apis, utils) for replace directives
COPY common/go.mod common/go.sum ./common/
COPY apis/go.mod apis/go.sum ./apis/
COPY utils/go.mod utils/go.sum ./utils/

# Download dependencies
WORKDIR /build/common
RUN go mod download

# Copy source code for all modules
WORKDIR /build
COPY common/pkg ./common/pkg
COPY apis ./apis
COPY utils ./utils

# Build the application
WORKDIR /build/common
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o s3-downloader ./pkg/s3/cmd/s3-downloader

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/common/s3-downloader .

# Run as non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

ENTRYPOINT ["/app/s3-downloader"]

