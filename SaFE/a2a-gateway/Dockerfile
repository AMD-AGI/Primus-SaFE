FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o a2a-gateway ./cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/a2a-gateway /usr/local/bin/a2a-gateway
EXPOSE 8089 9090
ENTRYPOINT ["a2a-gateway"]
