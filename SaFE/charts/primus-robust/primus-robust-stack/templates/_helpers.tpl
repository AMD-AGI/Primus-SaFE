{{- define "primus-robust.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "primus-robust.namespace" -}}
{{- default .Release.Namespace .Values.global.namespace }}
{{- end }}
