ARG BUILDPATH
ARG APPNAME
ARG BASEPATH

FROM golang:1.24.7 AS builder
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
    echo "✅ APPNAME=$APPNAME" && \
    echo "✅ GITHUB_TOKEN=$(echo $GITHUB_TOKEN | cut -c1-6)******"


RUN git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"

RUN echo "BUILDPATH: $BUILDPATH" && \
    echo "BASEPATH: $BASEPATH" && \
        echo "APPNAME: $APPNAME" && \
        echo "GOPROXY: $GOPROXY"

COPY . .

RUN cd $BASEPATH && CGO_ENABLED=0 go build -tags nosqlite -a -installsuffix cgo -o $APPNAME $BUILDPATH/main.go

FROM --platform=linux/amd64  primussafe/ubuntu-rdma-base:0.0.1

# 定义并导出环境变量
ARG APPNAME
ARG BASEPATH
ENV BASEPATH=${BASEPATH}
ENV APPNAME=${APPNAME}

WORKDIR /root/

RUN echo "APPNAME: $APPNAME"

COPY --from=builder /app/$BASEPATH/$APPNAME .
COPY --from=builder /app/Lens/ci/build/run.sh .
RUN chmod +x run.sh


# 运行应用程序
CMD ["./run.sh"]