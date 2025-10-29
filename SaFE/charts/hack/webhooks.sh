#!/usr/bin/env bash

#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

#ca_pem_b64=`cat ./build/webhooks/cert/ca.crt |base64`
output_file="primus-safe/templates/webhooks/manifests.yaml"

sed -e "s/metadata:/metadata:\n  labels:\n    app.kubernetes.io\/managed-by: Helm\n  annotations:\n    meta.helm.sh\/release-name: {{ .Release.Name }}\n    meta.helm.sh\/release-namespace: {{ .Release.Namespace }}\n    cert-manager.io\/inject-ca-from: {{ .Release.Namespace }}\/{{ .Release.Name }}-cert/g" ${output_file} > .tmp && mv .tmp ${output_file}
sed -e "s/namespace: system/namespace: {{ .Release.Namespace }}/g" ${output_file} > .tmp && mv .tmp ${output_file}
sed -e "s/mutating-webhook-configuration/primus-safe-webhook-mutate/g"  ${output_file} > .tmp && mv .tmp ${output_file}
sed -e "s/validating-webhook-configuration/primus-safe-webhook-validate/g" ${output_file} > .tmp && mv .tmp ${output_file}
sed -e "s/webhook-service/{{ .Release.Name }}-webhooks/g" ${output_file} > .tmp && mv .tmp ${output_file}