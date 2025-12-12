{{/*
Primus Lens Init Helper Templates
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "primus-lens-init.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "primus-lens-init.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- "primus-lens" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "primus-lens-init.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "primus-lens-init.labels" -}}
helm.sh/chart: {{ include "primus-lens-init.chart" . }}
{{ include "primus-lens-init.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "primus-lens-init.selectorLabels" -}}
app.kubernetes.io/name: {{ include "primus-lens-init.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Get the namespace
*/}}
{{- define "primus-lens-init.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{/*
Generate PostgreSQL host
*/}}
{{- define "primus-lens-init.postgresHost" -}}
primus-lens-ha.{{ include "primus-lens-init.namespace" . }}.svc.cluster.local
{{- end -}}

