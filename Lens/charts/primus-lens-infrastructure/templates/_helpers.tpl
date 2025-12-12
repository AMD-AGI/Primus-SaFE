{{/*
Primus Lens Infrastructure Helper Templates
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "primus-lens-infra.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "primus-lens-infra.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- "primus-lens" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "primus-lens-infra.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "primus-lens-infra.labels" -}}
helm.sh/chart: {{ include "primus-lens-infra.chart" . }}
{{ include "primus-lens-infra.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "primus-lens-infra.selectorLabels" -}}
app.kubernetes.io/name: {{ include "primus-lens-infra.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Get the namespace
*/}}
{{- define "primus-lens-infra.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{/*
Get the storage class name
*/}}
{{- define "primus-lens-infra.storageClass" -}}
{{- .Values.global.storageClass -}}
{{- end -}}

{{/*
Get the access mode
*/}}
{{- define "primus-lens-infra.accessMode" -}}
{{- .Values.global.accessMode -}}
{{- end -}}

{{/*
Get the current profile configuration
*/}}
{{- define "primus-lens-infra.profileConfig" -}}
{{- $profile := .Values.profile -}}
{{- index .Values.profiles $profile | toYaml -}}
{{- end -}}

{{/*
Generate PostgreSQL host
*/}}
{{- define "primus-lens-infra.postgresHost" -}}
primus-lens-ha.{{ include "primus-lens-infra.namespace" . }}.svc.cluster.local
{{- end -}}

{{/*
Generate OpenSearch endpoint
*/}}
{{- define "primus-lens-infra.opensearchEndpoint" -}}
{{ .Values.opensearch.clusterName }}-nodes.{{ include "primus-lens-infra.namespace" . }}.svc.cluster.local:9200
{{- end -}}

{{/*
Generate VictoriaMetrics vmselect endpoint
*/}}
{{- define "primus-lens-infra.vmSelectEndpoint" -}}
vmselect-primus-lens-vmcluster.{{ include "primus-lens-infra.namespace" . }}.svc.cluster.local:8481
{{- end -}}

{{/*
Convert memory string to Mi (for JVM heap calculation)
*/}}
{{- define "primus-lens-infra.memoryToMi" -}}
{{- $mem := . -}}
{{- if hasSuffix "Gi" $mem -}}
{{- $val := trimSuffix "Gi" $mem | int -}}
{{- mul $val 1024 -}}
{{- else if hasSuffix "Mi" $mem -}}
{{- trimSuffix "Mi" $mem | int -}}
{{- else -}}
{{- 1024 -}}
{{- end -}}
{{- end -}}

