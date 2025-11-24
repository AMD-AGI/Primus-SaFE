{{/*
Expand the name of the chart.
*/}}
{{- define "primus-lens.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "primus-lens.fullname" -}}
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
{{- define "primus-lens.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

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
primus-lens.ai/deployment-mode: {{ .Values.deploymentMode }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "primus-lens.selectorLabels" -}}
app.kubernetes.io/name: {{ include "primus-lens.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
判断是否应该启用管理集群组件
*/}}
{{- define "primus-lens.management.enabled" -}}
{{- if eq .Values.management.enabled true }}
true
{{- else if eq .Values.management.enabled "auto" }}
  {{- if or (eq .Values.deploymentMode "management") (eq .Values.deploymentMode "all-in-one") }}
true
  {{- else }}
false
  {{- end }}
{{- else }}
false
{{- end }}
{{- end }}

{{/*
判断是否应该启用数据集群组件
*/}}
{{- define "primus-lens.data.enabled" -}}
{{- if eq .Values.data.enabled true }}
true
{{- else if eq .Values.data.enabled "auto" }}
  {{- if or (eq .Values.deploymentMode "data") (eq .Values.deploymentMode "all-in-one") }}
true
  {{- else }}
false
  {{- end }}
{{- else }}
false
{{- end }}
{{- end }}

{{/*
判断是否应该部署中间件
*/}}
{{- define "primus-lens.middleware.enabled" -}}
{{- if eq .Values.middleware.enabled true }}
true
{{- else if eq .Values.middleware.enabled "auto" }}
  {{- if eq .Values.deploymentMode "all-in-one" }}
true
  {{- else if eq .Values.deploymentMode "management" }}
true
  {{- else }}
    {{- /* data 模式下，如果没有配置远程中间件，也需要部署 */ -}}
    {{- if not .Values.middleware.remote.postgresql.host }}
true
    {{- else }}
false
    {{- end }}
  {{- end }}
{{- else }}
false
{{- end }}
{{- end }}

{{/*
根据 profile 获取 PostgreSQL 内存配置
*/}}
{{- define "primus-lens.postgresql.memory" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.postgresql.memory }}
{{- else if eq $profile "large" }}
{{ $profiles.large.postgresql.memory }}
{{- else }}
{{ $profiles.normal.postgresql.memory }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 PostgreSQL CPU 配置
*/}}
{{- define "primus-lens.postgresql.cpu" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.postgresql.cpu }}
{{- else if eq $profile "large" }}
{{ $profiles.large.postgresql.cpu }}
{{- else }}
{{ $profiles.normal.postgresql.cpu }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 PostgreSQL 数据存储大小
*/}}
{{- define "primus-lens.postgresql.dataSize" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.postgresql.data }}
{{- else if eq $profile "large" }}
{{ $profiles.large.postgresql.data }}
{{- else }}
{{ $profiles.normal.postgresql.data }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 PostgreSQL 备份存储大小
*/}}
{{- define "primus-lens.postgresql.backupSize" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.postgresql.backup }}
{{- else if eq $profile "large" }}
{{ $profiles.large.postgresql.backup }}
{{- else }}
{{ $profiles.normal.postgresql.backup }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 PostgreSQL 副本数
*/}}
{{- define "primus-lens.postgresql.replicas" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.postgresql.replicas }}
{{- else if eq $profile "large" }}
{{ $profiles.large.postgresql.replicas }}
{{- else }}
{{ $profiles.normal.postgresql.replicas }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 OpenSearch 磁盘大小
*/}}
{{- define "primus-lens.opensearch.diskSize" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.opensearch.disk }}
{{- else if eq $profile "large" }}
{{ $profiles.large.opensearch.disk }}
{{- else }}
{{ $profiles.normal.opensearch.disk }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 OpenSearch 内存配置
*/}}
{{- define "primus-lens.opensearch.memory" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.opensearch.memory }}
{{- else if eq $profile "large" }}
{{ $profiles.large.opensearch.memory }}
{{- else }}
{{ $profiles.normal.opensearch.memory }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 OpenSearch CPU 配置
*/}}
{{- define "primus-lens.opensearch.cpu" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.opensearch.cpu }}
{{- else if eq $profile "large" }}
{{ $profiles.large.opensearch.cpu }}
{{- else }}
{{ $profiles.normal.opensearch.cpu }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 VictoriaMetrics VMStorage 大小
*/}}
{{- define "primus-lens.victoriametrics.storageSize" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.victoriametrics.vmstorage.size }}
{{- else if eq $profile "large" }}
{{ $profiles.large.victoriametrics.vmstorage.size }}
{{- else }}
{{ $profiles.normal.victoriametrics.vmstorage.size }}
{{- end }}
{{- end }}

{{/*
根据 profile 获取 VictoriaMetrics VMStorage 副本数
*/}}
{{- define "primus-lens.victoriametrics.storageReplicas" -}}
{{- $profile := .Values.global.profile }}
{{- $profiles := .Values.profiles }}
{{- if eq $profile "minimal" }}
{{ $profiles.minimal.victoriametrics.vmstorage.replicas }}
{{- else if eq $profile "large" }}
{{ $profiles.large.victoriametrics.vmstorage.replicas }}
{{- else }}
{{ $profiles.normal.victoriametrics.vmstorage.replicas }}
{{- end }}
{{- end }}

{{/*
构建完整的镜像地址
用法: {{ include "primus-lens.image" (dict "component" "api" "root" .) }}
*/}}
{{- define "primus-lens.image" -}}
{{- $registry := .root.Values.global.imageRegistry }}
{{- $component := .component }}
{{- $componentConfig := index .root.Values.management $component }}
{{- if not $componentConfig }}
  {{- $componentConfig = index .root.Values.data $component }}
{{- end }}
{{- if not $componentConfig }}
  {{- $componentConfig = index .root.Values.middleware $component }}
{{- end }}
{{- $repository := $componentConfig.image.repository }}
{{- $tag := $componentConfig.image.tag | default .root.Chart.AppVersion }}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}

{{/*
生成 ServiceAccount 名称
*/}}
{{- define "primus-lens.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "primus-lens.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
生成数据库连接字符串
*/}}
{{- define "primus-lens.databaseURL" -}}
{{- if eq (include "primus-lens.middleware.enabled" .) "true" }}
{{- /* 本地中间件 */ -}}
postgresql://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
{{- else }}
{{- /* 远程中间件 */ -}}
{{- with .Values.middleware.remote.postgresql }}
postgresql://{{ .user }}:$(DB_PASSWORD)@{{ .host }}:{{ .port }}/{{ .database }}?sslmode=disable
{{- end }}
{{- end }}
{{- end }}

{{/*
生成 OpenSearch 地址
*/}}
{{- define "primus-lens.opensearchURL" -}}
{{- if eq (include "primus-lens.middleware.enabled" .) "true" }}
http://primus-lens-opensearch:9200
{{- else }}
http://{{ .Values.middleware.remote.opensearch.host }}:{{ .Values.middleware.remote.opensearch.port }}
{{- end }}
{{- end }}

{{/*
生成 VictoriaMetrics 地址
*/}}
{{- define "primus-lens.victoriametricsURL" -}}
{{- if eq (include "primus-lens.middleware.enabled" .) "true" }}
http://primus-lens-vm-vmselect:8481/select/0/prometheus
{{- else }}
http://{{ .Values.middleware.remote.victoriametrics.host }}:{{ .Values.middleware.remote.victoriametrics.port }}
{{- end }}
{{- end }}

{{/*
生成 Otel Collector 地址
*/}}
{{- define "primus-lens.otelCollectorEndpoint" -}}
{{- if eq (include "primus-lens.middleware.enabled" .) "true" }}
primus-lens-otel-collector:4317
{{- else }}
{{ .Values.middleware.remote.otelCollector.endpoint }}
{{- end }}
{{- end }}

