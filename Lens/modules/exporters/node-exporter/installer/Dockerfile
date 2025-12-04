# Dockerfile for primus-lens-node-exporter
# Standard lightweight build

ARG GO_VERSION=1.24.7
ARG BUILDPATH
ARG APPNAME
ARG BASEPATH

# Stage 1: Build stage
FROM golang:${GO_VERSION} AS builder
WORKDIR /app

ARG GITHUB_TOKEN
ARG BASEPATH
ARG BUILDPATH
ARG APPNAME
ENV BUILDPATH=${BUILDPATH}
ENV APPNAME=${APPNAME}
ENV BASEPATH=${BASEPATH}

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

RUN cd $BASEPATH && CGO_ENABLED=0 go build -tags nosqlite -a -installsuffix cgo -o $APPNAME $BUILDPATH/main.go

# Stage 2: Runtime stage - with Python Inspector support
FROM --platform=linux/amd64 primussafe/ubuntu-rdma-base:0.0.1

ARG APPNAME
ARG BASEPATH
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

WORKDIR /root/

RUN echo "APPNAME: $APPNAME"

# Install Python Inspector dependencies
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    gdb \
    && rm -rf /var/lib/apt/lists/*

# Install pyrasite for process injection
RUN pip3 install pyrasite --break-system-packages

# Copy application binary
COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/run.sh .
RUN chmod +x run.sh

# Copy inspection scripts
COPY --from=builder /app/Lens/modules/exporters/node-exporter/pkg/collector/python-inspector/scripts /opt/primus-lens/inspector-scripts

CMD ["./run.sh"]

