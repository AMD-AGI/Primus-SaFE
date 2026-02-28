ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

ARG APP_HOME=/preprocess

# Optional driver paths (relative to build context, empty by default)
# When specified, driver files will be copied to ${APP_HOME}/drivers/
# Usage:
#   --build-arg BNXT_DRIVER_PATH=./path/to/bnxt_driver.tar.gz
#   --build-arg AINIC_DRIVER_PATH=./path/to/ainic_bundle.tar.gz
ARG BNXT_DRIVER_PATH=
ARG AINIC_DRIVER_PATH=

RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/scripts && mkdir -p ${APP_HOME}/drivers && chmod -R 750 ${APP_HOME}

COPY ./SaFE/docker/preprocess/*.sh ${APP_HOME}/
COPY ./SaFE/docker/preprocess/*.py ${APP_HOME}/
COPY ./SaFE/docker/preprocess/scripts/*.sh ${APP_HOME}/scripts/

# Copy bnxt driver if path is specified
# Use shell with heredoc to handle conditional copy
RUN --mount=type=bind,target=/build_context \
    if [ -n "${BNXT_DRIVER_PATH}" ] && [ -f "/build_context/${BNXT_DRIVER_PATH}" ]; then \
        cp "/build_context/${BNXT_DRIVER_PATH}" "${APP_HOME}/drivers/" && \
        echo "Copied bnxt driver from ${BNXT_DRIVER_PATH} to ${APP_HOME}/drivers/"; \
    else \
        echo "BNXT_DRIVER_PATH not specified or file not found, skipping."; \
    fi

# Copy ainic driver if path is specified
RUN --mount=type=bind,target=/build_context \
    if [ -n "${AINIC_DRIVER_PATH}" ] && [ -f "/build_context/${AINIC_DRIVER_PATH}" ]; then \
        cp "/build_context/${AINIC_DRIVER_PATH}" "${APP_HOME}/drivers/" && \
        echo "Copied ainic driver from ${AINIC_DRIVER_PATH} to ${APP_HOME}/drivers/"; \
    else \
        echo "AINIC_DRIVER_PATH not specified or file not found, skipping."; \
    fi
