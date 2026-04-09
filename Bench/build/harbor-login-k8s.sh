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
# Default REGISTRY_HOST: harbor.oci-slc.primus-safe.amd.com
#

set -euo pipefail

REGISTRY_HOST="${1:-harbor.oci-slc.primus-safe.amd.com}"
SECRET_NS="${HARBOR_SECRET_NAMESPACE:-harbor}"
SECRET_NAME="${HARBOR_SECRET_NAME:-harbor-core}"
SECRET_KEY="${HARBOR_ADMIN_PASSWORD_KEY:-HARBOR_ADMIN_PASSWORD}"
ADMIN_USER="${HARBOR_ADMIN_USER:-admin}"

if ! command -v kubectl >/dev/null 2>&1; then
    echo "Error: kubectl not found" >&2
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
