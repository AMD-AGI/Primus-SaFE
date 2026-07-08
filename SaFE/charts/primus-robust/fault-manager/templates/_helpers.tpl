{{/*
Expand the name of the chart.
*/}}
{{- define "fault-manager.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "fault-manager.fullname" -}}
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
{{- define "fault-manager.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "fault-manager.labels" -}}
helm.sh/chart: {{ include "fault-manager.chart" . }}
{{ include "fault-manager.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "fault-manager.selectorLabels" -}}
app.kubernetes.io/name: {{ include "fault-manager.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: {{ include "fault-manager.name" . }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "fault-manager.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "fault-manager.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

