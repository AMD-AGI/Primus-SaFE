{{/*
VictoriaMetrics Cluster Helper Templates
*/}}

{{- define "victoriametrics.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{- define "victoriametrics.storageClass" -}}
{{- .Values.global.storageClass -}}
{{- end -}}

{{- define "victoriametrics.labels" -}}
app.kubernetes.io/name: vmcluster
app.kubernetes.io/instance: {{ .Values.victoriametrics.name }}
app.kubernetes.io/component: metrics-storage
app.kubernetes.io/managed-by: Helm
{{- end -}}
