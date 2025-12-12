{{/*
Primus Lens Data Plane Helper Templates
This file contains reusable template functions for the Primus Lens Data Plane chart.
*/}}

{{/*
Expand the name of the chart.
*/}}
{{- define "primus-lens.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
Always use "primus-lens" as the base name for consistency with infrastructure resources.
*/}}
{{- define "primus-lens.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- "primus-lens" -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "primus-lens.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "primus-lens.labels" -}}
helm.sh/chart: {{ include "primus-lens.chart" . }}
{{ include "primus-lens.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "primus-lens.selectorLabels" -}}
app.kubernetes.io/name: {{ include "primus-lens.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Get the namespace
*/}}
{{- define "primus-lens.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{/*
Get the storage class name
*/}}
{{- define "primus-lens.storageClass" -}}
{{- .Values.global.storageClass -}}
{{- end -}}

{{/*
Get the access mode
*/}}
{{- define "primus-lens.accessMode" -}}
{{- .Values.global.accessMode -}}
{{- end -}}

{{/*
Get the current profile configuration
Returns the entire profile object (opensearch, postgres, victoriametrics)
*/}}
{{- define "primus-lens.profileConfig" -}}
{{- $profile := .Values.profile -}}
{{- index .Values.profiles $profile | toYaml -}}
{{- end -}}

{{/*
Generate image pull secrets
*/}}
{{- define "primus-lens.imagePullSecrets" -}}
{{- if .Values.global.imagePullSecrets }}
imagePullSecrets:
{{- range .Values.global.imagePullSecrets }}
  - name: {{ .name }}
{{- end }}
{{- end }}
{{- end -}}

{{/*
Generate Docker config JSON for image pull secret
*/}}
{{- define "primus-lens.dockerConfigJson" -}}
{{- $registry := .credentials.registry -}}
{{- $username := .credentials.username -}}
{{- $password := .credentials.password -}}
{{- if and $username $password -}}
{{- $auth := printf "%s:%s" $username $password | b64enc -}}
{{- $config := dict "auths" (dict $registry (dict "username" $username "password" $password "auth" $auth)) -}}
{{- $config | toJson -}}
{{- else -}}
{}
{{- end -}}
{{- end -}}

{{/*
Check if ingress is enabled
*/}}
{{- define "primus-lens.useIngress" -}}
{{- eq .Values.global.accessType "ingress" -}}
{{- end -}}

{{/*
Generate Grafana root URL based on access type
*/}}
{{- define "primus-lens.grafanaRootUrl" -}}
{{- if eq .Values.global.accessType "ssh-tunnel" -}}
http://127.0.0.1:30182/grafana
{{- else if eq .Values.global.accessType "ingress" -}}
https://{{ .Values.global.clusterName }}.{{ .Values.global.domain }}/grafana
{{- end -}}
{{- end -}}

{{/*
Generate Grafana domain
*/}}
{{- define "primus-lens.grafanaDomain" -}}
{{- if eq .Values.global.accessType "ingress" -}}
{{ .Values.global.clusterName }}.{{ .Values.global.domain }}
{{- else -}}
""
{{- end -}}
{{- end -}}

{{/*
Get image registry
*/}}
{{- define "primus-lens.imageRegistry" -}}
{{- .Values.global.imageRegistry -}}
{{- end -}}

{{/*
Build full image name
Usage: include "primus-lens.image" (dict "registry" .Values.global.imageRegistry "image" .Values.apps.api.image)
*/}}
{{- define "primus-lens.image" -}}
{{- if .registry -}}
{{ .registry }}/{{ .image }}
{{- else -}}
{{ .image }}
{{- end -}}
{{- end -}}

{{/*
Generate service account name
*/}}
{{- define "primus-lens.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{ default (include "primus-lens.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
{{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Generate PostgreSQL connection string
*/}}
{{- define "primus-lens.postgresHost" -}}
primus-lens-ha.{{ include "primus-lens.namespace" . }}.svc.cluster.local
{{- end -}}

{{/*
Generate OpenSearch endpoint
*/}}
{{- define "primus-lens.opensearchEndpoint" -}}
{{ .Values.opensearch.clusterName }}-nodes.{{ include "primus-lens.namespace" . }}.svc.cluster.local:9200
{{- end -}}

{{/*
Generate VictoriaMetrics vmselect endpoint
*/}}
{{- define "primus-lens.vmSelectEndpoint" -}}
vmselect-primus-lens-vmcluster.{{ include "primus-lens.namespace" . }}.svc.cluster.local:8481
{{- end -}}

{{/*
Check if a component is enabled
Usage: include "primus-lens.isEnabled" (dict "component" .Values.apps.api "global" .Values.global)
*/}}
{{- define "primus-lens.isEnabled" -}}
{{- and .component.enabled (not (hasKey .global "disabled")) -}}
{{- end -}}

{{/*
Generate common environment variables for apps
*/}}
{{- define "primus-lens.commonEnv" -}}
- name: CLUSTER_NAME
  value: {{ .Values.global.clusterName }}
- name: NAMESPACE
  value: {{ include "primus-lens.namespace" . }}
- name: STORAGE_CLASS
  value: {{ include "primus-lens.storageClass" . }}
{{- end -}}

{{/*
Generate database environment variables
*/}}
{{- define "primus-lens.dbEnv" -}}
- name: DB_HOST
  value: {{ include "primus-lens.postgresHost" . }}
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
Hook weight for different phases
Updated deployment order:
- Phase 0: pre-install hooks (-100 to -90)
- Phase 1: Operators (sub-charts auto-deployed)
- Phase 2: pre-install hook (0) - wait for operators
- Phase 3: normal resources - OpenSearch CR, VMCluster CR, PostgreSQL CR
- Phase 4: post-install hook (5) - wait for infrastructure
- Phase 5: post-install hook (10) - postgres init
- Phase 6: normal resources - Apps (via primus-lens-apps sub-chart)
- Phase 7: post-install hook (100) - FluentBit & VMAgent (depends on telemetry-processor)
*/}}
{{- define "primus-lens.hookWeight.namespace" -}}-100{{- end -}}
{{- define "primus-lens.hookWeight.secrets" -}}-90{{- end -}}
{{- define "primus-lens.hookWeight.crds" -}}-80{{- end -}}
{{- define "primus-lens.hookWeight.precheck" -}}-70{{- end -}}
{{- define "primus-lens.hookWeight.waitOperators" -}}0{{- end -}}
{{- define "primus-lens.hookWeight.waitInfrastructure" -}}5{{- end -}}
{{- define "primus-lens.hookWeight.postgresInit" -}}10{{- end -}}
{{- define "primus-lens.hookWeight.opensearchInit" -}}20{{- end -}}
{{- define "primus-lens.hookWeight.monitoring" -}}100{{- end -}}
{{- define "primus-lens.hookWeight.grafanaDashboards" -}}110{{- end -}}
{{- define "primus-lens.hookWeight.validation" -}}200{{- end -}}

