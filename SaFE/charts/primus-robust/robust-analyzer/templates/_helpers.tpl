{{/*
Expand the name of the chart.
*/}}
{{- define "robust-analyzer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "robust-analyzer.fullname" -}}
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
{{- define "robust-analyzer.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "robust-analyzer.labels" -}}
helm.sh/chart: {{ include "robust-analyzer.chart" . }}
{{ include "robust-analyzer.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "robust-analyzer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "robust-analyzer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: {{ include "robust-analyzer.name" . }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "robust-analyzer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "robust-analyzer.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
