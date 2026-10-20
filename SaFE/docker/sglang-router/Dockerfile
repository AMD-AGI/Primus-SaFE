# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

# Adds an Anthropic Messages API front to the SGLang gateway of an Atom image.
# The router role runs sglang-router-gateway, which starts sglang_router on
# loopback and serves /v1/messages on the public port.
ARG BASE_IMAGE=harbor.tw325.primus-safe.amd.com/primussafe/atom-dev:20261019082550

FROM ${BASE_IMAGE}

COPY anthropic_proxy.py /opt/sglang-router/anthropic_proxy.py
COPY sglang-router-gateway.sh /usr/local/bin/sglang-router-gateway
RUN chmod 0755 /usr/local/bin/sglang-router-gateway
