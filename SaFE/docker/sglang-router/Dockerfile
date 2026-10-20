# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Adds an Anthropic Messages API front to the SGLang gateway of an Atom image.
# The router role runs sglang-router-gateway, which starts sglang_router on
# loopback and serves /v1/messages on the public port.
# sshd listens on 2222. The platform command is /bin/sh -c, so /bin/sh starts
# sshd before the shell command when the image entrypoint is replaced.
ARG BASE_IMAGE=harbor.tw325.primus-safe.amd.com/primussafe/atom-dev:20261019082550

FROM ${BASE_IMAGE}

COPY anthropic_proxy.py /opt/sglang-router/anthropic_proxy.py
COPY sglang-router-gateway.sh /usr/local/bin/sglang-router-gateway
COPY sshd-init.sh /usr/local/bin/sshd-init.sh
COPY sh-wrapper.sh /usr/local/bin/sh-wrapper.sh
COPY entrypoint.sh /usr/local/bin/sglang-router-entrypoint
# Replace /usr/bin/sh with a file. Writing through the symlink would overwrite dash.
# /bin/sh is that same file on a merged /usr layout, and a separate copy otherwise.
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        openssh-server openssh-client openssh-sftp-server \
    && rm -rf /var/lib/apt/lists/* \
    && chmod 0755 /usr/local/bin/sglang-router-gateway /usr/local/bin/sshd-init.sh \
        /usr/local/bin/sh-wrapper.sh /usr/local/bin/sglang-router-entrypoint \
    && rm -f /usr/bin/sh \
    && cp /usr/local/bin/sh-wrapper.sh /usr/bin/sh \
    && if [ ! /bin/sh -ef /usr/bin/sh ]; then rm -f /bin/sh && cp /usr/local/bin/sh-wrapper.sh /bin/sh; fi \
    && mkdir -p /root/.ssh /run/sshd \
    && chmod 700 /root/.ssh \
    && if [ ! -s /root/.ssh/id_ed25519 ]; then \
        ssh-keygen -q -t ed25519 -N "" -f /root/.ssh/id_ed25519 \
        && cp /root/.ssh/id_ed25519.pub /root/.ssh/authorized_keys \
        && chmod 600 /root/.ssh/id_ed25519 /root/.ssh/authorized_keys; \
    fi \
    && printf '%s\n' \
        'Host *' \
        '  Port 2222' \
        '  StrictHostKeyChecking no' \
        '  UserKnownHostsFile /dev/null' \
        '  IdentityFile /root/.ssh/id_ed25519' \
        > /root/.ssh/config \
    && chmod 600 /root/.ssh/config \
    && ssh-keygen -A

ENTRYPOINT ["/usr/local/bin/sglang-router-entrypoint"]
CMD ["tail", "-f", "/dev/null"]
