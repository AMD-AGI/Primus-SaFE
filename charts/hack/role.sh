#!/usr/bin/env bash
#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
role_file="primus-safe/templates/rbac/role.yaml"

if [ -f ${role_file} ]; then
  sed -i '/^[[:space:]]*name: primus-safe$/a\  labels:\n    app.kubernetes.io/managed-by: Helm\n  annotations:\n    meta.helm.sh\/release-name: {{ .Release.Name }}\n    meta.helm.sh/release-namespace: {{ .Release.Namespace }}' ${role_file}
  cat config/role_patch.txt >> ${role_file}
fi