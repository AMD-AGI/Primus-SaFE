# Dockerfile for primus-lens-api
# Standard lightweight build without special dependencies

ARG REGISTRY=""
ARG GO_VERSION=1.24.7
ARG BUILDPATH
ARG APPNAME
ARG BASEPATH
ARG GOPROXY=https://proxy.golang.org,direct

# Stage 1: Build stage
FROM ${REGISTRY}golang:${GO_VERSION} AS builder
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

RUN echo "BUILDPATH: $BUILDPATH" && \
    echo "BASEPATH: $BASEPATH" && \
    echo "APPNAME: $APPNAME" && \
    echo "GOPROXY: $GOPROXY"

COPY . .

RUN cd Lens/modules/core && go mod tidy
RUN cd $BASEPATH && go mod tidy && CGO_ENABLED=0 go build -tags nosqlite -a -installsuffix cgo -o $APPNAME $BUILDPATH/main.go

# Stage 2: Runtime stage - lightweight
ARG REGISTRY=""
FROM ${REGISTRY}ubuntu:22.04

ARG APPNAME
ARG BASEPATH
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

# Install common runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

RUN echo "APPNAME: $APPNAME"

COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/run.sh .
RUN chmod +x run.sh

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep -f $APPNAME || exit 1

CMD ["./run.sh"]
