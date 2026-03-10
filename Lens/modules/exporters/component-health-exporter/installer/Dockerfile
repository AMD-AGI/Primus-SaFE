# Dockerfile for component-health-exporter
# Lightweight build: K8s + Prometheus collector only

ARG REGISTRY=""
ARG GO_VERSION=1.24.7
ARG GOPROXY=https://proxy.golang.org,direct

FROM ${REGISTRY}golang:${GO_VERSION} AS builder
WORKDIR /app

ARG GOPROXY
ENV GOPROXY=${GOPROXY}
ENV GOPRIVATE="github.com/AMD-AGI/*"

COPY . .

RUN cd Lens/modules/core && go mod tidy
RUN cd Lens/modules/exporters/component-health-exporter && go mod tidy && \
    CGO_ENABLED=0 go build -o component-health-exporter ./cmd/component-health-exporter

# Runtime stage
FROM ${REGISTRY}ubuntu:22.04
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /root
COPY --from=builder /app/Lens/modules/exporters/component-health-exporter/component-health-exporter .
CMD ["./component-health-exporter"]
