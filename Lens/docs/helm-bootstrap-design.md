# Primus-Lens Helm Chart 设计文档

## 1. 概述

本文档描述将 Primus-Lens bootstrap 脚本改造为 Helm Chart 的详细设计方案，包括目录结构、模板组织、安装顺序、等待逻辑和动态配置生成方式。

## 2. Chart 目录结构

```
primus-lens/
├── Chart.yaml                                    # Chart 元数据定义
├── Chart.lock                                    # 依赖锁定文件
├── values.yaml                                   # 默认配置值
├── values.schema.json                            # 配置值的 JSON Schema 验证
│
├── examples/                                     # 部署示例配置
│   ├── values-management.yaml                    # 管理集群配置示例
│   ├── values-data.yaml                          # 数据集群配置示例
│   └── values-all-in-one.yaml                   # 一体化配置示例
│
├── charts/                                       # 子 Chart 依赖（可选）
│   └── .gitkeep                                  # 暂不使用子 Chart，通过 Operator 管理
│
├── crds/                                         # 自定义 CRDs（如需要）
│   └── .gitkeep
│
├── templates/
│   ├── NOTES.txt                                # 安装后提示信息
│   ├── _helpers.tpl                             # 全局辅助函数和模板片段
│   │
│   ├── 00-common/                               # 阶段 0：通用基础组件
│   │   ├── namespace.yaml                       # Namespace 定义
│   │   ├── serviceaccount.yaml                  # ServiceAccount 和 RBAC
│   │   ├── clusterrole.yaml                     # ClusterRole
│   │   ├── clusterrolebinding.yaml              # ClusterRoleBinding
│   │   ├── imagepullsecret.yaml                 # ImagePullSecret
│   │   └── cert-secret.yaml                     # 证书 Secret
│   │
│   ├── 05-system-tuner/                         # 阶段 0.5：系统调优（在中间件之前）
│   │   ├── daemonset.yaml                       # System Tuner DaemonSet
│   │   └── wait-job.yaml                        # Hook: 等待 System Tuner 就绪
│   │
│   ├── 10-middleware-operators/                 # 阶段 1：中间件 Operators
│   │   ├── _middleware-helpers.tpl              # 中间件相关辅助函数
│   │   │
│   │   ├── postgresql/
│   │   │   ├── 00-operator-install-job.yaml     # Hook: 通过 Job 安装 PG Operator
│   │   │   └── 01-operator-values-configmap.yaml # PG Operator 配置
│   │   │
│   │   ├── opensearch/
│   │   │   ├── 00-operator-install-job.yaml     # Hook: 通过 Job 安装 OpenSearch Operator
│   │   │   └── 01-operator-values-configmap.yaml
│   │   │
│   │   ├── victoriametrics/
│   │   │   ├── 00-operator-install-job.yaml     # Hook: 通过 Job 安装 VM Operator
│   │   │   └── 01-operator-values-configmap.yaml
│   │   │
│   │   ├── fluentbit/
│   │   │   ├── 00-operator-install-job.yaml     # Hook: 通过 Job 安装 FluentBit Operator
│   │   │   └── 01-operator-values-configmap.yaml
│   │   │
│   │   └── grafana/
│   │       ├── 00-operator-install-job.yaml     # Hook: 通过 Job 安装 Grafana Operator
│   │       └── 01-operator-values-configmap.yaml
│   │
│   ├── 20-middleware-instances/                 # 阶段 2：中间件实例（CR）
│   │   ├── postgresql/
│   │   │   ├── 00-postgres-cluster.yaml         # PostgresCluster CR
│   │   │   ├── 01-wait-job.yaml                 # Hook: 等待 PG 就绪
│   │   │   ├── 02-init-db-job.yaml              # Hook: 初始化数据库 Schema
│   │   │   └── 03-password-extract-job.yaml     # Hook: 提取密码到 ConfigMap/Secret
│   │   │
│   │   ├── opensearch/
│   │   │   ├── 00-opensearch-cluster.yaml       # OpenSearchCluster CR
│   │   │   └── 01-wait-job.yaml                 # Hook: 等待 OpenSearch 就绪
│   │   │
│   │   ├── victoriametrics/
│   │   │   ├── 00-vmcluster.yaml                # VMCluster CR
│   │   │   ├── 01-vmagent.yaml                  # VMAgent CR
│   │   │   └── 02-wait-job.yaml                 # Hook: 等待 VM 就绪
│   │   │
│   │   └── otel-collector/
│   │       ├── 00-configmap.yaml                # Collector 配置
│   │       ├── 01-deployment.yaml               # Collector Deployment
│   │       ├── 02-service.yaml                  # Collector Service
│   │       └── 03-wait-job.yaml                 # Hook: 等待 Collector 就绪
│   │
│   ├── 30-management-components/                # 阶段 3：管理集群组件
│   │   ├── _management-helpers.tpl              # 管理集群辅助函数
│   │   │
│   │   ├── api/
│   │   │   ├── configmap.yaml                   # API 配置
│   │   │   ├── deployment.yaml                  # API Deployment
│   │   │   ├── service.yaml                     # API Service
│   │   │   └── ingress.yaml                     # API Ingress（可选）
│   │   │
│   │   ├── safe-adapter/
│   │   │   ├── configmap.yaml                   # Safe Adapter 配置
│   │   │   ├── deployment.yaml                  # Safe Adapter Deployment
│   │   │   └── service.yaml                     # Safe Adapter Service
│   │   │
│   │   ├── jobs-management/
│   │   │   ├── configmap.yaml                   # Jobs 配置（管理模式）
│   │   │   ├── deployment.yaml                  # Jobs Deployment
│   │   │   └── service.yaml                     # gRPC Service
│   │   │
│   │   ├── telemetry-processor-management/
│   │   │   ├── configmap.yaml                   # Telemetry Processor 配置（管理模式）
│   │   │   ├── deployment.yaml                  # Telemetry Processor Deployment
│   │   │   └── service.yaml                     # Service
│   │   │
│   │   └── multi-cluster-config-exporter/
│   │       ├── configmap.yaml                   # Multi-Cluster Config Exporter 配置
│   │       └── deployment.yaml                  # Deployment
│   │
│   ├── 40-data-components/                      # 阶段 4：数据集群组件
│   │   ├── _data-helpers.tpl                    # 数据集群辅助函数
│   │   │
│   │   ├── jobs-data/
│   │   │   ├── configmap.yaml                   # Jobs 配置（数据模式）
│   │   │   ├── deployment.yaml                  # Jobs Deployment
│   │   │   └── service.yaml                     # gRPC Service
│   │   │
│   │   ├── telemetry-processor-data/
│   │   │   ├── configmap.yaml                   # Telemetry Processor 配置（数据模式）
│   │   │   ├── deployment.yaml                  # Telemetry Processor Deployment
│   │   │   └── service.yaml                     # Service
│   │   │
│   │   ├── gpu-resource-exporter/
│   │   │   ├── configmap.yaml                   # GPU Resource Exporter 配置
│   │   │   ├── deployment.yaml                  # Deployment
│   │   │   └── service.yaml                     # Service
│   │   │
│   │   └── node-exporter/
│   │       ├── configmap.yaml                   # Node Exporter 配置
│   │       ├── daemonset.yaml                   # DaemonSet
│   │       └── service.yaml                     # Service
│   │
│   ├── 50-observability/                        # 阶段 5：可观测性组件
│   │   ├── grafana/
│   │   │   ├── 00-grafana-cr.yaml               # Grafana CR（仅管理集群）
│   │   │   ├── 01-datasources.yaml              # 数据源配置
│   │   │   ├── 02-folders.yaml                  # Dashboard 文件夹
│   │   │   ├── 03-dashboards-general.yaml       # 通用 Dashboards
│   │   │   ├── 04-dashboards-kubernetes.yaml    # Kubernetes Dashboards
│   │   │   ├── 05-dashboards-middleware.yaml    # 中间件 Dashboards
│   │   │   ├── 06-dashboards-node.yaml          # 节点 Dashboards
│   │   │   ├── 07-ingress.yaml                  # Grafana Ingress（可选）
│   │   │   └── 08-nginx-proxy.yaml              # Nginx 代理（SSH Tunnel 模式）
│   │   │
│   │   ├── fluentbit/
│   │   │   ├── 00-fluentbit-config.yaml         # FluentBit 配置 CR
│   │   │   └── 01-scrape-config.yaml            # 日志采集配置
│   │   │
│   │   └── vmscrape/
│   │       ├── 00-basic-metrics.yaml            # 基础指标采集配置
│   │       ├── 01-kube-state-metrics.yaml       # Kube State Metrics 配置
│   │       └── 02-node-metrics.yaml             # 节点指标采集配置
│   │
│   └── 60-post-install/                         # 阶段 6：安装后配置
│       ├── kube-state-metrics-job.yaml          # Hook: 安装 Kube State Metrics
│       └── validation-job.yaml                  # Hook: 验证安装完整性
│
└── files/                                        # 静态文件
    ├── dashboards/                              # Grafana Dashboard JSON 文件
    │   ├── general/
    │   ├── kubernetes/
    │   ├── middleware/
    │   └── node/
    │
    └── scripts/                                 # Shell 脚本
        ├── install-operator.sh                  # Operator 安装脚本（通用）
        ├── wait-for-ready.sh                    # 等待资源就绪脚本
        ├── init-database.sql                    # 数据库初始化 SQL
        └── extract-password.sh                  # 提取密码脚本
```

## 3. 安装阶段和顺序

### 3.1 安装阶段定义

Helm Chart 通过以下机制控制安装顺序：

1. **目录命名约定**：使用数字前缀（00-、10-、20-...）表示逻辑阶段
2. **Helm Hooks**：使用 annotations 控制执行时机
3. **Hook 权重**：使用 `helm.sh/hook-weight` 控制同一阶段内的顺序

### 3.2 详细安装流程

```yaml
# 安装流程时序图
Phase 0: Pre-Install (Helm Hook: pre-install)
├── 创建 Namespace
├── 创建 ServiceAccount 和 RBAC
├── 创建 ImagePullSecret
└── 创建证书 Secret

↓

Phase 0.5: System Tuner (Helm Hook: pre-install, weight: -10 to 0)
├── [weight: -10] 部署 System Tuner DaemonSet
│   └── 调整系统内核参数（vm.max_map_count, nofile limits）
└── [weight: 0] 等待 System Tuner 就绪
    └── Job: kubectl wait --for=condition=Ready pod -l app=system-tuner

↓

Phase 1: Operators Installation (Helm Hook: pre-install, weight: 10-90)
├── [weight: 10]  安装 PostgreSQL Operator
│   └── Job: helm repo add + helm install postgresql-operator
├── [weight: 20] 等待 PostgreSQL Operator 就绪
│   └── Job: kubectl wait --for=condition=Ready pod -l app=pgo
│
├── [weight: 30] 安装 OpenSearch Operator
│   └── Job: helm repo add + helm install opensearch-operator
├── [weight: 40] 等待 OpenSearch Operator 就绪
│
├── [weight: 50] 安装 VictoriaMetrics Operator
│   └── Job: helm repo add + helm install victoria-metrics-operator
├── [weight: 60] 等待 VictoriaMetrics Operator 就绪
│
├── [weight: 70] 安装 FluentBit Operator
│   └── Job: helm repo add + helm install fluent-operator
├── [weight: 80] 等待 FluentBit Operator 就绪
│
└── [weight: 85] 安装 Grafana Operator（仅管理集群）
    └── Job: helm repo add + helm install grafana-operator
    └── [weight: 90] 等待 Grafana Operator 就绪

↓

Phase 2: Middleware Instances (Helm Hook: pre-install, weight: 100-200)
├── [weight: 100] 创建 PostgreSQL Cluster CR
├── [weight: 110] 等待 PostgreSQL 就绪
│   └── Job: 等待 endpoints 有 IP + 等待 pod Running
├── [weight: 120] 初始化数据库
│   └── Job: kubectl exec pod -- psql < init-database.sql
├── [weight: 130] 提取 PostgreSQL 密码
│   └── Job: 从 Secret 提取密码，写入 ConfigMap 供后续组件使用
│
├── [weight: 140] 创建 OpenSearch Cluster CR
├── [weight: 150] 等待 OpenSearch 就绪
│   └── Job: kubectl wait + curl health check
│
├── [weight: 160] 创建 VictoriaMetrics Cluster CR
├── [weight: 165] 创建 VMAgent CR
├── [weight: 170] 等待 VictoriaMetrics 就绪
│   └── Job: kubectl wait + curl health check
│
└── [weight: 180] 部署 Otel Collector
    ├── 创建 ConfigMap
    ├── 创建 Deployment
    ├── 创建 Service
    └── [weight: 190] 等待 Otel Collector 就绪

↓

Phase 3: Main Installation (Normal Templates, 按字母序渲染)
├── 30-management-components/ (条件渲染：仅在 management 或 all-in-one 模式)
│   ├── API Deployment + Service
│   ├── Safe Adapter Deployment + Service
│   ├── Jobs (Management Mode) Deployment + Service
│   ├── Telemetry Processor (Management Mode) Deployment + Service
│   └── Multi-Cluster Config Exporter Deployment
│
├── 40-data-components/ (条件渲染：仅在 data 或 all-in-one 模式)
│   ├── Jobs (Data Mode) Deployment + Service
│   ├── Telemetry Processor (Data Mode) Deployment + Service
│   ├── GPU Resource Exporter Deployment + Service
│   └── Node Exporter DaemonSet + Service
│
└── 50-observability/
    ├── Grafana CR + Datasources + Dashboards (仅管理集群)
    ├── FluentBit Config CR
    └── VMScrape Configs

↓

Phase 4: Post-Install (Helm Hook: post-install)
├── [weight: 0] 安装 Kube State Metrics
│   └── Job: git clone + kubectl apply
├── [weight: 10] 配置 Nginx 代理（SSH Tunnel 模式）
│   └── 创建 Nginx Deployment + Service + ConfigMap
└── [weight: 20] 验证安装
    └── Job: 检查所有组件状态，输出访问信息
```

## 4. Helm Hooks 详细设计

### 4.1 Hook 类型和使用场景

```yaml
# Hook 类型映射表
┌─────────────────────┬────────────────────────────────────────┐
│ Hook 类型            │ 使用场景                                │
├─────────────────────┼────────────────────────────────────────┤
│ pre-install         │ 安装前准备工作（Operators、中间件）      │
│ post-install        │ 安装后配置（Kube State Metrics、验证）  │
│ pre-delete          │ 删除前清理（Finalizers）                │
│ post-delete         │ 删除后清理（PVC、Operator 卸载）         │
│ pre-upgrade         │ 升级前备份                              │
│ post-upgrade        │ 升级后验证                              │
│ pre-rollback        │ 回滚前准备                              │
│ post-rollback       │ 回滚后验证                              │
└─────────────────────┴────────────────────────────────────────┘
```

### 4.2 System Tuner Hook 示例

```yaml
# templates/05-system-tuner/daemonset.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {{ include "primus-lens.fullname" . }}-system-tuner
  namespace: {{ .Values.global.namespace }}
  labels:
    {{- include "primus-lens.labels" . | nindent 4 }}
    component: system-tuner
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "-10"
spec:
  selector:
    matchLabels:
      app: system-tuner
  template:
    metadata:
      labels:
        app: system-tuner
    spec:
      hostPID: true
      hostIPC: true
      containers:
      - name: system-tuner
        image: {{ .Values.systemTuner.image.repository }}:{{ .Values.systemTuner.image.tag }}
        securityContext:
          privileged: true
        env:
        - name: CHECK_INTERVAL
          value: "30"
        - name: VM_MAX_MAP_COUNT
          value: "262144"
        - name: NOFILE_LIMIT
          value: "131072"

---
# templates/05-system-tuner/wait-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "primus-lens.fullname" . }}-wait-system-tuner
  namespace: {{ .Values.global.namespace }}
  labels:
    {{- include "primus-lens.labels" . | nindent 4 }}
    component: system-tuner-waiter
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "0"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 3
  template:
    spec:
      restartPolicy: Never
      serviceAccountName: {{ include "primus-lens.serviceAccountName" . }}
      containers:
      - name: waiter
        image: bitnami/kubectl:1.28
        command:
        - /bin/bash
        - -c
        - |
          set -ex
          
          echo "Waiting for System Tuner to be ready on all nodes..."
          
          # 等待 System Tuner DaemonSet 就绪
          kubectl rollout status daemonset/{{ include "primus-lens.fullname" . }}-system-tuner \
            -n {{ .Values.global.namespace }} \
            --timeout=180s
          
          echo "✅ System Tuner is ready on all nodes"
          
          # 验证系统参数已设置
          echo "Verifying system parameters..."
          
          # 获取一个 System Tuner Pod 来验证
          POD=$(kubectl get pods -n {{ .Values.global.namespace }} \
            -l app=system-tuner \
            -o jsonpath='{.items[0].metadata.name}')
          
          # 检查日志确认参数已设置
          kubectl logs -n {{ .Values.global.namespace }} "$POD" --tail=50 | grep -q "vm.max_map_count" || {
            echo "⚠️  Warning: Could not verify vm.max_map_count setting"
          }
          
          echo "✅ System tuning completed successfully"
```

### 4.3 Operator 安装 Hook 示例

```yaml
# templates/10-middleware-operators/postgresql/00-operator-install-job.yaml
{{- if .Values.middleware.postgresql.enabled }}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "primus-lens.fullname" . }}-install-pg-operator
  namespace: {{ .Values.global.namespace }}
  labels:
    {{- include "primus-lens.labels" . | nindent 4 }}
    component: postgresql-operator-installer
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-weight": "0"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 3
  template:
    metadata:
      name: install-pg-operator
    spec:
      restartPolicy: Never
      serviceAccountName: {{ include "primus-lens.serviceAccountName" . }}
      containers:
      - name: installer
        image: alpine/helm:3.12.0
        command:
        - /bin/sh
        - -c
        - |
          set -ex
          
          # 添加 Helm Repo
          helm repo add postgres-operator-examples https://github.com/CrunchyData/postgres-operator-examples.git
          helm repo update
          
          # 检查是否已安装
          if helm list -n {{ .Values.global.namespace }} | grep -q pg-operator; then
            echo "PostgreSQL Operator already installed, skipping..."
            exit 0
          fi
          
          # 安装 Operator
          helm upgrade --install pg-operator \
            postgres-operator-examples/helm/install \
            --namespace {{ .Values.global.namespace }} \
            --wait \
            --timeout 5m \
            {{- with .Values.middleware.postgresql.operator }}
            {{- if .resources }}
            --set resources.requests.cpu={{ .resources.requests.cpu }} \
            --set resources.requests.memory={{ .resources.requests.memory }} \
            {{- end }}
            {{- end }}
          
          echo "PostgreSQL Operator installed successfully"
{{- end }}

---
# templates/10-middleware-operators/postgresql/01-wait-operator-job.yaml
{{- if .Values.middleware.postgresql.enabled }}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "primus-lens.fullname" . }}-wait-pg-operator
  namespace: {{ .Values.global.namespace }}
  labels:
    {{- include "primus-lens.labels" . | nindent 4 }}
    component: postgresql-operator-waiter
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-weight": "20"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 5
  template:
    spec:
      restartPolicy: Never
      serviceAccountName: {{ include "primus-lens.serviceAccountName" . }}
      containers:
      - name: waiter
        image: bitnami/kubectl:1.28
        command:
        - /bin/bash
        - -c
        - |
          set -ex
          
          echo "Waiting for PostgreSQL Operator to be ready..."
          
          # 等待 Operator Pod 运行
          kubectl wait --for=condition=Ready pod \
            -l app.kubernetes.io/name=pgo \
            -n {{ .Values.global.namespace }} \
            --timeout=300s
          
          # 额外等待 CRDs 注册完成
          for i in {1..30}; do
            if kubectl get crd postgresclusters.postgres-operator.crunchydata.com >/dev/null 2>&1; then
              echo "PostgreSQL Operator CRDs are ready"
              exit 0
            fi
            echo "Waiting for PostgreSQL Operator CRDs... ($i/30)"
            sleep 5
          done
          
          echo "Timeout waiting for PostgreSQL Operator CRDs"
          exit 1
{{- end }}
```

## 5. 等待逻辑设计

### 5.1 等待策略

```yaml
# 等待逻辑分层设计
┌─────────────────────────────────────────────────────────────┐
│ 层级 1: Operator 就绪等待                                    │
│ ├── 方式: kubectl wait --for=condition=Ready pod            │
│ ├── 超时: 300s (5分钟)                                       │
│ └── 验证: CRD 是否注册成功                                   │
├─────────────────────────────────────────────────────────────┤
│ 层级 2: 中间件实例就绪等待                                   │
│ ├── PostgreSQL:                                             │
│ │   ├── kubectl wait endpoints (有 IP)                      │
│ │   ├── kubectl wait pod (Running)                          │
│ │   └── psql 连接测试                                        │
│ ├── OpenSearch:                                             │
│ │   ├── kubectl wait pod (Running)                          │
│ │   └── curl /_cluster/health (status: green/yellow)        │
│ └── VictoriaMetrics:                                        │
│     ├── kubectl wait pod (vmselect/vminsert/vmstorage)      │
│     └── curl /health (HTTP 200)                             │
├─────────────────────────────────────────────────────────────┤
│ 层级 3: 应用组件就绪等待                                     │
│ ├── 方式: kubectl rollout status deployment                 │
│ ├── 超时: 180s (3分钟)                                       │
│ └── 验证: HTTP /healthz 端点                                │
└─────────────────────────────────────────────────────────────┘
```

### 5.2 通用等待脚本

```bash
# files/scripts/wait-for-ready.sh
#!/bin/bash
# 通用的资源就绪等待脚本

set -euo pipefail

RESOURCE_TYPE="$1"  # pod, deployment, endpoints, etc.
RESOURCE_NAME="$2"
NAMESPACE="${3:-default}"
TIMEOUT="${4:-300}"

echo "Waiting for $RESOURCE_TYPE/$RESOURCE_NAME in namespace $NAMESPACE..."

case "$RESOURCE_TYPE" in
  pod)
    kubectl wait --for=condition=Ready pod \
      -l "$RESOURCE_NAME" \
      -n "$NAMESPACE" \
      --timeout="${TIMEOUT}s"
    ;;
  
  deployment)
    kubectl rollout status deployment/"$RESOURCE_NAME" \
      -n "$NAMESPACE" \
      --timeout="${TIMEOUT}s"
    ;;
  
  endpoints)
    for i in $(seq 1 60); do
      IP=$(kubectl get endpoints "$RESOURCE_NAME" -n "$NAMESPACE" \
        -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || echo "")
      if [[ -n "$IP" ]]; then
        echo "✅ Endpoints $RESOURCE_NAME has IP: $IP"
        exit 0
      fi
      echo "⏳ [$i/60] Waiting for endpoints $RESOURCE_NAME..."
      sleep 5
    done
    echo "❌ Timeout waiting for endpoints"
    exit 1
    ;;
  
  statefulset)
    kubectl rollout status statefulset/"$RESOURCE_NAME" \
      -n "$NAMESPACE" \
      --timeout="${TIMEOUT}s"
    ;;
  
  *)
    echo "Unknown resource type: $RESOURCE_TYPE"
    exit 1
    ;;
esac

echo "✅ $RESOURCE_TYPE/$RESOURCE_NAME is ready"
```

### 5.3 中间件特定等待 Job

```yaml
# templates/20-middleware-instances/postgresql/01-wait-job.yaml
{{- if .Values.middleware.postgresql.enabled }}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "primus-lens.fullname" . }}-wait-postgresql
  namespace: {{ .Values.global.namespace }}
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-weight": "110"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 5
  template:
    spec:
      restartPolicy: Never
      serviceAccountName: {{ include "primus-lens.serviceAccountName" . }}
      containers:
      - name: waiter
        image: bitnami/kubectl:1.28
        command:
        - /bin/bash
        - -c
        - |
          set -ex
          
          SERVICE_NAME="primus-lens-ha"
          NAMESPACE="{{ .Values.global.namespace }}"
          
          echo "Waiting for PostgreSQL endpoints..."
          for i in {1..60}; do
            IP=$(kubectl get endpoints "$SERVICE_NAME" -n "$NAMESPACE" \
              -o jsonpath="{.subsets[0].addresses[0].ip}" 2>/dev/null || echo "")
            if [[ -n "$IP" ]]; then
              echo "✅ PostgreSQL endpoint IP: $IP"
              break
            fi
            echo "⏳ [$i/60] Waiting for PostgreSQL endpoint..."
            sleep 5
          done
          
          if [[ -z "$IP" ]]; then
            echo "❌ Timeout waiting for PostgreSQL endpoint"
            exit 1
          fi
          
          echo "Waiting for PostgreSQL pod to be ready..."
          POD_NAME=$(kubectl get pods -n "$NAMESPACE" -o wide | grep "$IP" | awk '{print $1}')
          echo "PostgreSQL Pod: $POD_NAME"
          
          kubectl wait --for=condition=Ready pod/"$POD_NAME" \
            -n "$NAMESPACE" \
            --timeout=300s
          
          echo "✅ PostgreSQL is ready"
{{- end }}
```

## 6. 动态配置生成

### 6.1 配置生成策略

```yaml
# 配置生成方式分类
┌────────────────────────────────────────────────────────────┐
│ 类型 1: 静态配置（通过 values.yaml）                        │
│ ├── 适用: 集群名、镜像 tag、资源配置等                      │
│ ├── 方式: 直接在 templates 中使用 Go template 渲染         │
│ └── 示例: {{ .Values.global.clusterName }}                 │
├────────────────────────────────────────────────────────────┤
│ 类型 2: 动态提取配置（Operator 生成的 Secret）              │
│ ├── 适用: PostgreSQL 密码、OpenSearch 密码等               │
│ ├── 方式: Hook Job 提取 Secret → 写入 ConfigMap            │
│ └── 组件通过 envFrom 引用 ConfigMap                         │
├────────────────────────────────────────────────────────────┤
│ 类型 3: 计算配置（基于 profile）                            │
│ ├── 适用: 资源 requests/limits、副本数、存储大小            │
│ ├── 方式: _helpers.tpl 中定义函数，根据 profile 计算       │
│ └── 示例: {{ include "primus-lens.postgresql.memory" . }}  │
├────────────────────────────────────────────────────────────┤
│ 类型 4: 条件配置（基于部署模式）                            │
│ ├── 适用: 管理集群 vs 数据集群的组件启用                    │
│ ├── 方式: {{- if }} 条件判断                               │
│ └── 示例: {{- if .Values.management.enabled }}             │
└────────────────────────────────────────────────────────────┘
```

### 6.2 动态密码提取

```yaml
# templates/20-middleware-instances/postgresql/03-password-extract-job.yaml
{{- if .Values.middleware.postgresql.enabled }}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "primus-lens.fullname" . }}-extract-pg-password
  namespace: {{ .Values.global.namespace }}
  annotations:
    "helm.sh/hook": pre-install
    "helm.sh/hook-weight": "130"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  template:
    spec:
      restartPolicy: Never
      serviceAccountName: {{ include "primus-lens.serviceAccountName" . }}
      containers:
      - name: extractor
        image: bitnami/kubectl:1.28
        command:
        - /bin/bash
        - -c
        - |
          set -ex
          
          NAMESPACE="{{ .Values.global.namespace }}"
          SECRET_NAME="primus-lens-pguser-primus-lens"
          CONFIGMAP_NAME="primus-lens-middleware-config"
          
          echo "Extracting PostgreSQL password from secret..."
          
          # 等待 Secret 创建
          for i in {1..30}; do
            if kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" >/dev/null 2>&1; then
              echo "✅ Secret found"
              break
            fi
            echo "⏳ [$i/30] Waiting for secret $SECRET_NAME..."
            sleep 5
          done
          
          # 提取密码
          PG_PASSWORD=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" \
            -o jsonpath="{.data.password}" | base64 -d)
          
          PG_HOST=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" \
            -o jsonpath="{.data.host}" | base64 -d)
          
          PG_PORT=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" \
            -o jsonpath="{.data.port}" | base64 -d)
          
          PG_DATABASE=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" \
            -o jsonpath="{.data.dbname}" | base64 -d)
          
          PG_USER=$(kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" \
            -o jsonpath="{.data.user}" | base64 -d)
          
          # 创建或更新 ConfigMap
          kubectl create configmap "$CONFIGMAP_NAME" \
            --from-literal=pg_host="$PG_HOST" \
            --from-literal=pg_port="$PG_PORT" \
            --from-literal=pg_database="$PG_DATABASE" \
            --from-literal=pg_user="$PG_USER" \
            --from-literal=pg_password="$PG_PASSWORD" \
            --namespace "$NAMESPACE" \
            --dry-run=client -o yaml | kubectl apply -f -
          
          echo "✅ PostgreSQL configuration saved to ConfigMap: $CONFIGMAP_NAME"
{{- end }}
```

### 6.3 配置引用示例

```yaml
# templates/30-management-components/api/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "primus-lens.fullname" . }}-api
spec:
  template:
    spec:
      containers:
      - name: api
        image: {{ include "primus-lens.image" (dict "component" "api" "root" .) }}
        env:
        # 静态配置
        - name: CLUSTER_NAME
          value: {{ .Values.global.clusterName | quote }}
        - name: DEPLOYMENT_MODE
          value: "management"
        
        # 从动态生成的 ConfigMap 引用
        - name: DB_HOST
          valueFrom:
            configMapKeyRef:
              name: primus-lens-middleware-config
              key: pg_host
        - name: DB_PORT
          valueFrom:
            configMapKeyRef:
              name: primus-lens-middleware-config
              key: pg_port
        - name: DB_NAME
          valueFrom:
            configMapKeyRef:
              name: primus-lens-middleware-config
              key: pg_database
        - name: DB_USER
          valueFrom:
            configMapKeyRef:
              name: primus-lens-middleware-config
              key: pg_user
        - name: DB_PASSWORD
          valueFrom:
            configMapKeyRef:
              name: primus-lens-middleware-config
              key: pg_password
        
        # 或者使用 envFrom 批量引用
        envFrom:
        - configMapRef:
            name: {{ include "primus-lens.fullname" . }}-api-config
        - configMapRef:
            name: primus-lens-middleware-config
            prefix: MIDDLEWARE_
```

## 7. _helpers.tpl 辅助函数

```yaml
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
{{- if (include "primus-lens.middleware.enabled" .) }}
{{- /* 本地中间件 */ -}}
postgresql://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable
{{- else }}
{{- /* 远程中间件 */ -}}
{{- with .Values.middleware.remote.postgresql }}
postgresql://$(DB_USER):$(DB_PASSWORD)@{{ .host }}:{{ .port }}/{{ .database }}?sslmode=disable
{{- end }}
{{- end }}
{{- end }}
```

## 8. 条件渲染逻辑

### 8.1 部署模式条件渲染

```yaml
# templates/30-management-components/api/deployment.yaml
{{- if or (eq .Values.deploymentMode "management") (eq .Values.deploymentMode "all-in-one") }}
{{- if .Values.management.api.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "primus-lens.fullname" . }}-api
  # ... rest of the template
{{- end }}
{{- end }}

# templates/40-data-components/node-exporter/daemonset.yaml
{{- if or (eq .Values.deploymentMode "data") (eq .Values.deploymentMode "all-in-one") }}
{{- if .Values.data.nodeExporter.enabled }}
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {{ include "primus-lens.fullname" . }}-node-exporter
  # ... rest of the template
{{- end }}
{{- end }}
```

### 8.2 中间件条件渲染

```yaml
# templates/20-middleware-instances/postgresql/00-postgres-cluster.yaml
{{- if (include "primus-lens.middleware.enabled" .) }}
{{- if .Values.middleware.postgresql.enabled }}
apiVersion: postgres-operator.crunchydata.com/v1beta1
kind: PostgresCluster
metadata:
  name: primus-lens
  # ... rest of the template
{{- end }}
{{- end }}
```

### 8.3 远程中间件配置

```yaml
# templates/30-management-components/_remote-middleware-config.yaml
{{- if not (include "primus-lens.middleware.enabled" .) }}
{{- /* 如果不部署本地中间件，创建远程中间件配置 */ -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: primus-lens-middleware-config
  namespace: {{ .Values.global.namespace }}
data:
  pg_host: {{ .Values.middleware.remote.postgresql.host | quote }}
  pg_port: {{ .Values.middleware.remote.postgresql.port | quote }}
  pg_database: {{ .Values.middleware.remote.postgresql.database | quote }}
  pg_user: {{ .Values.middleware.remote.postgresql.user | default "primus_lens" | quote }}
  
  opensearch_host: {{ .Values.middleware.remote.opensearch.host | quote }}
  opensearch_port: {{ .Values.middleware.remote.opensearch.port | quote }}
  
  victoriametrics_host: {{ .Values.middleware.remote.victoriametrics.host | quote }}
  victoriametrics_port: {{ .Values.middleware.remote.victoriametrics.port | quote }}
  
  otel_collector_endpoint: {{ .Values.middleware.remote.otelCollector.endpoint | quote }}

---
{{- /* 如果提供了远程数据库密码 Secret，创建引用 */ -}}
{{- if .Values.middleware.remote.postgresql.existingSecret }}
apiVersion: v1
kind: Secret
metadata:
  name: primus-lens-remote-db-password
  namespace: {{ .Values.global.namespace }}
type: Opaque
data:
  password: {{ .Values.middleware.remote.postgresql.password | b64enc | quote }}
{{- end }}
{{- end }}
```

## 9. NOTES.txt 设计

```yaml
# templates/NOTES.txt
Thank you for installing {{ .Chart.Name }}.

Your release is named {{ .Release.Name }}.

Deployment Mode: {{ .Values.deploymentMode }}
Cluster Name: {{ .Values.global.clusterName }}
Namespace: {{ .Values.global.namespace }}

{{- if or (eq .Values.deploymentMode "management") (eq .Values.deploymentMode "all-in-one") }}

=======================================================
  MANAGEMENT CLUSTER COMPONENTS
=======================================================

{{ if .Values.management.api.enabled }}
✓ Primus Lens API
  - Service: {{ include "primus-lens.fullname" . }}-api
  - Port: {{ .Values.management.api.service.port }}
  - Access: kubectl port-forward svc/{{ include "primus-lens.fullname" . }}-api {{ .Values.management.api.service.port }}:{{ .Values.management.api.service.port }} -n {{ .Values.global.namespace }}
{{- end }}

{{ if .Values.management.grafana.enabled }}
✓ Grafana Dashboard
{{- if eq .Values.management.grafana.accessType "ssh-tunnel" }}
  - Access via SSH Tunnel:
    kubectl port-forward svc/{{ include "primus-lens.fullname" . }}-nginx 30182:80 -n {{ .Values.global.namespace }}
    URL: http://127.0.0.1:30182/grafana
{{- else if eq .Values.management.grafana.accessType "ingress" }}
  - Access via Ingress:
    URL: http://{{ .Values.management.grafana.domain }}/grafana
{{- end }}
  - Default Credentials: admin / admin (please change on first login)
{{- end }}

{{- end }}

{{- if or (eq .Values.deploymentMode "data") (eq .Values.deploymentMode "all-in-one") }}

=======================================================
  DATA CLUSTER COMPONENTS
=======================================================

{{ if .Values.data.nodeExporter.enabled }}
✓ Node Exporter (DaemonSet)
  - Collecting GPU, RDMA, and container metrics from all nodes
{{- end }}

{{ if .Values.data.gpuResourceExporter.enabled }}
✓ GPU Resource Exporter
  - Tracking GPU pod lifecycle and workload management
{{- end }}

{{- end }}

{{- if (include "primus-lens.middleware.enabled" .) }}

=======================================================
  MIDDLEWARE COMPONENTS
=======================================================

{{ if .Values.middleware.postgresql.enabled }}
✓ PostgreSQL
  - Cluster: primus-lens
  - Service: primus-lens-ha
  - Database: primus_lens
  - Connection info stored in ConfigMap: primus-lens-middleware-config
{{- end }}

{{ if .Values.middleware.opensearch.enabled }}
✓ OpenSearch
  - Cluster: primus-lens-opensearch
  - Dashboard: kubectl port-forward svc/primus-lens-opensearch-dashboards 5601:5601 -n {{ .Values.global.namespace }}
{{- end }}

{{ if .Values.middleware.victoriametrics.enabled }}
✓ VictoriaMetrics
  - VMCluster: primus-lens-vm
  - VMSelect: http://primus-lens-vm-vmselect:8481/select/0/prometheus
  - VMInsert: http://primus-lens-vm-vminsert:8480/insert/0/prometheus
{{- end }}

{{- end }}

=======================================================
  VERIFICATION
=======================================================

Check deployment status:
  kubectl get pods -n {{ .Values.global.namespace }}

Check services:
  kubectl get svc -n {{ .Values.global.namespace }}

View logs:
  kubectl logs -n {{ .Values.global.namespace }} -l app.kubernetes.io/instance={{ .Release.Name }}

=======================================================
  NEXT STEPS
=======================================================

1. Verify all pods are running
2. Access Grafana dashboard to view metrics
3. Configure multi-cluster setup (if needed)
4. Review the documentation: https://github.com/AMD-AGI/Primus-SaFE/tree/main/Lens

For more information, visit:
  https://github.com/AMD-AGI/Primus-SaFE

{{- if .Values.troubleshooting }}
=======================================================
  TROUBLESHOOTING
=======================================================

If you encounter issues:
1. Check Hook Jobs: kubectl get jobs -n {{ .Values.global.namespace }}
2. Check Hook Logs: kubectl logs -n {{ .Values.global.namespace }} job/<job-name>
3. Verify Operators: kubectl get pods -n {{ .Values.global.namespace }} -l app.kubernetes.io/component=operator
4. Check CRDs: kubectl get crd | grep -E 'postgres|opensearch|victoriametrics|grafana'

{{- end }}
```

## 10. Chart.yaml 设计

```yaml
# Chart.yaml
apiVersion: v2
name: primus-lens
description: A comprehensive Kubernetes GPU cluster monitoring and management platform
type: application
version: 1.0.0
appVersion: "1.0.0"
kubeVersion: ">=1.23.0-0"

keywords:
  - gpu
  - monitoring
  - observability
  - amd
  - kubernetes
  - ml
  - ai

home: https://github.com/AMD-AGI/Primus-SaFE
sources:
  - https://github.com/AMD-AGI/Primus-SaFE/tree/main/Lens

maintainers:
  - name: AMD-AGI Team
    email: support@amd-agi.com

icon: https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/docs/images/primus-lens-logo.png

# 可选：依赖其他 Charts（如果决定使用）
# dependencies:
#   - name: postgresql
#     version: "12.x.x"
#     repository: "https://charts.bitnami.com/bitnami"
#     condition: middleware.postgresql.enabled
#     tags:
#       - middleware
```

## 11. 安装和使用示例

### 11.1 安装命令

```bash
# 1. 添加 Helm Repo（如果发布到公共 Repo）
helm repo add primus-lens https://amd-agi.github.io/primus-safe-helm-charts/
helm repo update

# 2. 管理集群安装
helm install primus-lens primus-lens/primus-lens \
  --namespace primus-lens \
  --create-namespace \
  --set deploymentMode=management \
  --set global.clusterName=management-cluster \
  --set global.profile=normal \
  --wait \
  --timeout 30m

# 3. 数据集群安装（连接到远程中间件）
helm install primus-lens primus-lens/primus-lens \
  --namespace primus-lens \
  --create-namespace \
  --set deploymentMode=data \
  --set global.clusterName=data-cluster-1 \
  --set middleware.enabled=false \
  --set middleware.remote.postgresql.host=management-cluster.example.com \
  --set middleware.remote.postgresql.port=5432 \
  --wait \
  --timeout 20m

# 4. 一体化安装
helm install primus-lens primus-lens/primus-lens \
  --namespace primus-lens \
  --create-namespace \
  -f examples/values-all-in-one.yaml \
  --wait \
  --timeout 30m

# 5. 使用自定义 values 文件
helm install primus-lens ./primus-lens \
  --namespace primus-lens \
  --create-namespace \
  -f my-custom-values.yaml \
  --wait
```

### 11.2 升级命令

```bash
# 升级到新版本
helm upgrade primus-lens primus-lens/primus-lens \
  --namespace primus-lens \
  -f my-custom-values.yaml \
  --wait

# 查看升级历史
helm history primus-lens -n primus-lens

# 回滚
helm rollback primus-lens 1 -n primus-lens
```

### 11.3 卸载命令

```bash
# 卸载 Release
helm uninstall primus-lens -n primus-lens

# 清理 PVCs（如果需要）
kubectl delete pvc -n primus-lens -l app.kubernetes.io/instance=primus-lens
```

## 12. 测试策略

### 12.1 Chart 测试

```yaml
# templates/tests/test-api-health.yaml
apiVersion: v1
kind: Pod
metadata:
  name: {{ include "primus-lens.fullname" . }}-test-api
  namespace: {{ .Values.global.namespace }}
  annotations:
    "helm.sh/hook": test
spec:
  restartPolicy: Never
  containers:
  - name: test
    image: curlimages/curl:latest
    command:
    - /bin/sh
    - -c
    - |
      set -ex
      API_URL="http://{{ include "primus-lens.fullname" . }}-api:{{ .Values.management.api.service.port }}"
      curl -f "$API_URL/healthz" || exit 1
      echo "✅ API health check passed"

---
# templates/tests/test-middleware.yaml
apiVersion: v1
kind: Pod
metadata:
  name: {{ include "primus-lens.fullname" . }}-test-middleware
  namespace: {{ .Values.global.namespace }}
  annotations:
    "helm.sh/hook": test
spec:
  restartPolicy: Never
  containers:
  - name: test
    image: postgres:15
    command:
    - /bin/bash
    - -c
    - |
      set -ex
      psql "postgresql://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)" \
        -c "SELECT 1;" || exit 1
      echo "✅ PostgreSQL connection test passed"
    envFrom:
    - configMapRef:
        name: primus-lens-middleware-config
```

### 12.2 运行测试

```bash
# 运行 Helm 测试
helm test primus-lens -n primus-lens

# 查看测试结果
kubectl logs -n primus-lens primus-lens-test-api
kubectl logs -n primus-lens primus-lens-test-middleware
```

## 13. 迁移路径

### 13.1 从现有脚本迁移到 Helm

```
阶段 1: 准备阶段（1-2 天）
├── 创建 Chart 基础结构
├── 编写 Chart.yaml 和 values.yaml
├── 迁移简单的 manifest（namespace, sa, cert）
└── 迁移 System Tuner DaemonSet

阶段 2: 中间件迁移（2-3 天）
├── 实现 Operator 安装 Hook Jobs
├── 迁移中间件 CR templates
├── 实现等待逻辑
└── 实现动态配置提取

阶段 3: 应用组件迁移（2-3 天）
├── 迁移管理集群组件
├── 迁移数据集群组件
├── 实现条件渲染逻辑
└── 配置 envFrom 和 ConfigMaps

阶段 4: 可观测性组件（1-2 天）
├── 迁移 Grafana 配置
├── 迁移 Dashboard 定义
├── 实现 FluentBit 配置
└── 实现 VMScrape 配置

阶段 5: 测试和文档（2-3 天）
├── 编写 Helm 测试
├── 三种部署模式测试
├── 编写 NOTES.txt
└── 编写用户文档

总计：8-13 天
```

## 14. 最佳实践总结

1. **使用 Helm Hooks** 控制安装顺序，而不是依赖目录命名
2. **Job 的幂等性**：所有 Hook Jobs 应该是幂等的，支持重复执行
3. **失败处理**：设置合理的 `backoffLimit` 和超时时间
4. **清理策略**：使用 `hook-delete-policy` 自动清理成功的 Hook Jobs
5. **配置验证**：使用 `values.schema.json` 验证用户输入
6. **条件渲染**：使用 `{{- if }}` 而不是创建多个 Chart
7. **辅助函数**：在 `_helpers.tpl` 中封装复杂逻辑
8. **文档完善**：在 `NOTES.txt` 中提供清晰的使用指南
9. **测试覆盖**：为关键组件编写 Helm 测试
10. **版本控制**：Chart 版本和应用版本分开管理

## 15. 附录

### 15.1 常用 Helm 命令

```bash
# Dry-run 检查生成的 manifests
helm install primus-lens ./primus-lens --dry-run --debug

# 只渲染模板（不安装）
helm template primus-lens ./primus-lens -f values.yaml

# 查看已安装的 Release
helm list -n primus-lens

# 查看 Release 详情
helm get all primus-lens -n primus-lens

# 查看 values
helm get values primus-lens -n primus-lens

# 查看生成的 manifests
helm get manifest primus-lens -n primus-lens

# 验证 Chart
helm lint ./primus-lens

# 打包 Chart
helm package ./primus-lens
```

### 15.2 调试技巧

```bash
# 查看 Hook Jobs
kubectl get jobs -n primus-lens -l helm.sh/chart=primus-lens

# 查看 Hook Job 日志
kubectl logs -n primus-lens job/primus-lens-install-pg-operator

# 查看失败的 Pods
kubectl get pods -n primus-lens --field-selector=status.phase=Failed

# 删除失败的 Hook Jobs 重新安装
kubectl delete jobs -n primus-lens -l helm.sh/chart=primus-lens
helm upgrade --install primus-lens ./primus-lens -f values.yaml
```

### 15.3 参考资源

- Helm 官方文档: https://helm.sh/docs/
- Helm Hooks: https://helm.sh/docs/topics/charts_hooks/
- Go Templates: https://pkg.go.dev/text/template
- Chart Best Practices: https://helm.sh/docs/chart_best_practices/

