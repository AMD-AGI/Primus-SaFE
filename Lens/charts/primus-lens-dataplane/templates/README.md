# Primus Lens Templates 目录说明

此目录包含所有 Kubernetes 资源模板，按部署 Phase 组织。

## 📋 目录结构

```
templates/
├── _helpers.tpl              # 40+ 辅助函数
├── NOTES.txt                 # 部署后显示的信息
│
├── 00-namespace/             # Phase 0 (pre-install, weight: -100)
│   └── namespace.yaml
│
├── 01-secrets/               # Phase 0 (pre-install, weight: -90)
│   ├── image-pull-secret.yaml
│   ├── tls-cert-secret.yaml
│   └── service-account.yaml
│
├── 02-wait-operators/        # Phase 2 (pre-install, weight: 0)
│   └── wait-for-operators-job.yaml
│
├── 03-infrastructure/        # Phase 3 (正常资源)
│   ├── pg-cr.yaml           # PostgreSQL Cluster CR
│   ├── opensearch-cr.yaml   # OpenSearch Cluster CR
│   └── vmcluster.yaml       # VictoriaMetrics Cluster CR
│
├── 04-wait-infrastructure/   # Phase 4 (post-install, weight: 5)
│   └── wait-for-infrastructure-job.yaml
│
├── 05-postgres-init/         # Phase 5 (post-install, weight: 10)
│   ├── postgres-init-configmap.yaml
│   └── postgres-init-job.yaml
│
├── 06-apps/                  # Phase 6 (正常资源)
│   ├── app-api.yaml
│   ├── app-web.yaml
│   └── app-node-exporter.yaml
│
├── 07-monitoring/            # Phase 7 (post-install, weight: 100)
│   ├── fluentbit-config.yaml
│   └── vmagent.yaml
│
└── 08-grafana/               # Phase 8 (正常资源)
    ├── grafana-cr.yaml
    ├── datasource.yaml
    ├── folders.yaml
    └── nginx-ingress.yaml
```

## 🎯 部署顺序说明

### Phase 0: 前置准备 (Pre-Install Hooks)
**目录**: `00-namespace/`, `01-secrets/`  
**说明**: 创建命名空间、密钥、RBAC，为后续部署做准备

### Phase 1: Operators 部署
**说明**: 由 Helm 子 Charts 自动处理，部署 6 个 Operators

### Phase 2: 等待 Operators
**目录**: `02-wait-operators/`  
**说明**: 等待所有 Operator Pods Ready

### Phase 3: 基础设施 CR
**目录**: `03-infrastructure/`  
**说明**: 部署 PostgreSQL, OpenSearch, VictoriaMetrics 的 Custom Resources

### Phase 4: 等待基础设施
**目录**: `04-wait-infrastructure/`  
**说明**: 等待基础设施对应的 Pods Running

### Phase 5: 数据库初始化
**目录**: `05-postgres-init/`  
**说明**: 执行 SQL 脚本初始化数据库模式

### Phase 6: 应用组件
**目录**: `06-apps/`  
**说明**: 部署 API、Web 控制台、Exporters 等应用

### Phase 7: 监控组件
**目录**: `07-monitoring/`  
**说明**: 部署 FluentBit 和 VMAgent（依赖 telemetry-processor）

### Phase 8: 可视化和入口
**目录**: `08-grafana/`  
**说明**: 部署 Grafana、数据源、Dashboard 文件夹和 Ingress

## 🔑 命名规范

- **目录名称**: `XX-{phase-name}/` 其中 XX 是两位数字，表示部署顺序
- **文件命名**: 
  - `{resource-name}.yaml` - 普通资源
  - `{resource-name}-job.yaml` - Job 资源
  - `{resource-name}-cr.yaml` - Custom Resource
  - `{resource-name}-config.yaml` - ConfigMap

## 📝 添加新资源

### 1. 确定 Phase
根据资源的依赖关系，确定应该放在哪个 Phase。

### 2. 选择目录
选择对应的 Phase 目录，如果需要新的 Phase，创建新目录。

### 3. 创建模板文件
在对应目录下创建 YAML 文件，使用 Helm 模板语法。

### 4. 使用辅助函数
利用 `_helpers.tpl` 中的函数，避免重复代码。

### 示例：添加新的应用组件

```yaml
# templates/06-apps/app-new-component.yaml
{{- if .Values.apps.newComponent.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-new-component
  namespace: {{ include "primus-lens.namespace" . }}
  labels:
    {{- include "primus-lens.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.apps.newComponent.replicas }}
  # ... 其他配置
{{- end }}
```

然后在 `values.yaml` 中添加配置：

```yaml
apps:
  newComponent:
    enabled: true
    image: "primuslens/new-component:v1.0.0"
    replicas: 2
```

## 🛠️ 调试技巧

### 渲染单个模板
```bash
helm template primus-lens . -s templates/06-apps/app-api.yaml
```

### 查看特定 Phase 的资源
```bash
# Phase 3: 基础设施
helm template primus-lens . | grep -A 20 "kind: PostgresCluster"

# Phase 6: 应用
helm template primus-lens . | grep -A 10 "name: primus-lens-api"
```

### 验证 Hook Annotations
```bash
helm template primus-lens . | grep -B 5 "helm.sh/hook"
```

## 📚 相关文档

- [STRUCTURE.md](../STRUCTURE.md) - 完整目录结构
- [DEPLOYMENT_ORDER.md](../DEPLOYMENT_ORDER.md) - 详细部署流程
- [README.md](../README.md) - 用户文档
- [_helpers.tpl](./_helpers.tpl) - 辅助函数定义

---

通过这种按 Phase 组织的目录结构，部署顺序一目了然，维护更加容易！

