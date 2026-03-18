{{/*
OpenSearch Cluster Helper Templates
*/}}

{{- define "opensearch.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{- define "opensearch.storageClass" -}}
{{- .Values.global.storageClass -}}
{{- end -}}

{{- define "opensearch.labels" -}}
app.kubernetes.io/name: opensearch
app.kubernetes.io/instance: {{ .Values.opensearch.name }}
app.kubernetes.io/component: log-storage
app.kubernetes.io/managed-by: Helm
{{- end -}}

{{- define "opensearch.adminSecretName" -}}
{{- if .Values.opensearch.adminSecretName -}}
{{- .Values.opensearch.adminSecretName -}}
{{- else -}}
{{- .Values.opensearch.name }}-admin-password
{{- end -}}
{{- end -}}
