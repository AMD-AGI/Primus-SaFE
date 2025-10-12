# 使用官方 Go 镜像作为构建阶段
FROM golang:1.23.4-alpine AS builder

WORKDIR /app

COPY tools/sync-image .

# 下载依赖
RUN go mod tidy

RUN apk add --no-cache \
    gpgme-dev \
    pkgconfig \
    build-base

RUN go build -o app .

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache \
    gpgme-dev \
    pkgconfig \
    build-base

COPY --from=builder /app/app .

COPY tools/sync-image/config.yaml /app/config.yaml

CMD ["./app"]