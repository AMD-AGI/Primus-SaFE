{{/*
Primus Lens Operators Helper Templates
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "primus-lens-operators.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "primus-lens-operators.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- "primus-lens" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "primus-lens-operators.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "primus-lens-operators.labels" -}}
helm.sh/chart: {{ include "primus-lens-operators.chart" . }}
{{ include "primus-lens-operators.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "primus-lens-operators.selectorLabels" -}}
app.kubernetes.io/name: {{ include "primus-lens-operators.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Get the namespace
*/}}
{{- define "primus-lens-operators.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

