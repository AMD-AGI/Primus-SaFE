ARG REGISTRY=docker.io
ARG GO_VERSION=1.25.5

# save-image runs inside workload containers (see resource-manager/cmd/save-image): a
# static binary, so it runs on any image of the platform this image is built for.
FROM ${REGISTRY}/library/golang:${GO_VERSION} AS save-image
ARG GOPROXY
ARG TARGETARCH
ENV GOPROXY=${GOPROXY} CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64}
COPY ./SaFE /src
WORKDIR /src
RUN go build -trimpath -ldflags='-s -w' -o /out/save-image ./resource-manager/cmd/save-image

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

# 0755 so a non-root init (PSA restricted / external provider identity) can
# traverse ${APP_HOME} and copy its contents to the shared volume.
RUN mkdir -p ${APP_HOME}/scripts ${APP_HOME}/drivers ${APP_HOME}/apps \
    && chmod 0755 ${APP_HOME} ${APP_HOME}/scripts ${APP_HOME}/drivers ${APP_HOME}/apps

COPY ./SaFE/docker/preprocess/*.sh ${APP_HOME}/
COPY ./SaFE/docker/preprocess/*.py ${APP_HOME}/
COPY ./SaFE/docker/preprocess/scripts/*.sh ${APP_HOME}/scripts/
COPY --from=save-image /out/save-image ${APP_HOME}/save-image

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

# World-readable tree: dirs 0755, files 0644, scripts 0755. Default process user is
# non-root so the init matches PSA restricted / OpenShift SCC expectations.
RUN find "${APP_HOME}" -type d -exec chmod 0755 {} + \
    && find "${APP_HOME}" -type f -exec chmod 0644 {} + \
    && find "${APP_HOME}" -type f -name '*.sh' -exec chmod 0755 {} + \
    && chmod 0755 "${APP_HOME}/save-image" \
    && useradd --uid 65532 --user-group --no-create-home --shell /usr/sbin/nologin preprocess \
    && chown -R preprocess:preprocess "${APP_HOME}"

USER 65532:65532
