# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Adds the Anthropic gateway, the shell wrapper, and the ABI-1 ionic provider
# on an Atom image. build.sh creates that Atom image from
# docker.io/lmsysorg/sglang-rocm:v0.5.21-rocm720-mi35x-20261008.
# Do not pass a sglang-router tag as BASE_IMAGE.
# The router role runs sglang-router-gateway, which starts sglang_router on
# loopback and serves /v1/messages on the public port.
# sshd starts only when MN_SSH_AUTHORIZED_KEY is set. The platform command is
# /bin/sh -c, so /bin/sh runs sshd-init before the shell command when the image
# entrypoint is replaced.
ARG BASE_IMAGE

FROM ${BASE_IMAGE}

COPY anthropic_proxy.py /opt/sglang-router/anthropic_proxy.py
COPY sglang-router-gateway.sh /usr/local/bin/sglang-router-gateway
COPY sshd-init.sh /usr/local/bin/sshd-init.sh
COPY sh-wrapper.sh /usr/local/bin/sh-wrapper.sh
COPY entrypoint.sh /usr/local/bin/sglang-router-entrypoint
# ABI-1 provider. libibverbs opens the libionic-rdmav34.so path.
COPY libionic.so.1.1.54.0-187 /usr/lib/x86_64-linux-gnu/libionic.so.1.1.54.0-187
# Replace /usr/bin/sh with a file. Writing through the symlink would overwrite dash.
# /bin points at usr/bin, so this file is also /bin/sh. Do not write a bin/sh
# member into the layer.
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        openssh-server openssh-client openssh-sftp-server \
    && rm -rf /var/lib/apt/lists/* \
    && chmod 0755 /usr/local/bin/sglang-router-gateway /usr/local/bin/sshd-init.sh \
        /usr/local/bin/sh-wrapper.sh /usr/local/bin/sglang-router-entrypoint \
    && rm -f /usr/bin/sh \
    && cp /usr/local/bin/sh-wrapper.sh /usr/bin/sh \
    && chmod 0644 /usr/lib/x86_64-linux-gnu/libionic.so.1.1.54.0-187 \
    && ln -sfn libionic.so.1.1.54.0-187 /usr/lib/x86_64-linux-gnu/libionic.so.1 \
    && ln -sfn ../libionic.so.1.1.54.0-187 /usr/lib/x86_64-linux-gnu/libibverbs/libionic-rdmav34.so \
    && mkdir -p /root/.ssh /run/sshd \
    && chmod 700 /root/.ssh \
    && rm -rf /run/sshd-init.lock /run/sshd-init.done /run/.containerenv

# Unset means dma-buf. These nodes reject peer-memory registration.
ENV MOONCAKE_DISABLE_HIP_DMABUF=0

ENTRYPOINT ["/usr/local/bin/sglang-router-entrypoint"]
CMD ["tail", "-f", "/dev/null"]
