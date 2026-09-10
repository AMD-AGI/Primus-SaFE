# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Minimal squid image for the CI/CD proxy-relay sidecar. The pod startup
# script writes /tmp/squid.conf and execs squid -N at runtime.

ARG REGISTRY=docker.io

FROM ${REGISTRY}/library/ubuntu:22.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends squid && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
