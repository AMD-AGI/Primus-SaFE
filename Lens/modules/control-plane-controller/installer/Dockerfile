# Dockerfile for control-plane-controller
# Runs management plane jobs: dataplane_installer, multi_cluster_config_sync,
# tracelens_cleanup, gpu_usage_weekly_report

ARG REGISTRY=docker.io
ARG GO_VERSION=1.24.7
ARG BUILDPATH
ARG APPNAME
ARG BASEPATH
ARG GOPROXY=https://proxy.golang.org,direct

# Stage 1: Build stage
FROM ${REGISTRY}/primussafe/golang:${GO_VERSION} AS builder
WORKDIR /app

ARG GITHUB_TOKEN
ARG BASEPATH
ARG BUILDPATH
ARG APPNAME
ARG GOPROXY
ENV BUILDPATH=${BUILDPATH}
ENV APPNAME=${APPNAME}
ENV BASEPATH=${BASEPATH}
ENV GOPROXY=${GOPROXY}

ENV GOPRIVATE="github.com/AMD-AGI/*"
SHELL ["/bin/bash", "-c"]

RUN echo "✅ GOPRIVATE=$GOPRIVATE" && \
    echo "✅ BUILDPATH=$BUILDPATH" && \
    echo "✅ BASEPATH=$BASEPATH" && \
    echo "✅ APPNAME=$APPNAME"

COPY . .

RUN cd $BASEPATH && CGO_ENABLED=0 go build -tags nosqlite -a -installsuffix cgo -o $APPNAME $BUILDPATH/main.go

# Stage 2: Runtime stage
ARG REGISTRY
FROM ${REGISTRY}/primussafe/debian:bookworm-slim

ARG APPNAME
ARG BASEPATH
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

WORKDIR /root/

# Install minimal dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

RUN echo "APPNAME: $APPNAME"

COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/run.sh .
RUN chmod +x run.sh

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD pgrep -f $APPNAME || exit 1

CMD ["./run.sh"]
