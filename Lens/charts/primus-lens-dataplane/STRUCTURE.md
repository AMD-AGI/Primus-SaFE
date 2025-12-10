# Primus Lens Helm Chart - 目录结构

完整的 Chart 目录结构和文件说明。

## 📁 根目录结构

```
Lens/charts/
├── Chart.yaml                    # Chart 元数据和依赖定义
├── values.yaml                   # 默认配置值（3 个 Profile）
├── values-dev.yaml               # 开发环境配置
├── values-prod.yaml              # 生产环境配置
├── .helmignore                   # Helm 打包忽略规则
│
├── README.md                     # 用户文档（安装、配置、使用）
├── QUICKSTART.md                 # 5 分钟快速开始指南
├── DEPLOYMENT_SUMMARY.md         # 部署总结文档
├── STRUCTURE.md                  # 本文档
├── Makefile                      # 便捷命令集合
│
├── charts/                       # 子 Chart 依赖（自动下载）
│   ├── victoria-metrics-operator/
│   ├── fluent-operator/
│   ├── opensearch-operator/
│   ├── pgo/
│   ├── grafana-operator/
│   └── kube-state-metrics/
│
├── files/                        # 静态文件（SQL、配置等）
│   └── setup_primus_lens.sql
│
└── templates/                    # Kubernetes 资源模板
    ├── _helpers.tpl              # 辅助函数库（40+ 函数）
    ├── NOTES.txt                 # 部署后显示的信息
    │
    ├── 00-namespace/             # Phase 0: 命名空间 (pre-install, weight: -100)
    ├── 01-secrets/               # Phase 0: 密钥和 RBAC (pre-install, weight: -90)
    ├── 02-wait-operators/        # Phase 2: 等待 Operators (pre-install, weight: 0)
    ├── 03-infrastructure/        # Phase 3: 基础设施 CR (正常资源)
    ├── 04-wait-infrastructure/   # Phase 4: 等待基础设施 (post-install, weight: 5)
    ├── 05-postgres-init/         # Phase 5: 数据库初始化 (post-install, weight: 10)
    ├── 06-apps/                  # Phase 6: 应用组件 (正常资源)
    ├── 07-monitoring/            # Phase 7: 监控组件 (post-install, weight: 100)
    └── 08-grafana/               # Phase 8: 可视化和入口 (正常资源)
```

## 📂 详细目录说明

### 🔹 核心配置文件

| 文件 | 说明 | 重要度 |
|------|------|--------|
| `Chart.yaml` | Chart 元数据、版本、依赖定义 | ⭐⭐⭐⭐⭐ |
| `values.yaml` | 默认配置，包含 3 个 Profile | ⭐⭐⭐⭐⭐ |
| `values-dev.yaml` | 开发环境覆盖配置 | ⭐⭐⭐⭐ |
| `values-prod.yaml` | 生产环境覆盖配置 | ⭐⭐⭐⭐⭐ |

### 🔹 文档文件

| 文件 | 目标受众 | 内容 |
|------|---------|------|
| `README.md` | 所有用户 | 完整文档：安装、配置、故障排查 |
| `QUICKSTART.md` | 新用户 | 5 分钟快速开始，最小化步骤 |
| `DEPLOYMENT_SUMMARY.md` | 开发者 | 实现总结、架构对比 |
| `STRUCTURE.md` | 开发者 | 本文档，目录结构说明 |

### 🔹 templates/ 目录（按部署阶段）

```
templates/
│
├── _helpers.tpl                          # 辅助函数库
│   ├── primus-lens.namespace             # 获取命名空间
│   ├── primus-lens.profileConfig         # 获取 Profile 配置
│   ├── primus-lens.imagePullSecrets      # 生成镜像拉取密钥
│   ├── primus-lens.grafanaRootUrl        # 生成 Grafana URL
│   ├── primus-lens.dbEnv                 # 数据库环境变量
│   └── ... (40+ 函数)
│
├── NOTES.txt                             # 部署后提示信息
│
├── 00-namespace/                         # Phase 0 (pre-install, weight: -100)
│   └── namespace.yaml                    # 创建命名空间
│
├── 01-secrets/                           # Phase 0 (pre-install, weight: -90)
│   ├── image-pull-secret.yaml           # 镜像拉取密钥
│   ├── tls-cert-secret.yaml             # TLS 证书占位符
│   └── service-account.yaml             # ServiceAccount + RBAC
│       ├── primus-lens-installer         # 用于初始化 Jobs
│       └── primus-lens-app               # 用于应用组件
│
├── 02-wait-operators/                    # Phase 2 (pre-install, weight: 0)
│   └── wait-for-operators-job.yaml      # 等待所有 Operators Ready
│
├── 03-infrastructure/                    # Phase 3 (正常资源)
│   ├── pg-cr.yaml                       # PostgreSQL 集群 CR
│   │   ├── PostgresCluster CR
│   │   ├── 实例配置（replicas, storage）
│   │   ├── 备份配置（pgbackrest）
│   │   └── 监控配置（postgres_exporter）
│   ├── opensearch-cr.yaml               # OpenSearch 集群 CR
│   │   ├── OpenSearchCluster CR
│   │   ├── 节点池配置（master, data, ingest）
│   │   ├── Dashboard 配置
│   │   └── 安全配置（admin password）
│   └── vmcluster.yaml                   # VictoriaMetrics 集群 CR
│       ├── VMStorage (存储层)
│       ├── VMSelect (查询层)
│       └── VMInsert (写入层)
│
├── 04-wait-infrastructure/               # Phase 4 (post-install, weight: 5)
│   └── wait-for-infrastructure-job.yaml # 等待 PG, OS, VM Ready
│
├── 05-postgres-init/                     # Phase 5 (post-install, weight: 10)
│   ├── postgres-init-configmap.yaml     # SQL 脚本 ConfigMap
│   └── postgres-init-job.yaml           # 执行数据库初始化
│
├── 06-apps/                              # Phase 6 (正常资源)
│   ├── app-api.yaml                     # API 服务
│   │   ├── Deployment (2 replicas)
│   │   └── Service (ClusterIP)
│   ├── app-web.yaml                     # Web 控制台
│   │   ├── Deployment (2 replicas)
│   │   └── Service (NodePort 30180)
│   └── app-node-exporter.yaml           # Node Exporter
│       ├── DaemonSet (每节点一个)
│       └── Service (Headless)
│
├── 07-monitoring/                        # Phase 7 (post-install, weight: 100)
│   ├── fluentbit-config.yaml           # FluentBit 配置 + CR
│   │   └── 依赖 telemetry-processor
│   └── vmagent.yaml                     # VMAgent CR
│       └── 依赖 telemetry-processor
│
└── 08-grafana/                           # Phase 8 (正常资源)
    ├── grafana-cr.yaml                  # Grafana 实例
    │   ├── Grafana CR
    │   ├── PostgreSQL 后端存储
    │   └── Service (NodePort 30182)
    ├── datasource.yaml                  # 数据源
    │   ├── GrafanaDatasource: VictoriaMetrics
    │   └── GrafanaDatasource: PostgreSQL
    ├── folders.yaml                     # Dashboard 文件夹
    │   ├── Default
    │   ├── Node
    │   ├── Kubernetes
    │   └── Middleware
    └── nginx-ingress.yaml               # Ingress 配置
        ├── Ingress: Web Console
        └── Ingress: Grafana
```

## 🎯 部署阶段映射

### Phase 0: 前置准备 (pre-install hooks)

| 目录 | Hook Weight | 资源 | 说明 |
|------|-------------|------|------|
| `00-namespace/` | -100 | Namespace | 创建命名空间 |
| `01-secrets/` | -90 | Secret, SA, RBAC | 密钥和权限配置 |

### Phase 1: Operators 部署 (子 Charts)

自动部署在 `charts/` 目录下：
- victoria-metrics-operator
- fluent-operator
- opensearch-operator
- pgo (PostgreSQL Operator)
- grafana-operator
- kube-state-metrics

### Phase 2: 等待 Operators (pre-install hook)

| 目录 | Hook Weight | 资源 | 说明 |
|------|-------------|------|------|
| `02-wait-operators/` | 0 | Job | 等待所有 Operators Ready |

### Phase 3: 基础设施部署 (正常资源)

| 目录 | 资源类型 | 说明 |
|------|---------|------|
| `03-infrastructure/` | PostgresCluster | PostgreSQL 数据库 CR |
| `03-infrastructure/` | OpenSearchCluster | OpenSearch 日志存储 CR |
| `03-infrastructure/` | VMCluster | VictoriaMetrics 指标存储 CR |

### Phase 4: 等待基础设施 (post-install hook)

| 目录 | Hook Weight | 资源 | 说明 |
|------|-------------|------|------|
| `04-wait-infrastructure/` | 5 | Job | 等待 PG, OS, VM Pods Ready |

### Phase 5: 数据库初始化 (post-install hook)

| 目录 | Hook Weight | 资源 | 说明 |
|------|-------------|------|------|
| `05-postgres-init/` | 10 | ConfigMap, Job | 初始化数据库模式 |

### Phase 6: 应用部署 (正常资源)

| 目录 | 资源类型 | 说明 |
|------|---------|------|
| `06-apps/` | Deployment, DaemonSet | 应用组件 (API, Web, Exporters) |

### Phase 7: 监控组件 (post-install hook)

| 目录 | Hook Weight | 资源 | 说明 |
|------|-------------|------|------|
| `07-monitoring/` | 100 | FluentBit CR, VMAgent CR | 日志和指标收集 (依赖应用) |

### Phase 8: 可视化和入口 (正常资源)

| 目录 | 资源类型 | 说明 |
|------|---------|------|
| `08-grafana/` | Grafana CR, Datasource, Folder | 可视化平台 |
| `08-grafana/` | Ingress | 外部访问（可选）|

## 📊 文件数量统计

```
总计文件数: 约 17 个

模板文件 (templates/):
├── 辅助文件: 2 (_helpers.tpl, NOTES.txt)
├── 00-namespace: 1 (namespace.yaml)
├── 01-secrets: 3 (image-pull-secret, tls-cert, service-account)
├── 02-wait-operators: 1 (wait-for-operators-job)
├── 03-infrastructure: 3 (pg-cr, opensearch-cr, vmcluster)
├── 04-wait-infrastructure: 1 (wait-for-infrastructure-job)
├── 05-postgres-init: 2 (configmap, job)
├── 06-apps: 3 (api, web, node-exporter) - 可扩展
├── 07-monitoring: 2 (fluentbit-config, vmagent)
└── 08-grafana: 4 (grafana-cr, datasource, folders, nginx-ingress)

配置文件:
├── Chart 定义: 1 (Chart.yaml)
├── Values 文件: 3 (default, dev, prod)
├── 静态文件: 1 (SQL)
├── 文档: 4 (README, QUICKSTART, SUMMARY, STRUCTURE)
├── 工具: 1 (Makefile)
└── 配置: 1 (.helmignore)
```

## 🔑 核心模板说明

### 1. _helpers.tpl (辅助函数库)

**命名相关**:
- `primus-lens.name`: Chart 名称
- `primus-lens.fullname`: 完整应用名称
- `primus-lens.namespace`: 命名空间
- `primus-lens.labels`: 通用标签
- `primus-lens.selectorLabels`: 选择器标签

**配置相关**:
- `primus-lens.profileConfig`: 获取当前 Profile 配置
- `primus-lens.storageClass`: 存储类名称
- `primus-lens.accessMode`: 访问模式

**网络相关**:
- `primus-lens.useIngress`: 是否使用 Ingress
- `primus-lens.grafanaRootUrl`: Grafana 根 URL
- `primus-lens.postgresHost`: PostgreSQL 主机名
- `primus-lens.opensearchEndpoint`: OpenSearch 端点

**环境变量**:
- `primus-lens.commonEnv`: 通用环境变量
- `primus-lens.dbEnv`: 数据库环境变量

**Hook 权重**:
- `primus-lens.hookWeight.namespace`: -100
- `primus-lens.hookWeight.secrets`: -90
- `primus-lens.hookWeight.waitOperators`: 0
- `primus-lens.hookWeight.postgresInit`: 10

### 2. 关键 CR 模板

| CR 类型 | 文件 | Operator | 说明 |
|---------|------|----------|------|
| PostgresCluster | `05-database/pg-cr.yaml` | PGO | 定义 PG 集群规格 |
| OpenSearchCluster | `06-storage/opensearch-cr.yaml` | OpenSearch Op | 定义 OpenSearch 规格 |
| VMCluster | `04-monitoring/vmcluster.yaml` | VM Op | 定义 VM 集群规格 |
| VMAgent | `04-monitoring/vmagent.yaml` | VM Op | 定义指标采集 |
| Grafana | `07-grafana/grafana-cr.yaml` | Grafana Op | 定义 Grafana 实例 |
| GrafanaDatasource | `07-grafana/datasource.yaml` | Grafana Op | 定义数据源 |

## 🛠️ 常用操作文件

### 查看配置
```bash
# 查看默认配置
cat values.yaml

# 查看开发环境配置
cat values-dev.yaml

# 查看所有可配置项
helm show values .
```

### 渲染模板
```bash
# 渲染所有模板
helm template primus-lens . -f values.yaml

# 渲染特定模板
helm template primus-lens . -f values.yaml -s templates/03-apps/app-api.yaml
```

### 验证
```bash
# 语法检查
helm lint .

# Dry-run
helm install primus-lens . --dry-run --debug
```

## 📝 扩展点

如需添加新组件，按以下模式添加：

### 1. 新增应用组件
在 `templates/03-apps/` 下创建 `app-xxx.yaml`:

```yaml
{{- if .Values.apps.xxx.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-xxx
  namespace: {{ include "primus-lens.namespace" . }}
  labels:
    {{- include "primus-lens.labels" . | nindent 4 }}
    app: primus-lens-xxx
spec:
  # ... 配置
{{- end }}
```

### 2. 新增配置项
在 `values.yaml` 中添加:

```yaml
apps:
  xxx:
    enabled: true
    image: "primuslens/xxx:v1.0.0"
    replicas: 2
```

### 3. 新增初始化 Job
在 `templates/02-init-jobs/` 下创建 `xxx-init-job.yaml`:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  annotations:
    "helm.sh/hook": post-install
    "helm.sh/hook-weight": "20"  # 选择合适的权重
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
# ... 配置
```

## 🔗 相关文档

- [完整用户文档](README.md)
- [快速开始](QUICKSTART.md)
- [部署总结](DEPLOYMENT_SUMMARY.md)
- [架构设计](../bootstrap/HELM_REFACTOR_DESIGN.md)
- [Makefile 命令](Makefile)

---

通过这个结构化的组织方式，Primus Lens Helm Chart 实现了清晰的分层、易于维护和扩展！

