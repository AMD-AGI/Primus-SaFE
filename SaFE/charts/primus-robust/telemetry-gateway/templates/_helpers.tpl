{{/*
Expand the name of the chart.
*/}}
{{- define "telemetry-gateway.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "telemetry-gateway.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "telemetry-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "telemetry-gateway.labels" -}}
helm.sh/chart: {{ include "telemetry-gateway.chart" . }}
{{ include "telemetry-gateway.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "telemetry-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "telemetry-gateway.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: {{ include "telemetry-gateway.name" . }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "telemetry-gateway.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "telemetry-gateway.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
