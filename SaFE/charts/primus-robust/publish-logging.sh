#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# Package and publish the primus-robust-logging umbrella chart to an OCI
# registry so the `primus-robust-logging.<ver>` AddonTemplate can resolve its
# `url`/`version`. Mirrors how the other SaFE charts (primus-safe, primus-safe-cr,
# nats, cert-manager, ...) are published under <registry>/primussafe.
#
# Usage:
#   ./publish-logging.sh [REGISTRY]
#
#   REGISTRY  OCI registry host (+ optional path) to push under, WITHOUT the
#             trailing /primussafe. Defaults to registry-1.docker.io.
#             Examples:
#               ./publish-logging.sh                       # -> oci://registry-1.docker.io/primussafe
#               ./publish-logging.sh harbor.example.com    # -> oci://harbor.example.com/primussafe
#
# Prerequisites:
#   - helm v3.8+ (OCI support), logged in to the target registry
#     (`helm registry login <REGISTRY>`).
#   - The referenced container images must be mirrored to the matching
#     <image_registry>/primussafe (see the AddonTemplate header for the list).
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="${SCRIPT_DIR}/primus-robust-logging"
REGISTRY="${1:-registry-1.docker.io}"
OCI_REPO="oci://${REGISTRY}/primussafe"

CHART_VERSION="$(awk '/^version:/ {print $2; exit}' "${CHART_DIR}/Chart.yaml")"

echo "==> Building chart dependencies (file:// subcharts)"
helm dependency build "${CHART_DIR}"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

echo "==> Packaging primus-robust-logging ${CHART_VERSION}"
helm package "${CHART_DIR}" --destination "${WORKDIR}"

PKG="${WORKDIR}/primus-robust-logging-${CHART_VERSION}.tgz"

echo "==> Pushing ${PKG} to ${OCI_REPO}"
helm push "${PKG}" "${OCI_REPO}"

echo "✅ Published ${OCI_REPO}/primus-robust-logging:${CHART_VERSION}"
