# Dockerfile for gpu-resource-exporter
# Standard lightweight build

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
  
RUN echo "BUILDPATH: $BUILDPATH" && \
    echo "BASEPATH: $BASEPATH" && \
    echo "APPNAME: $APPNAME" && \
    echo "GOPROXY: $GOPROXY"

COPY . .

RUN cd $BASEPATH && CGO_ENABLED=0 go build -tags nosqlite -a -installsuffix cgo -o $APPNAME $BUILDPATH/main.go

# Stage 2: Runtime stage - lightweight
ARG REGISTRY
FROM --platform=linux/amd64 ${REGISTRY}/primussafe/ubuntu-rdma-base:0.0.1

ARG APPNAME
ARG BASEPATH
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

WORKDIR /root/

RUN echo "APPNAME: $APPNAME"

COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/run.sh .
RUN chmod +x run.sh

CMD ["./run.sh"]

