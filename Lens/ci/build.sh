#!/bin/bash

# 检查是否传入模块名参数
if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <module_name>"
    exit 1
fi

MODULE_NAME=$1
BUILD_FILE="build/build.txt"
DOCKERFILE="build/Dockerfile"
IMAGES_FILE="build/images.txt"

# 查找build.txt中的路径和APPNAME
APP_PATH=$(grep "^${MODULE_NAME} " ${BUILD_FILE} | awk '{print $2}' | tr -d '\r')
APP_NAME=$(grep "^${MODULE_NAME} " ${BUILD_FILE} | awk '{print $1}' | tr -d '\r')
BASE_PATH=$(grep "^${MODULE_NAME} " ${BUILD_FILE} | awk '{print $3}' | tr -d '\r')

if [ -z "${APP_PATH}" ] || [ -z "${APP_NAME}" ] || [ -z "${BASE_PATH}" ]; then
    echo "Module name ${MODULE_NAME} not found in ${BUILD_FILE}"
    exit 1
fi

# 获取当前时间戳
TIMESTAMP=$(date +%Y%m%d%H%M)

echo "Building Docker image for ${APP_NAME} in ${BASE_PATH} ${APP_PATH} with timestamp ${TIMESTAMP}"

# 构建镜像
docker buildx build --platform=linux/amd64 --build-arg BUILDPATH=${APP_PATH} --build-arg BASEPATH=${BASE_PATH} --build-arg APPNAME=${APP_NAME} -t ${APP_NAME}:${TIMESTAMP} -f ${DOCKERFILE} . --load

if [ $? -ne 0 ]; then
    echo "Docker build failed"
    exit 1
fi

REPOSITORIES=("harbor.safe.primus.ai/primussafe/${APP_NAME}")
#REPOSITORIES=("primussafe/${APP_NAME}")

# 推送镜像到多个仓库
for REPO in "${REPOSITORIES[@]}"; do
    docker tag ${APP_NAME}:${TIMESTAMP} ${REPO}:${TIMESTAMP}
    docker push ${REPO}:${TIMESTAMP}
    if [ $? -ne 0 ]; then
        echo "Docker push to ${REPO} failed"
        exit 1
    fi
done

echo "Docker image ${APP_NAME}:${TIMESTAMP} built and pushed to all repositories successfully"