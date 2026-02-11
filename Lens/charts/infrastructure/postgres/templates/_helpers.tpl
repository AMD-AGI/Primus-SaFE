{{/*
PostgreSQL Cluster Helper Templates
*/}}

{{- define "postgres.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{- define "postgres.storageClass" -}}
{{- .Values.global.storageClass -}}
{{- end -}}

{{- define "postgres.labels" -}}
app.kubernetes.io/name: postgres
app.kubernetes.io/instance: {{ .Values.postgres.name }}
app.kubernetes.io/component: database
app.kubernetes.io/managed-by: Helm
{{- end -}}
