#!/usr/bin/env bash
#ca_pem_b64=`cat ./build/webhooks/cert/ca.crt |base64`
output_file="primus-safe/templates/webhooks/manifests.yaml"

sed -e "s/metadata:/metadata:\n  annotations:\n     cert-manager.io\/inject-ca-from: {{ .Release.Namespace }}\/webhook-cert/g" ${output_file} > .tmp && mv .tmp ${output_file}
sed -e "s/namespace: system/namespace: {{ .Release.Namespace }}/g" ${output_file} > .tmp && mv .tmp ${output_file}
sed -e "s/mutating-webhook-configuration/primus-safe-webhook-mutate/g"  ${output_file} > .tmp && mv .tmp ${output_file}
sed -e "s/validating-webhook-configuration/primus-safe-webhook-validate/g" ${output_file} > .tmp && mv .tmp ${output_file}
