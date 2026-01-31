# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy module files
COPY modules/core/go.mod modules/core/go.sum ./modules/core/
COPY modules/skills-repository/go.mod modules/skills-repository/go.sum* ./modules/skills-repository/

# Download dependencies
WORKDIR /app/modules/skills-repository
RUN go mod download

# Copy source code
WORKDIR /app
COPY modules/core ./modules/core
COPY modules/skills-repository ./modules/skills-repository

# Build
WORKDIR /app/modules/skills-repository
RUN CGO_ENABLED=0 GOOS=linux go build -o /skills-repository ./cmd/skills-repository

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /skills-repository /app/skills-repository

EXPOSE 8080

ENTRYPOINT ["/app/skills-repository"]
