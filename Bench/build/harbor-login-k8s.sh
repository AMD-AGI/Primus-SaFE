#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
# docker login to Harbor using admin password from in-cluster secret (no static credentials in repo).
#
# Prerequisites: kubectl configured for the cluster where Harbor runs; secret harbor-core in ns harbor.
#
# Usage:
#   ./build/harbor-login-k8s.sh [REGISTRY_HOST]
#
# REGISTRY_HOST defaults to the harbor ingress host of the cluster kubectl
# currently points at, so this works on any cluster. It used to default to
# harbor.oci-slc.primus-safe.amd.com, which logged in to the wrong registry
# on every other cluster.
#

set -euo pipefail

REGISTRY_HOST="${1:-}"
SECRET_NS="${HARBOR_SECRET_NAMESPACE:-harbor}"
SECRET_NAME="${HARBOR_SECRET_NAME:-harbor-core}"
SECRET_KEY="${HARBOR_ADMIN_PASSWORD_KEY:-HARBOR_ADMIN_PASSWORD}"
ADMIN_USER="${HARBOR_ADMIN_USER:-admin}"

if ! command -v kubectl >/dev/null 2>&1; then
    echo "Error: kubectl not found" >&2
    exit 1
fi

if [ -z "${REGISTRY_HOST}" ]; then
    REGISTRY_HOST=$(kubectl -n "${SECRET_NS}" get ingress \
        -o jsonpath='{.items[0].spec.rules[0].host}' 2>/dev/null || true)
fi
if [ -z "${REGISTRY_HOST}" ]; then
    echo "Error: no registry host given and none found from the harbor ingress" >&2
    echo "  in namespace ${SECRET_NS}. Pass it explicitly: $0 <registry-host>" >&2
    exit 1
fi

PASS_B64=$(kubectl get secret "${SECRET_NAME}" -n "${SECRET_NS}" -o jsonpath="{.data.${SECRET_KEY}}" 2>/dev/null || true)
if [ -z "${PASS_B64}" ]; then
    echo "Error: could not read ${SECRET_KEY} from secret ${SECRET_NAME}/${SECRET_NS}" >&2
    exit 1
fi

PASS=$(echo "${PASS_B64}" | base64 -d)
printf '%s' "${PASS}" | docker login "${REGISTRY_HOST}" -u "${ADMIN_USER}" --password-stdin
echo "Docker login succeeded for ${REGISTRY_HOST} as ${ADMIN_USER}"
