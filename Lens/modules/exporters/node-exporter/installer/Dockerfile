# Dockerfile for primus-lens-node-exporter
# Standard lightweight build

ARG GO_VERSION=1.24.7
ARG BUILDPATH
ARG APPNAME
ARG BASEPATH
ARG GOPROXY=https://proxy.golang.org,direct

# Stage 1: Build stage
FROM golang:${GO_VERSION} AS builder
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

# Stage 2: Runtime stage - use pre-built base image with dependencies
FROM ubuntu:22.04

ARG APPNAME
ARG BASEPATH
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

# py-spy version to install
ARG PYSPY_VERSION=0.4.1

WORKDIR /root/

RUN echo "APPNAME: $APPNAME"

# Install py-spy for Python profiling support
# py-spy is a sampling profiler for Python programs
# Download the wheel file and extract the binary directly
RUN set -eux; \
    apt-get update && apt-get install -y --no-install-recommends \
        curl \
        ca-certificates \
        unzip \
    && curl -sSfL -o /tmp/py_spy.whl \
        "https://github.com/benfred/py-spy/releases/download/v${PYSPY_VERSION}/py_spy-${PYSPY_VERSION}-py2.py3-none-manylinux_2_5_x86_64.manylinux1_x86_64.whl" \
    && unzip -j /tmp/py_spy.whl "py_spy-${PYSPY_VERSION}.data/scripts/py-spy" -d /usr/local/bin/ \
    && chmod +x /usr/local/bin/py-spy \
    && py-spy --version \
    && rm -f /tmp/py_spy.whl \
    && apt-get purge -y unzip \
    && apt-get autoremove -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create storage directory for py-spy profiling files
RUN mkdir -p /var/lib/lens/pyspy/profiles && chmod 755 /var/lib/lens/pyspy

# Copy application binary
COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/run.sh .
RUN chmod +x run.sh


CMD ["./run.sh"]

