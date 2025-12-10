# Primus Lens/SaFE 纯 Helm 部署架构设计文档

## 1. 概述

### 1.1 目标
将当前基于 Shell 脚本 + kubectl + helm 混合的部署方式重构为**纯 Helm Chart 方式**，实现一次性部署，无需手动执行脚本。

### 1.2 当前架构的问题

| 问题类型 | 当前实现 | 影响 |
|---------|---------|------|
| **交互式参数收集** | 脚本中使用 `read -rp` 收集参数 | 不支持自动化部署，CI/CD 集成困难 |
| **配置模板处理** | 使用 `envsubst` 和 `sed` 动态替换配置 | 逻辑分散在脚本中，难以维护和版本管理 |
| **部署顺序控制** | 脚本中使用 `for` 循环和 `sleep` 等待资源就绪 | 不可靠，可能因超时导致部署失败 |
| **依赖管理** | 手动 `git clone` 和 `helm repo add` | 依赖版本不受控，重复执行会产生临时文件 |
| **初始化任务** | 脚本中执行 `kubectl exec` 初始化数据库 | 与部署逻辑耦合，无法利用 K8s 原生重试机制 |
| **密钥管理** | 脚本中使用 `kubectl create secret` | 敏感信息处理分散，难以审计 |

### 1.3 目标架构优势

| 特性 | Helm 纯实现 | 优势 |
|------|-----------|------|
| **声明式配置** | 所有参数集中在 `values.yaml` | 支持 GitOps，易于审计和版本管理 |
| **模板化** | Helm 模板引擎处理所有配置 | 统一的模板语法，内置函数丰富 |
| **依赖管理** | Helm dependencies + subcharts | 版本锁定，自动下载和管理 |
| **部署编排** | Helm hooks + readiness probes | 利用 K8s 原生机制，更可靠 |
| **初始化作业** | Kubernetes Jobs with hooks | 自动重试，状态可追踪 |
| **一键部署** | `helm install` 一条命令 | 简化操作，支持回滚和升级 |

---

## 2. 整体架构设计

### 2.1 Chart 层级结构

```
primus-lens/                          # 父 Chart (Umbrella Chart)
├── Chart.yaml                        # Chart 元数据和依赖定义
├── values.yaml                       # 默认配置值
├── values-dev.yaml                   # 开发环境配置
├── values-prod.yaml                  # 生产环境配置
├── templates/                        # 主模板目录
│   ├── NOTES.txt                     # 部署后显示的提示信息
│   ├── _helpers.tpl                  # 通用模板函数
│   │
│   ├── 00-namespace.yaml             # 命名空间 (pre-install hook)
│   ├── 01-secrets/                   # 密钥资源
│   │   ├── image-pull-secret.yaml
│   │   ├── postgres-init-secret.yaml
│   │   └── tls-cert-secret.yaml
│   │
│   ├── 02-init-jobs/                 # 初始化作业 (pre-install hooks)
│   │   ├── wait-for-operators-job.yaml
│   │   ├── postgres-init-job.yaml
│   │   └── opensearch-init-job.yaml
│   │
│   ├── 03-apps/                      # 应用组件
│   │   ├── app-api.yaml
│   │   ├── app-telemetry-collector.yaml
│   │   ├── app-jobs.yaml
│   │   ├── app-node-exporter.yaml
│   │   ├── app-gpu-resource-exporter.yaml
│   │   ├── app-system-tuner.yaml
│   │   └── app-web.yaml
│   │
│   ├── 04-monitoring/                # 监控相关资源
│   │   ├── vmcluster.yaml
│   │   ├── vmagent.yaml
│   │   ├── vmscrape-basic-metrics.yaml
│   │   └── fluent-bit-config.yaml
│   │
│   ├── 05-database/                  # 数据库 CRs
│   │   └── pg-cr.yaml
│   │
│   ├── 06-storage/                   # 存储 CRs
│   │   └── opensearch-cr.yaml
│   │
│   ├── 07-grafana/                   # Grafana 相关
│   │   ├── grafana-cr.yaml
│   │   ├── datasource.yaml
│   │   ├── folders.yaml
│   │   └── dashboards/
│   │       ├── node-exporter.yaml
│   │       ├── node-rdma.yaml
│   │       ├── workload-metrics.yaml
│   │       └── ...
│   │
│   ├── 08-ingress/                   # 入口资源
│   │   ├── nginx-ingress.yaml
│   │   └── grafana-ingress.yaml
│   │
│   └── 99-post-install/              # 后置任务 (post-install hooks)
│       ├── validation-job.yaml
│       └── notification-job.yaml
│
├── charts/                           # 子 Charts (依赖)
│   ├── victoria-metrics-operator/   # 自动下载
│   ├── fluent-operator/              # 自动下载
│   ├── opensearch-operator/          # 自动下载
│   ├── postgres-operator/            # 自动下载
│   ├── grafana-operator/             # 自动下载
│   └── kube-state-metrics/           # 自动下载
│
└── crds/                             # 自定义资源定义 (可选)
    └── ...
```

### 2.2 Chart.yaml 依赖配置示例

```yaml
apiVersion: v2
name: primus-lens
description: Primus Lens - AI Training Platform Observability
version: 1.0.0
appVersion: "1.0"

dependencies:
  # VictoriaMetrics Operator
  - name: victoria-metrics-operator
    version: "0.35.2"
    repository: https://victoriametrics.github.io/helm-charts/
    condition: victoriametrics.enabled
    alias: vm-operator

  # Fluent Operator
  - name: fluent-operator
    version: "3.1.0"
    repository: https://fluent.github.io/helm-charts
    condition: logging.enabled

  # OpenSearch Operator
  - name: opensearch-operator
    version: "2.6.0"
    repository: https://opensearch-project.github.io/opensearch-k8s-operator/
    condition: opensearch.enabled

  # PostgreSQL Operator (Crunchy)
  - name: pgo
    version: "5.7.0"
    repository: oci://registry.developers.crunchydata.com/crunchydata
    condition: database.enabled

  # Grafana Operator
  - name: grafana-operator
    version: "5.15.0"
    repository: oci://ghcr.io/grafana/helm-charts
    condition: grafana.enabled

  # Kube State Metrics
  - name: kube-state-metrics
    version: "5.27.0"
    repository: https://prometheus-community.github.io/helm-charts
    condition: monitoring.kubeStateMetrics.enabled
```

---

## 3. 核心设计模式

### 3.1 参数配置管理

**设计原则**: 所有可配置项集中在 `values.yaml`，支持多环境覆盖

```yaml
# values.yaml (精简示例)
global:
  # 集群基本信息
  clusterName: "my-cluster"
  namespace: "primus-lens"
  
  # 存储配置
  storageClass: "local-path"
  accessMode: "ReadWriteOnce"  # ReadWriteMany 如果支持
  
  # 镜像仓库
  imageRegistry: "docker.io"
  imagePullSecrets:
    - name: primus-lens-image
    credentials:
      registry: "docker.io"
      username: ""  # 通过 --set 或环境变量传入
      password: ""  # 通过 --set 或环境变量传入
  
  # 访问方式
  accessType: "ssh-tunnel"  # 或 "ingress"
  domain: "lens-primus.ai"

# 资源配置 Profile
profile: "normal"  # minimal, normal, large

profiles:
  minimal:
    opensearch:
      diskSize: "30Gi"
      memory: "2Gi"
      cpu: "1000m"
    postgres:
      backupSize: "10Gi"
      dataSize: "20Gi"
      replicas: 1
    victoriametrics:
      vmagent:
        cpu: "500m"
        memory: "512Mi"
      vmstorage:
        replicas: 1
        cpu: "1000m"
        memory: "2Gi"
        size: "30Gi"
      vmselect:
        replicas: 1
        cpu: "500m"
        memory: "1Gi"
      vminsert:
        replicas: 1
        cpu: "500m"
        memory: "1Gi"
  
  normal:
    opensearch:
      diskSize: "50Gi"
      memory: "4Gi"
      cpu: "2000m"
    postgres:
      backupSize: "20Gi"
      dataSize: "50Gi"
      replicas: 2
    victoriametrics:
      vmagent:
        cpu: "1000m"
        memory: "1Gi"
      vmstorage:
        replicas: 2
        cpu: "2000m"
        memory: "4Gi"
        size: "50Gi"
      vmselect:
        replicas: 2
        cpu: "1000m"
        memory: "2Gi"
      vminsert:
        replicas: 2
        cpu: "1000m"
        memory: "2Gi"
  
  large:
    opensearch:
      diskSize: "100Gi"
      memory: "8Gi"
      cpu: "4000m"
    postgres:
      backupSize: "50Gi"
      dataSize: "100Gi"
      replicas: 3
    victoriametrics:
      vmagent:
        cpu: "2000m"
        memory: "2Gi"
      vmstorage:
        replicas: 3
        cpu: "4000m"
        memory: "8Gi"
        size: "100Gi"
      vmselect:
        replicas: 3
        cpu: "2000m"
        memory: "4Gi"
      vminsert:
        replicas: 3
        cpu: "2000m"
        memory: "4Gi"

# 应用组件配置
apps:
  api:
    enabled: true
    image: "primuslens/api:v1.0.0"
    replicas: 2
  
  telemetryCollector:
    enabled: true
    image: "primuslens/telemetry-collector:v1.0.0"
    replicas: 2
  
  # ... 其他组件

# Operator 子 Chart 配置透传
victoria-metrics-operator:
  enabled: true
  operator:
    resources:
      limits:
        cpu: 200m
        memory: 150Mi

fluent-operator:
  enabled: true
  # ... 配置项

opensearch-operator:
  enabled: true
  # ... 配置项

pgo:
  enabled: true
  # ... 配置项

grafana-operator:
  enabled: true
  # ... 配置项
```

### 3.2 模板化配置处理

**核心技术**: 使用 Helm 模板函数替代 envsubst

```yaml
# templates/_helpers.tpl
{{/*
获取当前 Profile 的配置
*/}}
{{- define "primus-lens.profileConfig" -}}
{{- $profile := .Values.profile -}}
{{- index .Values.profiles $profile -}}
{{- end -}}

{{/*
生成存储类名称
*/}}
{{- define "primus-lens.storageClass" -}}
{{- .Values.global.storageClass -}}
{{- end -}}

{{/*
生成命名空间
*/}}
{{- define "primus-lens.namespace" -}}
{{- .Values.global.namespace -}}
{{- end -}}

{{/*
生成镜像拉取密钥引用
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
判断是否启用 Ingress
*/}}
{{- define "primus-lens.useIngress" -}}
{{- eq .Values.global.accessType "ingress" -}}
{{- end -}}

{{/*
生成 Grafana Root URL
*/}}
{{- define "primus-lens.grafanaRootUrl" -}}
{{- if eq .Values.global.accessType "ssh-tunnel" -}}
http://127.0.0.1:30182/grafana
{{- else if eq .Values.global.accessType "ingress" -}}
https://{{ .Values.global.clusterName }}.{{ .Values.global.domain }}/grafana
{{- end -}}
{{- end -}}
```

**应用示例**:

```yaml
# templates/03-apps/app-api.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-api
  namespace: {{ include "primus-lens.namespace" . }}
  labels:
    app: primus-lens-api
spec:
  replicas: {{ .Values.apps.api.replicas }}
  selector:
    matchLabels:
      app: primus-lens-api
  template:
    metadata:
      labels:
        app: primus-lens-api
    spec:
      {{- include "primus-lens.imagePullSecrets" . | nindent 6 }}
      containers:
      - name: api
        image: {{ .Values.global.imageRegistry }}/{{ .Values.apps.api.image }}
        env:
        - name: CLUSTER_NAME
          value: {{ .Values.global.clusterName }}
        - name: STORAGE_CLASS
          value: {{ include "primus-lens.storageClass" . }}
        - name: PG_PASSWORD
          valueFrom:
            secretKeyRef:
              name: primus-lens-pguser-primus-lens
              key: password
        # ... 其他配置
```

### 3.3 部署顺序控制

**核心技术**: Helm Hooks + Kubernetes Readiness Probes

#### 3.3.1 Helm Hooks 类型和用途

| Hook 类型 | 执行时机 | 用途示例 |
|-----------|---------|---------|
| **pre-install** | helm install 之前 | 创建命名空间、验证前置条件 |
| **post-install** | helm install 之后，所有资源创建完成 | 执行初始化脚本、发送通知 |
| **pre-upgrade** | helm upgrade 之前 | 备份数据、验证升级条件 |
| **post-upgrade** | helm upgrade 之后 | 数据迁移、清理旧资源 |
| **pre-delete** | helm uninstall 之前 | 备份数据、清理外部资源 |
| **post-delete** | helm uninstall 之后 | 清理持久化数据 (可选) |

#### 3.3.2 Hook 权重 (Weight)

使用 `helm.sh/hook-weight` 注解控制同类 Hook 的执行顺序（数值越小越先执行）

**部署阶段划分**:

```
Phase 0: 前置准备 (pre-install hooks, weight: -100 到 -1)
  ├── Weight -100: 命名空间创建
  ├── Weight -90:  密钥创建 (镜像拉取密钥等)
  ├── Weight -80:  CRD 安装 (如果未由子 Chart 处理)
  └── Weight -70:  验证前置条件 Job

Phase 1: Operator 部署 (子 Chart 自动处理)
  ├── victoria-metrics-operator
  ├── fluent-operator
  ├── opensearch-operator
  ├── postgres-operator (pgo)
  └── grafana-operator

Phase 2: 等待 Operators 就绪 (pre-install hook, weight: 0)
  └── Weight 0: wait-for-operators Job

Phase 3: 基础设施部署 (正常资源)
  ├── 数据库 CR (postgres cluster)
  ├── 存储 CR (opensearch cluster)
  ├── 监控 CR (vmcluster, vmagent)
  └── 日志 CR (fluentbit config)

Phase 4: 初始化作业 (post-install hooks, weight: 1-100)
  ├── Weight 10: 数据库初始化 Job
  ├── Weight 20: OpenSearch 初始化 Job
  └── Weight 30: 导入 Grafana Dashboards Job

Phase 5: 应用部署 (post-install hooks, weight: 100+)
  ├── Weight 100: 应用组件 (api, collector, jobs, exporters, web)
  ├── Weight 200: Ingress/Service
  └── Weight 300: 验证和通知 Job
```

#### 3.3.3 等待 Operators 就绪示例

```yaml
# templates/02-init-jobs/wait-for-operators-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: primus-lens-wait-operators
  namespace: {{ include "primus-lens.namespace" . }}
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "0"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 30  # 最多重试 30 次
  template:
    metadata:
      name: wait-operators
    spec:
      restartPolicy: OnFailure
      serviceAccountName: primus-lens-installer
      containers:
      - name: wait
        image: bitnami/kubectl:latest
        command:
        - /bin/bash
        - -c
        - |
          set -e
          echo "⏳ Waiting for operators to be ready..."
          
          # 等待 VictoriaMetrics Operator
          {{- if .Values.vm-operator.enabled }}
          kubectl wait --for=condition=ready pod \
            -l app.kubernetes.io/name=victoria-metrics-operator \
            -n {{ include "primus-lens.namespace" . }} \
            --timeout=300s
          echo "✅ VictoriaMetrics Operator is ready"
          {{- end }}
          
          # 等待 Fluent Operator
          {{- if index .Values "fluent-operator" "enabled" }}
          kubectl wait --for=condition=ready pod \
            -l app.kubernetes.io/name=fluent-operator \
            -n {{ include "primus-lens.namespace" . }} \
            --timeout=300s
          echo "✅ Fluent Operator is ready"
          {{- end }}
          
          # 等待 OpenSearch Operator
          {{- if index .Values "opensearch-operator" "enabled" }}
          kubectl wait --for=condition=ready pod \
            -l app.kubernetes.io/name=opensearch-operator \
            -n {{ include "primus-lens.namespace" . }} \
            --timeout=300s
          echo "✅ OpenSearch Operator is ready"
          {{- end }}
          
          # 等待 PostgreSQL Operator
          {{- if .Values.pgo.enabled }}
          kubectl wait --for=condition=ready pod \
            -l postgres-operator.crunchydata.com/control-plane=postgres-operator \
            -n {{ include "primus-lens.namespace" . }} \
            --timeout=300s
          echo "✅ PostgreSQL Operator is ready"
          {{- end }}
          
          # 等待 Grafana Operator
          {{- if index .Values "grafana-operator" "enabled" }}
          kubectl wait --for=condition=ready pod \
            -l app.kubernetes.io/name=grafana-operator \
            -n {{ include "primus-lens.namespace" . }} \
            --timeout=300s
          echo "✅ Grafana Operator is ready"
          {{- end }}
          
          echo "🎉 All operators are ready!"
```

### 3.4 数据库初始化

**核心技术**: Kubernetes Job + post-install Hook + initContainer

```yaml
# templates/02-init-jobs/postgres-init-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: primus-lens-postgres-init
  namespace: {{ include "primus-lens.namespace" . }}
  annotations:
    "helm.sh/hook": post-install
    "helm.sh/hook-weight": "10"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 5
  template:
    metadata:
      name: postgres-init
    spec:
      restartPolicy: OnFailure
      serviceAccountName: primus-lens-installer
      
      # 使用 initContainer 等待 PostgreSQL 就绪
      initContainers:
      - name: wait-postgres
        image: postgres:16
        command:
        - /bin/bash
        - -c
        - |
          until pg_isready -h primus-lens-ha.{{ include "primus-lens.namespace" . }}.svc.cluster.local -p 5432 -U postgres; do
            echo "⏳ Waiting for PostgreSQL..."
            sleep 5
          done
          echo "✅ PostgreSQL is ready"
      
      containers:
      - name: init-db
        image: postgres:16
        env:
        - name: PGHOST
          value: primus-lens-ha.{{ include "primus-lens.namespace" . }}.svc.cluster.local
        - name: PGPORT
          value: "5432"
        - name: PGUSER
          value: postgres
        - name: PGPASSWORD
          valueFrom:
            secretKeyRef:
              name: primus-lens-pguser-postgres
              key: password
        - name: PGDATABASE
          value: postgres
        
        # 挂载初始化脚本
        volumeMounts:
        - name: init-script
          mountPath: /scripts
        
        command:
        - /bin/bash
        - -c
        - |
          echo "📥 Initializing PostgreSQL database..."
          psql -f /scripts/setup_primus_lens.sql
          echo "✅ Database initialized successfully"
      
      volumes:
      - name: init-script
        configMap:
          name: primus-lens-postgres-init-script
---
# 将 SQL 脚本作为 ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: primus-lens-postgres-init-script
  namespace: {{ include "primus-lens.namespace" . }}
data:
  setup_primus_lens.sql: |
    {{- .Files.Get "files/setup_primus_lens.sql" | nindent 4 }}
```

### 3.5 密钥管理

**设计原则**: 支持三种密钥来源优先级

1. **外部密钥管理系统** (如 Vault、AWS Secrets Manager) - 最高优先级
2. **通过 helm install --set 传递** - 中等优先级
3. **空密钥占位符** - 最低优先级 (部署后手动更新)

```yaml
# templates/01-secrets/image-pull-secret.yaml
{{- if .Values.global.imagePullSecrets }}
{{- range .Values.global.imagePullSecrets }}
{{- if or .credentials.username .credentials.password }}
---
apiVersion: v1
kind: Secret
metadata:
  name: {{ .name }}
  namespace: {{ include "primus-lens.namespace" $ }}
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: {{ include "primus-lens.dockerConfigJson" . | b64enc }}
{{- else }}
---
apiVersion: v1
kind: Secret
metadata:
  name: {{ .name }}
  namespace: {{ include "primus-lens.namespace" $ }}
  annotations:
    description: "Empty placeholder. Update manually after deployment."
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: e30K  # Empty JSON object "{}" in base64
{{- end }}
{{- end }}
{{- end }}
```

```yaml
# templates/_helpers.tpl 中的密钥生成函数
{{- define "primus-lens.dockerConfigJson" -}}
{{- $registry := .credentials.registry -}}
{{- $username := .credentials.username -}}
{{- $password := .credentials.password -}}
{{- $auth := printf "%s:%s" $username $password | b64enc -}}
{{- $config := dict "auths" (dict $registry (dict "username" $username "password" $password "auth" $auth)) -}}
{{- $config | toJson -}}
{{- end -}}
```

**部署时传递密钥**:

```bash
# 方式 1: 通过命令行 --set
helm install primus-lens ./primus-lens \
  --set global.imagePullSecrets[0].credentials.username=myuser \
  --set global.imagePullSecrets[0].credentials.password=mypass

# 方式 2: 通过环境变量和 values 文件模板
export DOCKER_USERNAME="myuser"
export DOCKER_PASSWORD="mypass"
envsubst < values-prod.yaml.tmpl > values-prod.yaml
helm install primus-lens ./primus-lens -f values-prod.yaml

# 方式 3: 通过外部密钥管理 (推荐生产环境)
helm install primus-lens ./primus-lens \
  --set-file global.imagePullSecrets[0].credentials.password=<(aws secretsmanager get-secret-value --secret-id docker-pass --query SecretString --output text)
```

---

## 4. 部署流程图

### 4.1 完整部署流程 (时序图)

```
用户                Helm CLI           Kubernetes API       Operators         应用组件
 │                     │                     │                   │                │
 │  helm install       │                     │                   │                │
 │─────────────────────>│                     │                   │                │
 │                     │                     │                   │                │
 │                     │ [Phase 0: Pre-Install Hooks]           │                │
 │                     │                     │                   │                │
 │                     │  创建 Namespace      │                   │                │
 │                     │────────────────────>│                   │                │
 │                     │                     │                   │                │
 │                     │  创建 Secrets        │                   │                │
 │                     │────────────────────>│                   │                │
 │                     │                     │                   │                │
 │                     │ [Phase 1: 部署子 Charts - Operators]   │                │
 │                     │                     │                   │                │
 │                     │  安装 VM Operator    │                   │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │                     │                   │ (Pod Running)  │
 │                     │  安装 Fluent Operator│                   │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │  安装 OpenSearch Op  │                   │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │  安装 PGO           │                   │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │  安装 Grafana Op    │                   │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │                     │                   │                │
 │                     │ [Phase 2: 等待 Operators 就绪]          │                │
 │                     │                     │                   │                │
 │                     │  创建 wait-operators Job                │                │
 │                     │────────────────────>│                   │                │
 │                     │                     │  kubectl wait     │                │
 │                     │                     │──────────────────>│                │
 │                     │                     │  所有 Operators Ready               │
 │                     │                     │<──────────────────│                │
 │                     │  Job Succeeded      │                   │                │
 │                     │<────────────────────│                   │                │
 │                     │                     │                   │                │
 │                     │ [Phase 3: 部署基础设施 CRs]             │                │
 │                     │                     │                   │                │
 │                     │  创建 PostgresCluster CR               │                │
 │                     │────────────────────>│                   │                │
 │                     │                     │  Reconcile        │                │
 │                     │                     │──────────────────>│                │
 │                     │                     │  创建 PG Pods      │                │
 │                     │                     │<──────────────────│                │
 │                     │                     │                   │                │
 │                     │  创建 OpenSearchCluster CR             │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │  创建 VMCluster CR                      │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │  创建 VMAgent CR                        │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │  创建 FluentBit Config                  │                │
 │                     │────────────────────>│──────────────────>│                │
 │                     │                     │                   │                │
 │                     │ [Phase 4: Post-Install Hooks - 初始化]  │                │
 │                     │                     │                   │                │
 │                     │  创建 postgres-init Job                 │                │
 │                     │────────────────────>│                   │                │
 │                     │                     │  initContainer:    │                │
 │                     │                     │  wait for PG ready │                │
 │                     │                     │──────────────────>│                │
 │                     │                     │  PG Ready          │                │
 │                     │                     │<──────────────────│                │
 │                     │                     │  执行 SQL 脚本     │                │
 │                     │                     │──────────────────>│                │
 │                     │  Job Succeeded      │                   │                │
 │                     │<────────────────────│                   │                │
 │                     │                     │                   │                │
 │                     │  创建 opensearch-init Job               │                │
 │                     │────────────────────>│                   │                │
 │                     │  (类似流程)          │                   │                │
 │                     │                     │                   │                │
 │                     │ [Phase 5: 部署应用组件]                 │                │
 │                     │                     │                   │                │
 │                     │  创建 app Deployments                   │               │
 │                     │────────────────────>│                   │                │
 │                     │                     │  创建 Pods         │                │
 │                     │                     │───────────────────────────────────>│
 │                     │                     │                   │  (Apps Running)│
 │                     │  创建 Services       │                   │                │
 │                     │────────────────────>│                   │                │
 │                     │  创建 Ingress        │                   │                │
 │                     │────────────────────>│                   │                │
 │                     │                     │                   │                │
 │                     │  创建 validation Job │                   │                │
 │                     │────────────────────>│                   │                │
 │                     │  验证服务可用性       │                   │                │
 │                     │                     │───────────────────────────────────>│
 │                     │                     │  Health Check OK   │                │
 │                     │                     │<───────────────────────────────────│
 │                     │  Job Succeeded      │                   │                │
 │                     │<────────────────────│                   │                │
 │                     │                     │                   │                │
 │   安装成功           │                     │                   │                │
 │<─────────────────────│                     │                   │                │
 │  (显示 NOTES.txt)    │                     │                   │                │
```

### 4.2 依赖关系图 (DAG)

```
                                   [helm install]
                                         │
                   ┌─────────────────────┼─────────────────────┐
                   │                     │                     │
              [Namespace]           [Secrets]              [CRDs]
                   │                     │                     │
                   └─────────────────────┼─────────────────────┘
                                         │
                           [Wait for Dependencies]
                                         │
             ┌───────────────────────────┼───────────────────────────┐
             │                           │                           │
             │                           │                           │
    [VM Operator] ────┐         [Fluent Operator] ────┐     [PGO] ────┐
             │        │                  │            │        │        │
    [OpenSearch Op]   │         [Grafana Operator]    │        │        │
             │        │                  │            │        │        │
             └────────┼──────────────────┴────────────┼────────┴────────┘
                      │                               │
                [Operators Ready]                     │
                      │                               │
          ┌───────────┴───────────┐                   │
          │                       │                   │
     [VMCluster]            [OpenSearchCluster]  [PostgresCluster]
          │                       │                   │
     [VMAgent]              [FluentBit Config]        │
          │                       │                   │
          └───────────────────────┴───────────────────┘
                                  │
                          [Infrastructure Ready]
                                  │
                          [PostgreSQL Init Job]
                                  │
                          [OpenSearch Init Job]
                                  │
                          [Database Ready]
                                  │
              ┌───────────────────┼───────────────────┐
              │                   │                   │
          [App API]        [Telemetry Collector]  [Jobs Service]
              │                   │                   │
          [Node Exporter]   [GPU Exporter]      [System Tuner]
              │                   │                   │
              └───────────────────┼───────────────────┘
                                  │
                            [App Web Console]
                                  │
                         [Grafana CR + Dashboards]
                                  │
                            [Ingress/Service]
                                  │
                          [Validation Job]
                                  │
                            [🎉 Complete]
```

### 4.3 状态转换图

```
                         ┌──────────────┐
                         │  Not Installed│
                         └──────┬───────┘
                                │ helm install
                                ▼
                         ┌──────────────┐
                         │  Installing  │◄────┐
                         └──────┬───────┘     │
                                │             │ 重试 (Job Failed)
                                ▼             │
                         ┌──────────────┐     │
                         │Operators     │─────┘
                         │Deploying     │
                         └──────┬───────┘
                                │
                                ▼
                         ┌──────────────┐
                         │Infrastructure│
                         │Deploying     │
                         └──────┬───────┘
                                │
                                ▼
                         ┌──────────────┐
                         │Initializing  │◄────┐
                         │(Running Jobs)│     │ 重试
                         └──────┬───────┘     │
                                │             │
                                ▼             │
                         ┌──────────────┐     │
                         │Apps Deploying│─────┘
                         └──────┬───────┘
                                │
                                ▼
                         ┌──────────────┐
                         │  Validating  │
                         └──────┬───────┘
                                │
                    ┌───────────┴───────────┐
                    │                       │
                    ▼                       ▼
            ┌──────────────┐        ┌──────────────┐
            │   Deployed   │        │   Failed     │
            │  (Success)   │        │              │
            └──────────────┘        └──────┬───────┘
                    │                      │
                    │ helm upgrade         │ helm rollback
                    ▼                      ▼
            ┌──────────────┐        ┌──────────────┐
            │  Upgrading   │        │  Rolling Back│
            └──────────────┘        └──────────────┘
```

---

## 5. 关键技术点实现

### 5.1 条件渲染

根据配置动态启用/禁用组件:

```yaml
# templates/08-ingress/nginx-ingress.yaml
{{- if and (eq .Values.global.accessType "ingress") (not (eq .Values.net.ingress "higress")) }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: primus-lens-console
  namespace: {{ include "primus-lens.namespace" . }}
spec:
  ingressClassName: nginx
  rules:
  - host: {{ .Values.global.clusterName }}.{{ .Values.global.domain }}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: primus-lens-web
            port:
              number: 80
{{- end }}
```

### 5.2 动态 Profile 选择

```yaml
# templates/05-database/pg-cr.yaml
{{- $profile := include "primus-lens.profileConfig" . | fromYaml }}
apiVersion: postgres-operator.crunchydata.com/v1beta1
kind: PostgresCluster
metadata:
  name: primus-lens
  namespace: {{ include "primus-lens.namespace" . }}
spec:
  postgresVersion: 16
  instances:
  - name: instance1
    replicas: {{ $profile.postgres.replicas }}
    dataVolumeClaimSpec:
      accessModes:
      - {{ .Values.global.accessMode }}
      resources:
        requests:
          storage: {{ $profile.postgres.dataSize }}
      storageClassName: {{ include "primus-lens.storageClass" . }}
  backups:
    pgbackrest:
      repos:
      - name: repo1
        volume:
          volumeClaimSpec:
            accessModes:
            - {{ .Values.global.accessMode }}
            resources:
              requests:
                storage: {{ $profile.postgres.backupSize }}
            storageClassName: {{ include "primus-lens.storageClass" . }}
```

### 5.3 密码传递和引用

```yaml
# templates/03-apps/app-api.yaml
env:
- name: DB_HOST
  value: primus-lens-ha.{{ include "primus-lens.namespace" . }}.svc.cluster.local
- name: DB_PORT
  value: "5432"
- name: DB_USER
  value: primus-lens
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: primus-lens-pguser-primus-lens
      key: password  # 由 PGO 自动生成
- name: DB_NAME
  value: primus_lens
```

### 5.4 Grafana Dashboards 自动导入

```yaml
# templates/07-grafana/dashboards/node-exporter.yaml
{{- if .Values.grafana-operator.enabled }}
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDashboard
metadata:
  name: node-exporter
  namespace: {{ include "primus-lens.namespace" . }}
  labels:
    app: grafana
spec:
  instanceSelector:
    matchLabels:
      dashboards: "primus-lens"
  
  # 方式 1: 从文件加载 JSON
  json: |
    {{- .Files.Get "files/dashboards/NodeExporter.json" | nindent 4 }}
  
  # 方式 2: 从 ConfigMap 加载
  # configMapRef:
  #   name: dashboard-node-exporter
  #   key: dashboard.json
{{- end }}
```

### 5.5 子 Chart 配置透传

```yaml
# values.yaml
victoria-metrics-operator:  # 子 Chart 名称
  enabled: true
  operator:
    enable_converter_ownership: true
    resources:
      limits:
        cpu: 200m
        memory: 150Mi
      requests:
        cpu: 50m
        memory: 100Mi
  
  # 透传镜像仓库配置
  image:
    repository: {{ .Values.global.imageRegistry }}/victoriametrics/operator
    pullSecrets:
      - name: {{ (index .Values.global.imagePullSecrets 0).name }}
```

---

## 6. 安装和使用

### 6.1 准备工作

```bash
# 1. 添加 Chart 依赖
cd primus-lens
helm dependency update

# 2. 验证 Chart 语法
helm lint .

# 3. 渲染模板查看生成的资源 (Dry-run)
helm template primus-lens . \
  -f values.yaml \
  -f values-dev.yaml \
  --debug \
  > rendered.yaml
```

### 6.2 安装命令

```bash
# 最小化安装 (默认配置)
helm install primus-lens ./primus-lens \
  --namespace primus-lens \
  --create-namespace

# 使用自定义配置文件
helm install primus-lens ./primus-lens \
  -f values-prod.yaml \
  --namespace primus-lens \
  --create-namespace

# 覆盖特定参数
helm install primus-lens ./primus-lens \
  --set global.clusterName=my-cluster \
  --set profile=large \
  --set global.storageClass=ceph-rbd \
  --set global.accessType=ingress \
  --set global.imagePullSecrets[0].credentials.username=myuser \
  --set global.imagePullSecrets[0].credentials.password=mypass \
  --namespace primus-lens \
  --create-namespace

# 带超时和等待
helm install primus-lens ./primus-lens \
  -f values-prod.yaml \
  --timeout 30m \
  --wait \
  --wait-for-jobs \
  --namespace primus-lens \
  --create-namespace
```

### 6.3 升级

```bash
# 升级到新版本
helm upgrade primus-lens ./primus-lens \
  -f values-prod.yaml \
  --namespace primus-lens

# 升级并强制重建 Pods
helm upgrade primus-lens ./primus-lens \
  -f values-prod.yaml \
  --force \
  --namespace primus-lens

# 升级时修改配置
helm upgrade primus-lens ./primus-lens \
  --set apps.api.replicas=5 \
  --namespace primus-lens
```

### 6.4 回滚

```bash
# 查看历史版本
helm history primus-lens -n primus-lens

# 回滚到上一个版本
helm rollback primus-lens -n primus-lens

# 回滚到指定版本
helm rollback primus-lens 3 -n primus-lens
```

### 6.5 卸载

```bash
# 卸载 Release (保留 PVC)
helm uninstall primus-lens -n primus-lens

# 卸载并删除命名空间
helm uninstall primus-lens -n primus-lens
kubectl delete namespace primus-lens

# 如需清理 PVC (慎重!)
kubectl delete pvc -n primus-lens --all
```

### 6.6 调试

```bash
# 查看渲染后的 manifests
helm get manifest primus-lens -n primus-lens

# 查看所有资源状态
helm status primus-lens -n primus-lens

# 查看 Hooks 执行情况
kubectl get jobs -n primus-lens
kubectl logs job/primus-lens-wait-operators -n primus-lens
kubectl logs job/primus-lens-postgres-init -n primus-lens

# 检查依赖 Chart
helm dependency list ./primus-lens
```

---

## 7. 对比分析

### 7.1 脚本方式 vs Helm 方式

| 维度 | 脚本方式 (当前) | Helm 方式 (目标) |
|------|----------------|-----------------|
| **部署命令** | `bash install.sh` (需交互输入) | `helm install primus-lens ./primus-lens -f values.yaml` |
| **配置管理** | 分散在脚本和模板文件中 | 集中在 values.yaml，支持多环境 |
| **依赖管理** | 手动 git clone 和 helm repo add | Chart.yaml 中声明，自动下载 |
| **部署顺序** | 脚本中 sleep 等待 | Helm hooks + K8s probes 自动编排 |
| **错误处理** | 脚本可能在某步骤失败后退出 | K8s Job 自动重试，Helm 支持回滚 |
| **幂等性** | 需脚本自行处理 (kubectl apply) | Helm 原生支持 |
| **版本管理** | 无版本概念 | Helm release history，支持回滚 |
| **升级** | 重新运行脚本 (可能有风险) | `helm upgrade` 安全升级 |
| **CI/CD 集成** | 需要处理交互输入，复杂 | 标准化 Helm 命令，易集成 |
| **可审计性** | 难以追踪变更历史 | Helm values 可存储在 Git，完整审计 |
| **多集群管理** | 每个集群需重新运行脚本 | 使用不同 values 文件一键部署 |

### 7.2 迁移成本评估

| 阶段 | 工作量 | 风险 | 建议 |
|------|-------|------|------|
| **Chart 结构设计** | 3-5 天 | 低 | 使用本文档作为蓝图 |
| **模板转换** | 5-7 天 | 中 | 将现有 .tpl 文件转为 Helm 模板 |
| **Hooks 实现** | 3-4 天 | 中 | 重点测试 wait-for-operators 和 init jobs |
| **依赖配置** | 2-3 天 | 低 | 使用官方 Helm Charts |
| **测试** | 5-7 天 | 高 | 在测试环境充分测试，覆盖各种场景 |
| **文档** | 2-3 天 | 低 | 更新安装文档和 troubleshooting |
| **总计** | 20-29 天 | 中 | 建议分阶段迁移，保留脚本作为备份 |

---

## 8. 最佳实践建议

### 8.1 开发阶段

1. **模块化拆分**: 按功能将模板拆分到不同目录，便于维护
2. **使用 _helpers.tpl**: 封装通用逻辑，避免重复
3. **命名规范**: 使用 `{{ include "primus-lens.fullname" . }}-component` 模式
4. **注释充分**: 在模板中添加注释说明复杂逻辑
5. **版本锁定**: 在 Chart.yaml 中明确依赖版本

### 8.2 测试阶段

1. **Dry-run 测试**: 先使用 `helm template` 检查生成的资源
2. **分环境测试**: 测试 minimal, normal, large 三种 profile
3. **网络场景**: 测试 ssh-tunnel 和 ingress 两种访问方式
4. **失败场景**: 故意触发 Job 失败，验证重试机制
5. **升级测试**: 测试从旧版本升级到新版本

### 8.3 生产部署

1. **使用 values 文件**: 避免 --set 传递大量参数
2. **密钥管理**: 集成 Vault 或 Sealed Secrets 管理敏感信息
3. **备份 values**: 将 values 文件存储在 Git 仓库
4. **监控安装**: 使用 `--wait --wait-for-jobs` 等待部署完成
5. **设置超时**: 使用 `--timeout` 避免长时间阻塞
6. **日志收集**: 保存安装日志供问题排查

### 8.4 运维阶段

1. **定期升级**: 使用 `helm upgrade` 升级组件版本
2. **配置变更**: 通过修改 values 文件并 upgrade 实现
3. **监控 Hooks**: 定期检查 Jobs 的执行历史和日志
4. **资源清理**: 使用 `helm.sh/hook-delete-policy` 自动清理临时资源
5. **备份策略**: 定期备份数据库和关键 ConfigMap/Secret

---

## 9. 常见问题和解决方案

### Q1: Operators 长时间未 Ready 导致安装超时

**解决方案**:
- 增加 wait-for-operators Job 的 `backoffLimit`
- 检查镜像拉取是否正常 (imagePullSecrets)
- 调整 `--timeout` 参数

### Q2: 数据库初始化 Job 失败

**解决方案**:
- 检查 PostgreSQL CR 是否成功创建
- 查看 init Job 的日志: `kubectl logs job/primus-lens-postgres-init`
- 验证 SQL 脚本语法是否正确
- Job 会自动重试，无需手动干预

### Q3: 如何在已有集群中只升级部分组件

**解决方案**:
```bash
# 方式 1: 使用条件渲染
helm upgrade primus-lens ./primus-lens \
  --set apps.api.enabled=true \
  --set apps.web.enabled=false \
  --reuse-values

# 方式 2: 单独管理子 Charts
helm upgrade vm-operator ./primus-lens/charts/victoria-metrics-operator
```

### Q4: 如何在不同命名空间部署多个实例

**解决方案**:
```bash
# 实例 1
helm install primus-lens-dev ./primus-lens \
  -f values-dev.yaml \
  --namespace primus-lens-dev \
  --create-namespace

# 实例 2
helm install primus-lens-prod ./primus-lens \
  -f values-prod.yaml \
  --namespace primus-lens-prod \
  --create-namespace
```

### Q5: 如何处理敏感信息 (密码、API Key)

**解决方案**:
```bash
# 方式 1: 使用 Helm Secrets 插件
helm secrets install primus-lens ./primus-lens -f values.yaml -f secrets.yaml.enc

# 方式 2: 使用外部 Secret Operator
# 在 values.yaml 中引用外部密钥
global:
  imagePullSecrets:
    - name: primus-lens-image
      external:
        secretStore: vault
        key: docker-credentials

# 方式 3: 使用 Sealed Secrets
kubeseal -f secrets.yaml -w sealed-secrets.yaml
helm install primus-lens ./primus-lens -f sealed-secrets.yaml
```

---

## 10. 下一步行动计划

### 阶段 1: 原型验证 (Week 1-2)
- [ ] 搭建基础 Chart 结构
- [ ] 实现 Operators 子 Chart 依赖
- [ ] 实现核心 Hooks (wait-for-operators, postgres-init)
- [ ] 在测试环境验证基本部署流程

### 阶段 2: 功能完善 (Week 3-4)
- [ ] 转换所有应用组件模板
- [ ] 实现 Profile 配置逻辑
- [ ] 实现密钥管理
- [ ] 实现 Grafana Dashboards 自动导入
- [ ] 添加 Ingress 配置

### 阶段 3: 测试和文档 (Week 5-6)
- [ ] 完整端到端测试 (minimal, normal, large)
- [ ] 失败场景测试和优化
- [ ] 升级和回滚测试
- [ ] 编写用户文档和 troubleshooting
- [ ] 性能测试和优化

### 阶段 4: 生产准备 (Week 7-8)
- [ ] 安全审计
- [ ] 集成 CI/CD
- [ ] 生产环境试运行
- [ ] 收集反馈和优化
- [ ] 正式发布

---

## 11. 参考资源

- [Helm 官方文档](https://helm.sh/docs/)
- [Helm Charts 最佳实践](https://helm.sh/docs/chart_best_practices/)
- [Helm Hooks 文档](https://helm.sh/docs/topics/charts_hooks/)
- [Kubernetes Job 模式](https://kubernetes.io/docs/concepts/workloads/controllers/job/)
- [VictoriaMetrics Operator Helm Chart](https://github.com/VictoriaMetrics/helm-charts)
- [Fluent Operator Helm Chart](https://github.com/fluent/fluent-operator)
- [OpenSearch Operator](https://github.com/opensearch-project/opensearch-k8s-operator)
- [Crunchy PostgreSQL Operator](https://github.com/CrunchyData/postgres-operator)
- [Grafana Operator](https://grafana.com/docs/grafana-cloud/developer-resources/infrastructure-as-code/helm/)

---

## 12. 总结

通过将当前基于脚本的部署方式重构为纯 Helm Chart 方式，可以实现：

✅ **简化部署**: 一条命令完成所有组件安装  
✅ **标准化**: 使用 Helm 生态的标准工具和实践  
✅ **可维护性**: 配置集中管理，版本可控  
✅ **可靠性**: 利用 K8s 原生机制处理依赖和重试  
✅ **可扩展性**: 支持多环境、多集群部署  
✅ **GitOps 友好**: 配置即代码，支持 CI/CD  

虽然前期需要投入一定的开发和测试成本，但长期来看将大幅降低运维复杂度，提升用户体验和系统稳定性。

