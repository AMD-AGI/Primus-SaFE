FROM golang:1.23.4-alpine AS builder

WORKDIR /app

COPY SaFE/tools/sync-image .

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

COPY SaFE/tools/sync-image/config.yaml /app/config.yaml

CMD ["./app"]