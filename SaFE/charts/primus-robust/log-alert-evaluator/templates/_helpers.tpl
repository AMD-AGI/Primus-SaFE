{{- define "log-alert-evaluator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "log-alert-evaluator.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "log-alert-evaluator.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{ include "log-alert-evaluator.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "log-alert-evaluator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "log-alert-evaluator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "log-alert-evaluator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "log-alert-evaluator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
