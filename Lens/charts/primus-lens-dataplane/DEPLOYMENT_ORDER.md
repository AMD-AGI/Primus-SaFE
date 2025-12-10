# Primus Lens 部署顺序说明

详细的部署阶段和资源创建顺序。

## 📋 完整部署流程

```
用户执行: helm install primus-lens ./charts --timeout 30m --wait

┌─────────────────────────────────────────────────────────────────┐
│ Phase 0: Pre-Install Hooks (weight: -100 到 -90)               │
│ 目的: 准备部署环境                                              │
└─────────────────────────────────────────────────────────────────┘
  ├── Weight -100: Namespace
  │   └── 创建 primus-lens namespace
  │
  └── Weight -90: Secrets & RBAC
      ├── 创建 image-pull-secret (支持空占位符)
      ├── 创建 tls-cert-secret (Webhook 证书占位符)
      ├── 创建 ServiceAccount: primus-lens-installer
      ├── 创建 ServiceAccount: primus-lens-app
      ├── 创建 ClusterRole & ClusterRoleBinding
      └── ✓ RBAC 权限配置完成

┌─────────────────────────────────────────────────────────────────┐
│ Phase 1: Operators 部署 (子 Charts 自动处理)                   │
│ 目的: 安装所有 CRD Operators                                    │
└─────────────────────────────────────────────────────────────────┘
  Helm 自动部署 6 个 Operator Charts:
  ├── victoria-metrics-operator (v0.35.2)
  │   └── 管理 VMCluster, VMAgent, VMServiceScrape 等
  │
  ├── fluent-operator (v3.1.0)
  │   └── 管理 FluentBit, ClusterFluentBitConfig 等
  │
  ├── opensearch-operator (v2.6.0)
  │   └── 管理 OpenSearchCluster
  │
  ├── pgo - PostgreSQL Operator (v5.7.0)
  │   └── 管理 PostgresCluster
  │
  ├── grafana-operator (v5.15.0)
  │   └── 管理 Grafana, GrafanaDatasource, GrafanaDashboard 等
  │
  └── kube-state-metrics (v5.27.0)
      └── 导出 Kubernetes 资源指标

┌─────────────────────────────────────────────────────────────────┐
│ Phase 2: Wait for Operators (pre-install hook, weight: 0)      │
│ 目的: 确保所有 Operators 就绪后再创建 CR                        │
└─────────────────────────────────────────────────────────────────┘
  Job: primus-lens-wait-operators
  ├── 使用 kubectl wait --for=condition=ready pod 等待
  ├── 检查所有 Operator Pods 状态
  ├── 最多重试 30 次 (约 15 分钟)
  └── ✓ 所有 Operators 就绪

┌─────────────────────────────────────────────────────────────────┐
│ Phase 3: 基础设施 CR 部署 (正常资源，无 hook)                  │
│ 目的: 创建数据库、日志存储、指标存储                           │
│ ⚠️  这些必须在 init-jobs 之前部署，以便初始化作业可以连接      │
└─────────────────────────────────────────────────────────────────┘
  并行部署以下 Custom Resources:
  
  ├── PostgresCluster: primus-lens
  │   ├── 实例数: 根据 profile (1/2/3)
  │   ├── 数据存储: 根据 profile (20Gi/50Gi/100Gi)
  │   ├── 备份存储: 根据 profile (10Gi/20Gi/50Gi)
  │   ├── PGO 创建 Pods: primus-lens-instance1-xxxx
  │   └── PGO 创建 Service: primus-lens-ha, primus-lens-primary
  │
  ├── OpenSearchCluster: primus-lens-logs
  │   ├── 节点数: 根据 nodeSets 配置 (默认 3)
  │   ├── 角色: master, data, ingest
  │   ├── 存储: 根据 profile (30Gi/50Gi/100Gi)
  │   ├── OpenSearch Operator 创建 Pods
  │   └── 创建 Service: primus-lens-logs-nodes
  │
  └── VMCluster: primus-lens-vmcluster
      ├── VMStorage: 根据 profile (1/2/3 replicas)
      │   └── 存储: 根据 profile (30Gi/50Gi/100Gi)
      ├── VMSelect: 根据 profile (1/2/3 replicas)
      ├── VMInsert: 根据 profile (1/2/3 replicas)
      ├── VictoriaMetrics Operator 创建 StatefulSets
      └── 创建 Services: vmselect-*, vminsert-*, vmstorage-*

┌─────────────────────────────────────────────────────────────────┐
│ Phase 4: Wait for Infrastructure (post-install hook, weight: 5)│
│ 目的: 等待基础设施 CR 对应的 Pods 就绪                         │
│ ⚠️  必须等待这些就绪后才能初始化数据库                          │
└─────────────────────────────────────────────────────────────────┘
  Job: primus-lens-wait-infrastructure
  
  等待以下资源就绪:
  ├── PostgreSQL Cluster
  │   ├── 检查 PostgresCluster CR 存在
  │   ├── 等待 PostgreSQL Pods Running
  │   ├── 标签: postgres-operator.crunchydata.com/cluster=primus-lens
  │   └── ✓ 至少 1 个 Pod Running (约 5-10 分钟)
  │
  ├── OpenSearch Cluster
  │   ├── 检查 OpenSearchCluster CR 存在
  │   ├── 等待 OpenSearch Pods Running
  │   ├── 标签: opensearch.cluster.name=primus-lens-logs
  │   └── ✓ 至少 1 个 Pod Running (约 5-10 分钟)
  │
  └── VictoriaMetrics Cluster
      ├── 检查 VMCluster CR 存在
      ├── 等待 VMStorage Pods Running
      ├── 等待 VMSelect Pods Running
      ├── 等待 VMInsert Pods Running
      └── ✓ 所有组件 Running (约 3-5 分钟)
  
  最多重试: 60 次 (约 30 分钟)

┌─────────────────────────────────────────────────────────────────┐
│ Phase 5: Database Initialization (post-install hook, weight: 10)│
│ 目的: 初始化 PostgreSQL 数据库模式                             │
└─────────────────────────────────────────────────────────────────┘
  Job: primus-lens-postgres-init
  
  ├── initContainer: wait-postgres
  │   ├── 使用 pg_isready 检查连接
  │   ├── 目标: primus-lens-ha.primus-lens.svc.cluster.local:5432
  │   └── ✓ PostgreSQL 可连接
  │
  └── container: init-db
      ├── 连接数据库 (用户: postgres)
      ├── 执行 SQL 脚本: files/setup_primus_lens.sql
      │   ├── 创建数据库: primus-lens
      │   ├── 创建用户: primus-lens
      │   ├── 创建所有表 (node, gpu_device, workload, etc.)
      │   ├── 创建索引
      │   └── 授予权限
      └── ✓ 数据库初始化完成

┌─────────────────────────────────────────────────────────────────┐
│ Phase 6: 应用组件部署 (正常资源，无 hook)                      │
│ 目的: 部署 Primus Lens 核心应用                                │
└─────────────────────────────────────────────────────────────────┘
  并行部署以下应用组件:
  
  ├── Deployment: primus-lens-api
  │   ├── Replicas: 2 (可配置)
  │   ├── 端口: 8080 (HTTP), 9090 (gRPC)
  │   ├── 连接 PostgreSQL (DB_HOST, DB_PASSWORD from secret)
  │   ├── 连接 OpenSearch (OPENSEARCH_ENDPOINT)
  │   ├── 连接 VictoriaMetrics (VMSELECT_ENDPOINT)
  │   └── Service: primus-lens-api (ClusterIP)
  │
  ├── Deployment: primus-lens-telemetry-collector
  │   ├── Replicas: 2 (可配置)
  │   ├── 收集训练日志和指标
  │   ├── 写入 OpenSearch 和 VictoriaMetrics
  │   └── Service: primus-lens-telemetry-collector (ClusterIP)
  │
  ├── Deployment: primus-lens-jobs
  │   ├── Replicas: 2 (可配置)
  │   ├── 任务管理和调度
  │   └── Service: primus-lens-jobs (ClusterIP)
  │
  ├── Deployment: primus-lens-web
  │   ├── Replicas: 2 (可配置)
  │   ├── 端口: 80
  │   ├── 环境变量: API_ENDPOINT, GRAFANA_URL
  │   └── Service: primus-lens-web (NodePort 30180)
  │
  ├── DaemonSet: primus-lens-node-exporter
  │   ├── 每个节点运行 1 个 Pod
  │   ├── hostNetwork: true
  │   ├── 导出节点级别指标
  │   └── Service: primus-lens-node-exporter (Headless)
  │
  ├── DaemonSet: primus-lens-gpu-resource-exporter
  │   ├── 每个 GPU 节点运行 1 个 Pod
  │   ├── 导出 GPU 使用率、温度等指标
  │   └── Service: primus-lens-gpu-resource-exporter (Headless)
  │
  └── DaemonSet: primus-lens-system-tuner
      ├── 系统参数优化
      └── 特权模式运行

┌─────────────────────────────────────────────────────────────────┐
│ Phase 7: 监控组件部署 (post-install hook, weight: 100)         │
│ 目的: 部署依赖应用的监控组件                                    │
│ ⚠️  必须在 telemetry-processor 启动后部署                       │
└─────────────────────────────────────────────────────────────────┘
  ├── FluentBit (日志收集)
  │   ├── FluentBit CR + ConfigMap
  │   ├── DaemonSet: 每个节点运行 1 个
  │   ├── 收集容器日志: /var/log/containers/*.log
  │   ├── Kubernetes 元数据过滤
  │   ├── 输出到 OpenSearch
  │   └── 依赖: telemetry-processor 处理日志
  │
  └── VMAgent (指标收集)
      ├── VMAgent CR
      ├── Replicas: 2
      ├── 自动发现 ServiceScrape, PodScrape
      ├── 抓取所有 Exporters 指标
      ├── 写入 VMInsert endpoint
      └── 依赖: telemetry-processor 处理指标

┌─────────────────────────────────────────────────────────────────┐
│ Phase 8: Grafana 和可视化 (正常资源)                           │
│ 目的: 部署仪表板和数据源                                        │
└─────────────────────────────────────────────────────────────────┘
  ├── Grafana CR: primus-lens-grafana
  │   ├── Replicas: 2
  │   ├── 数据库: PostgreSQL (grafana database)
  │   ├── 端口: 3000
  │   └── Service: grafana-service (NodePort 30182 或 ClusterIP)
  │
  ├── GrafanaDatasource: VictoriaMetrics
  │   ├── Type: prometheus
  │   ├── URL: vmselect service
  │   └── 设为默认数据源
  │
  ├── GrafanaDatasource: PostgreSQL
  │   ├── Type: postgres
  │   └── URL: primus-lens-ha service
  │
  └── GrafanaFolder: Dashboard 文件夹
      ├── Default (通用仪表板)
      ├── Node (节点监控)
      ├── Kubernetes (K8s 监控)
      └── Middleware (中间件监控)

┌─────────────────────────────────────────────────────────────────┐
│ Phase 9: Ingress (可选，正常资源)                              │
│ 目的: 配置外部访问                                              │
└─────────────────────────────────────────────────────────────────┘
  如果 global.accessType == "ingress":
  ├── Ingress: primus-lens-console
  │   ├── Host: <clusterName>.<domain>
  │   ├── Path: / → primus-lens-web:80
  │   └── TLS: 可选 (使用 cert-manager)
  │
  └── Ingress: primus-lens-grafana
      ├── Host: <clusterName>.<domain>
      ├── Path: /grafana → grafana-service:3000
      └── TLS: 可选

┌─────────────────────────────────────────────────────────────────┐
│ 🎉 部署完成！                                                   │
└─────────────────────────────────────────────────────────────────┘
  显示 NOTES.txt:
  ├── 访问信息 (SSH Tunnel 或 Ingress URLs)
  ├── 验证命令
  └── 故障排查提示
```

## 🔑 关键依赖关系

### 1. PostgreSQL 初始化依赖

```
PostgresCluster CR (Phase 3)
    ↓ (等待 Pods Running)
wait-infrastructure Job (Phase 4)
    ↓ (等待完成)
postgres-init Job (Phase 5)
    ↓ (initContainer 等待连接)
PostgreSQL Ready
    ↓ (执行 SQL 脚本)
Database Schema 初始化完成
    ↓
API/Jobs 等应用可以连接数据库
```

### 2. 监控组件依赖

```
telemetry-processor Deployment (Phase 6)
    ↓ (应用启动)
telemetry-processor Service Ready
    ↓
FluentBit + VMAgent 部署 (Phase 7)
    ↓
开始收集日志和指标
```

### 3. Grafana 数据源依赖

```
VictoriaMetrics Cluster (Phase 3)
    ↓
VMSelect Service 可用
    ↓
GrafanaDatasource CR (Phase 8)
    ↓
Grafana 可以查询指标
```

## ⏱️ 预计部署时间

| 阶段 | 预计时间 | 说明 |
|------|---------|------|
| Phase 0-2 | 2-5 分钟 | Operators 部署和就绪 |
| Phase 3 | 并行执行 | CR 创建瞬间完成 |
| Phase 4 | 10-15 分钟 | 等待 PostgreSQL, OpenSearch, VM 就绪 |
| Phase 5 | 1-2 分钟 | 数据库初始化 |
| Phase 6 | 2-5 分钟 | 应用 Pods 启动 |
| Phase 7 | 1-2 分钟 | FluentBit, VMAgent 启动 |
| Phase 8 | 1-2 分钟 | Grafana 启动 |
| **总计** | **17-32 分钟** | 取决于集群性能和镜像拉取速度 |

建议 `--timeout` 设置为 **30m** 或更长。

## 🔍 监控部署进度

### 实时查看所有 Pods

```bash
watch kubectl get pods -n primus-lens
```

### 按阶段查看

```bash
# Phase 1: Operators
kubectl get pods -n primus-lens | grep operator

# Phase 4: 基础设施
kubectl get pods -n primus-lens -l postgres-operator.crunchydata.com/cluster=primus-lens
kubectl get pods -n primus-lens -l opensearch.cluster.name=primus-lens-logs
kubectl get pods -n primus-lens -l app.kubernetes.io/instance=primus-lens-vmcluster

# Phase 5: 初始化 Jobs
kubectl get jobs -n primus-lens
kubectl logs -n primus-lens job/primus-lens-wait-infrastructure
kubectl logs -n primus-lens job/primus-lens-postgres-init

# Phase 6: 应用
kubectl get pods -n primus-lens -l app.kubernetes.io/name=primus-lens

# Phase 7: 监控
kubectl get pods -n primus-lens -l app=fluent-bit
kubectl get pods -n primus-lens -l app.kubernetes.io/name=vmagent
```

## 🚨 常见问题

### Q1: wait-infrastructure Job 超时

**原因**: PostgreSQL/OpenSearch/VictoriaMetrics Pods 未在 30 分钟内启动

**排查**:
```bash
# 检查存储是否可用
kubectl get pvc -n primus-lens

# 检查 PostgreSQL
kubectl describe postgrescluster primus-lens -n primus-lens

# 检查 OpenSearch
kubectl describe opensearchcluster primus-lens-logs -n primus-lens

# 检查 VictoriaMetrics
kubectl describe vmcluster primus-lens-vmcluster -n primus-lens
```

### Q2: postgres-init Job 失败

**原因**: 数据库连接失败或 SQL 脚本错误

**排查**:
```bash
# 查看 Job 日志
kubectl logs -n primus-lens job/primus-lens-postgres-init

# 手动测试连接
kubectl exec -it -n primus-lens \
  $(kubectl get pod -n primus-lens -l postgres-operator.crunchydata.com/role=master -o name | head -1) \
  -- psql -U postgres
```

### Q3: FluentBit/VMAgent 未启动

**原因**: telemetry-processor 应用未就绪

**排查**:
```bash
# 检查 telemetry-processor
kubectl get pods -n primus-lens -l app=primus-lens-telemetry-collector
kubectl logs -n primus-lens -l app=primus-lens-telemetry-collector
```

---

通过这个分阶段的部署流程，确保了正确的依赖顺序，避免了竞态条件！

