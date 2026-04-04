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

# Optional prebuilt RCCL and AMD ANP directories (relative to build context)
# When specified, directories will be copied to ${APP_HOME}/apps/rccl and ${APP_HOME}/apps/amd-anp
# Usage:
#   --build-arg RCCL_PATH=./path/to/rccl
#   --build-arg AMD_ANP_PATH=./path/to/amd-anp
ARG RCCL_PATH=
ARG AMD_ANP_PATH=

RUN mkdir -p ${APP_HOME} && mkdir -p ${APP_HOME}/scripts && mkdir -p ${APP_HOME}/drivers && mkdir -p ${APP_HOME}/apps && chmod -R 750 ${APP_HOME}

COPY ./SaFE/docker/preprocess/*.sh ${APP_HOME}/
COPY ./SaFE/docker/preprocess/*.py ${APP_HOME}/
COPY ./SaFE/docker/preprocess/utils/ ${APP_HOME}/utils/
COPY ./SaFE/docker/preprocess/scripts/*.sh ${APP_HOME}/scripts/

# Copy bnxt driver if path is specified
# Use shell with heredoc to handle conditional copy
RUN --mount=type=bind,source=.,target=/build_context \
    if [ -n "${BNXT_DRIVER_PATH}" ] && [ -f "/build_context/${BNXT_DRIVER_PATH}" ]; then \
        cp "/build_context/${BNXT_DRIVER_PATH}" "${APP_HOME}/drivers/" && \
        echo "Copied bnxt driver from ${BNXT_DRIVER_PATH} to ${APP_HOME}/drivers/"; \
    else \
        echo "BNXT_DRIVER_PATH not specified or file not found, skipping."; \
    fi

# Copy ainic driver if path is specified
RUN --mount=type=bind,source=.,target=/build_context \
    if [ -n "${AINIC_DRIVER_PATH}" ] && [ -f "/build_context/${AINIC_DRIVER_PATH}" ]; then \
        cp "/build_context/${AINIC_DRIVER_PATH}" "${APP_HOME}/drivers/" && \
        echo "Copied ainic driver from ${AINIC_DRIVER_PATH} to ${APP_HOME}/drivers/"; \
    else \
        echo "AINIC_DRIVER_PATH not specified or file not found, skipping."; \
    fi

# Copy RCCL directory if path is specified
RUN --mount=type=bind,source=.,target=/build_context \
    if [ -n "${RCCL_PATH}" ] && [ -d "/build_context/${RCCL_PATH}" ]; then \
        cp -r "/build_context/${RCCL_PATH}" "${APP_HOME}/apps/rccl" && \
        echo "Copied RCCL from ${RCCL_PATH} to ${APP_HOME}/apps/rccl"; \
    else \
        echo "RCCL_PATH not specified or directory not found, skipping."; \
    fi

# Copy AMD ANP directory if path is specified
RUN --mount=type=bind,source=.,target=/build_context \
    if [ -n "${AMD_ANP_PATH}" ] && [ -d "/build_context/${AMD_ANP_PATH}" ]; then \
        cp -r "/build_context/${AMD_ANP_PATH}" "${APP_HOME}/apps/amd-anp" && \
        echo "Copied AMD ANP from ${AMD_ANP_PATH} to ${APP_HOME}/apps/amd-anp"; \
    else \
        echo "AMD_ANP_PATH not specified or directory not found, skipping."; \
    fi
