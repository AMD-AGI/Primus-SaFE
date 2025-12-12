{{/*
Primus Lens Apps Helper Templates
This file contains reusable template functions for the Primus Lens Apps chart.
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "lens.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
For apps chart, we use "primus-lens" as the base name to maintain compatibility
with the full-stack deployment.
*/}}
{{- define "lens.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := "primus-lens" -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s" $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "lens.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "lens.labels" -}}
helm.sh/chart: {{ include "lens.chart" . }}
{{ include "lens.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "lens.selectorLabels" -}}
app.kubernetes.io/name: primus-lens
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Get the namespace
*/}}
{{- define "lens.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{/*
Generate service account name
*/}}
{{- define "lens.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{ default (include "lens.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
{{ default "primus-lens-app" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Generate image pull secrets
*/}}
{{- define "lens.imagePullSecrets" -}}
{{- if .Values.global.imageRegistry.pullSecret }}
imagePullSecrets:
  - name: {{ .Values.global.imageRegistry.pullSecret }}
{{- end }}
{{- end -}}

{{/*
Build full image name
Usage: include "lens.image" (dict "registry" .Values.global.imageRegistry.url "image" .Values.apps.api.image)
*/}}
{{- define "lens.image" -}}
{{- if .registry -}}
{{ .registry }}/{{ .image }}
{{- else -}}
{{ .image }}
{{- end -}}
{{- end -}}

{{/*
Generate common environment variables for apps
*/}}
{{- define "lens.commonEnv" -}}
- name: CLUSTER_NAME
  value: {{ .Values.global.clusterName }}
- name: NAMESPACE
  value: {{ include "lens.namespace" . }}
{{- end -}}

{{/*
Generate database environment variables
*/}}
{{- define "lens.dbEnv" -}}
- name: DB_HOST
  value: primus-lens-ha.{{ include "lens.namespace" . }}.svc.cluster.local
- name: DB_PORT
  value: "5432"
- name: DB_USER
  value: primus-lens
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: primus-lens-pguser-primus-lens
      key: password
- name: DB_NAME
  value: primus_lens
{{- end -}}

{{/*
Generate PostgreSQL host
*/}}
{{- define "lens.postgresHost" -}}
primus-lens-ha.{{ include "lens.namespace" . }}.svc.cluster.local
{{- end -}}

{{/*
InitContainer to wait for PostgreSQL to be ready
Use this in apps that require database connectivity
*/}}
{{- define "lens.waitForPostgres" -}}
- name: wait-postgres
  image: postgres:16
  imagePullPolicy: IfNotPresent
  command:
  - /bin/bash
  - -c
  - |
    set -e
    echo "Waiting for PostgreSQL to be ready..."
    POSTGRES_HOST="{{ include "lens.postgresHost" . }}"
    MAX_ATTEMPTS=120
    ATTEMPT=0
    
    until pg_isready -h "$POSTGRES_HOST" -p 5432 -U postgres 2>/dev/null; do
      ATTEMPT=$((ATTEMPT + 1))
      if [ $ATTEMPT -ge $MAX_ATTEMPTS ]; then
        echo "PostgreSQL is not ready after $MAX_ATTEMPTS attempts"
        exit 1
      fi
      echo "[$ATTEMPT/$MAX_ATTEMPTS] PostgreSQL is not ready yet, waiting..."
      sleep 5
    done
    
    echo "PostgreSQL is ready!"
{{- end -}}

