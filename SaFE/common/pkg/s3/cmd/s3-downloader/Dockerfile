# Build stage
# go 1.25.5 to match the go.mod `go 1.25.5` directive (1.24 fails `go mod
# download` with GOTOOLCHAIN=local). Matches the other SaFE Dockerfiles.
FROM golang:1.25.5-alpine AS builder

WORKDIR /build

# Copy all required modules (common, apis, utils) for replace directives
COPY SaFE/common/go.mod SaFE/common/go.sum ./common/
COPY SaFE/apis/go.mod SaFE/apis/go.sum ./apis/
COPY SaFE/utils/go.mod SaFE/utils/go.sum ./utils/

# Download dependencies
WORKDIR /build/common
RUN go mod download

# Copy source code for all modules
WORKDIR /build
COPY SaFE/common/pkg ./common/pkg
COPY SaFE/apis ./apis
COPY SaFE/utils ./utils

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

