ARG GO_VERSION=1.24.7
ARG BUILDPATH=cmd/skills-repository
ARG APPNAME=skills-repository
ARG BASEPATH=Lens/modules/skills-repository/
ARG GOPROXY=https://proxy.golang.org,direct

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
    echo "✅ APPNAME=$APPNAME" && \
    echo "✅ GOPROXY=$GOPROXY"

COPY . .

# Ensure dependencies are up to date
RUN cd Lens/modules/core && go mod tidy
RUN cd $BASEPATH && go mod tidy && CGO_ENABLED=0 go build -tags nosqlite -a -installsuffix cgo -o $APPNAME $BUILDPATH/main.go

FROM ubuntu:22.04

# Install common runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

ARG APPNAME=skills-repository
ARG BASEPATH=Lens/modules/skills-repository/
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

WORKDIR /root/

RUN echo "APPNAME: $APPNAME"

COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/run.sh .
RUN chmod +x run.sh

EXPOSE 8080

CMD ["./run.sh"]
