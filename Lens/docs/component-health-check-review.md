# feature/lens/health-check 分支代码评审与重构方案

> 评审日期: 2026-02-27
> 评审分支: feature/lens/health-check (commit: 0fdfa0c8)

---

## 1. 当前实现概述

本次提交在 `primus-lens-api` 进程中实现了组件健康探测功能，主要包括：

- **HTTP API**: `GET /components/probe`，实时查询 K8s API 返回组件健康状态
- **Prometheus 指标**: `primus_lens_component_healthy` gauge，通过 `init()` 启动后台 goroutine 每 30s 采集
- **告警规则**: VMRule `ComponentUnhealthy`，当指标值为 0 持续 2 分钟触发告警
- **部署清单**: 给所有 Lens 组件 Pod 添加 `primus-lens-app-name` label

涉及文件 24 个，新增 771 行，删除 10 行。

---

## 2. 架构问题分析

### 2.1 采集逻辑不应耦合在 API 进程中

**问题**: `component_probe_metrics.go` 在 `init()` 中注册 Prometheus gauge 并启动后台 goroutine，这意味着 API 进程承担了指标采集的职责。

**影响**:
- 违反 Lens 现有的架构惯例——所有采集逻辑都在独立 exporter 中运行（`node-exporter`、`gpu-resource-exporter`、`gateway-exporter` 等），API 层从不自行采集
- API 进程挂掉 → 指标中断 → 告警链路断裂，采集与服务入口的单点故障耦合
- 后续如需探测更多组件（VictoriaMetrics、OpenSearch 等基础设施），API 进程会持续膨胀

**相关代码**:

```go
// Lens/modules/api/pkg/api/component_probe_metrics.go
func init() {
    prometheus.MustRegister(componentHealthyGauge)
    go runComponentProbeMetricsLoop()  // API 进程启动即运行采集循环
}
```

### 2.2 HTTP API 与指标采集重复探测

**问题**: `GET /components/probe` 接口和 metrics goroutine 调用完全相同的探测函数（`probeCoreDNS`、`listComponentsByLabel` 等），存在两条独立的探测路径。

**影响**:
- 数据不一致风险：指标每 30s 更新一次，而 API 每次请求实时查询 K8s，两者返回的健康状态可能不同
- API 每次被调用都直接查 K8s API，高频调用场景下可能对 kube-apiserver 造成压力
- 重复逻辑增加维护成本

**建议**: API 直接查询 VictoriaMetrics 中已有的 `primus_lens_component_healthy` 指标，作为唯一数据源。Lens 的其他 API（如 `unified_node.go` 中的 GPU 利用率查询）已经通过 `StorageClientSet.PrometheusRead` 查询 VM，应保持一致。

### 2.3 全量 List Pod 性能问题

**问题**: `listComponentsByLabel` 函数没有使用任何过滤条件，拉取整个集群的所有 Pod 后在本地筛选。

```go
// Lens/modules/api/pkg/api/component_probe.go:120-131
func listComponentsByLabel(ctx context.Context, c client.Client, labelKey string) ([]PlatformComponentItem, error) {
    var podList corev1.PodList
    if err := c.List(ctx, &podList); err != nil {  // 无过滤条件，全量拉取
        return nil, err
    }
    // ... 本地筛选
}
```

**影响**: 一个典型 GPU 集群可能有数千个 Pod，全量拉取带来不必要的 apiserver 负载、网络传输和内存分配。且该操作每 30 秒执行一次。

**对比**: 同一文件中的 `probeCoreDNS` 正确使用了 `client.MatchingLabels` 和 `client.InNamespace`，但 `listComponentsByLabel` 没有遵循同样的模式。

**建议**: 使用 `client.HasLabels{labelKey}` 将过滤下推到 apiserver 端。

### 2.4 只探测默认集群，未覆盖多集群场景

**问题**: metrics 循环中写死了空字符串获取默认集群客户端：

```go
// Lens/modules/api/pkg/api/component_probe_metrics.go:50
clients, err := cm.GetClusterClientsOrDefault("")
```

**影响**: Lens 管控面管理多个数据面集群（参考 `ClusterManager.ListAllClientSets()`），但当前实现只探测默认集群，非默认集群的组件健康状态完全没有指标覆盖。

### 2.5 通过 Pod 发现组件，无法检测"缺失"

**问题**: 平台组件的探测逻辑是从 Pod 出发——列出带有 `primus-lens-app-name` label 的 Pod，然后分组统计。如果一个组件的 Deployment 被删除或 replicas 缩为 0，Pod 不存在，则探测结果中完全不会出现该组件。

**影响**: 健康检查系统在组件彻底消失时反而最安静——不会产出任何不健康的指标，告警也不会触发。这是最危险的盲区。

**对比**: kube-system 组件的探测（`probeCoreDNS`）先查 Deployment 获取 desired replicas，即使 Pod 为 0 也能识别异常。但平台组件没有采用相同的模式。

### 2.6 Stale 指标不清理

**问题**: 代码注释明确提到了这个问题但未解决：

```go
// Reset gauges for this cluster so stale labels disappear (optional: only set current cluster labels)
// Here we set only the labels we'll update this round; old labels may remain until next full sync.
// For simplicity we don't reset the whole vector; we only Set() below.
```

**影响**: 已下线的组件（如删除了某个 Deployment），其 `healthy=1` 的指标永远残留在内存中，VictoriaMetrics 持续抓到过期数据，导致该组件永远显示"健康"。

### 2.7 健康判定标准不一致

| 组件类型 | 健康条件 | 代码位置 |
|---------|---------|---------|
| kube-system | `Ready == Desired` | `component_probe.go:84` |
| platform | `Ready >= 1` | `component_probe.go:158` |

平台组件使用 `ready >= 1` 过于宽松。例如 `node-exporter` DaemonSet 期望 8 个 Pod 但只有 1 个 Running，当前逻辑认为"健康"。应统一通过查询 workload controller 的 desired vs ready 来判断。

### 2.8 `init()` 中启动 goroutine 的副作用

- **不可测试**: `import` 该 package 即触发 goroutine 和 `prometheus.MustRegister`，测试中重复 import 会导致 duplicate registration panic
- **不可停止**: goroutine 使用 `context.Background()`，无法响应 graceful shutdown，进程收到 SIGTERM 后仍在执行无意义的 K8s API 调用

### 2.9 错误静默丢弃

多处 `_ =` 丢弃错误，K8s API 超时或 RBAC 权限不足时静默返回空列表，探测系统自身的故障不可感知：

```go
safeList, _ := listComponentsByLabel(ctx, c, labelPrimusSafeAppName)
lensList, _ := listComponentsByLabel(ctx, c, labelPrimusLensAppName)
```

---

## 3. 重构方案

### 3.1 总体思路

采用**每个集群独立探测 + 指标存储在本地 VM + API 聚合查询**的架构：

```
每个集群（管控面集群 + 所有数据面集群）:

  component-health-exporter (独立 Deployment)
    │
    │  通过 K8s API 查询本集群的 Deployment/DaemonSet/StatefulSet
    │  (label selector: primus-lens-app-name 或 primus-safe-app-name)
    │
    │  暴露 /metrics
    │      primus_component_status{...} desired=N ready=M
    │      primus_component_healthy{...} 1 or 0
    ▼
  VMAgent 抓取
    ▼
  VMCluster 存储
    ▼
  VMAlert 按规则评估 → 触发告警 → 推送到 telemetry-processor

管控面 API:

  GET /components/probe?cluster=xxx
    │
    │  查对应集群的 VMSelect (通过 StorageClientSet.PrometheusRead)
    │  执行 PromQL 查询 primus_component_healthy / primus_component_status
    │
    ▼
  返回聚合结果
```

### 3.2 独立 Exporter 设计

#### 3.2.1 职责

一个轻量级的 Deployment，运行在每个集群的 `primus-lens` namespace 中，只做一件事：**定期查询本集群中带有 `primus-lens-app-name` 或 `primus-safe-app-name` label 的 workload controller，对比 desired 和 ready 副本数，暴露为 Prometheus 指标。**

#### 3.2.2 探测逻辑

不再从 Pod 出发，而是从 workload controller（Deployment / DaemonSet / StatefulSet）出发：

1. 使用 label selector 查询 Deployment:
   `client.HasLabels{"primus-lens-app-name"}` + 可选的 `client.InNamespace("primus-lens")`
2. 使用 label selector 查询 DaemonSet（同上）
3. 使用 label selector 查询 StatefulSet（同上）
4. 对 `primus-safe-app-name` 重复以上步骤

对每个查到的 workload controller：
- 读取 `app-name` label 值作为组件名
- 读取 desired replicas（Deployment: `spec.replicas`; DaemonSet: `status.desiredNumberScheduled`; StatefulSet: `spec.replicas`）
- 读取 ready replicas（Deployment: `status.readyReplicas`; DaemonSet: `status.numberReady`; StatefulSet: `status.readyReplicas`）
- 计算 healthy: `desired > 0 && ready == desired`

这样当 Deployment 存在但 Pod 为 0 时，探测仍能发现异常（desired > 0 but ready = 0）。

#### 3.2.3 指标定义

```
# 组件副本数详情（用于 dashboard 展示和精细化告警）
primus_component_replicas{
    platform="primus_lens|primus_safe",
    app_name="api|jobs|...",
    namespace="primus-lens|primus-safe",
    kind="Deployment|DaemonSet|StatefulSet",
    cluster="<cluster_name>"
} desired=3 ready=3

# 组件健康状态（用于告警判断，1=healthy, 0=unhealthy）
primus_component_healthy{
    platform="primus_lens|primus_safe",
    app_name="api|jobs|...",
    namespace="primus-lens|primus-safe",
    kind="Deployment|DaemonSet|StatefulSet",
    cluster="<cluster_name>"
} 1
```

也可以直接暴露 `desired` 和 `ready` 两个 gauge，让 VMRule 用表达式 `primus_component_replicas_ready < primus_component_replicas_desired` 来判断，这样更灵活。

#### 3.2.4 kube-system 组件

CoreDNS 和 NodeLocal DNS 的探测也统一到同一个 exporter 中，使用 `k8s-app` label 查询，逻辑与平台组件一致（查 Deployment/DaemonSet controller 而非 Pod）。

#### 3.2.5 代码结构参考

参考现有 exporter 的结构（如 `gpu-resource-exporter`），使用 `server.InitServerWithPreInitFunc` 启动：

```
Lens/modules/exporters/component-health-exporter/
├── cmd/component-health-exporter/main.go
├── pkg/
│   ├── bootstrap/bootstrap.go        # ComponentDeclaration: DataPlane, RequireK8S=true, RequireStorage=false
│   ├── collector/collector.go         # 探测逻辑 + Prometheus collector 接口
│   └── collector/collector_test.go    # 使用 controller-runtime/fake 测试
├── go.mod
└── installer/Dockerfile
```

关键点：
- `ComponentDeclaration` 设置为 DataPlane + RequireK8S=true + RequireStorage=false（exporter 不需要数据库连接）
- 实现标准的 `prometheus.Collector` 接口（`Describe` + `Collect`），每次 scrape 时执行探测，不需要自己维护 goroutine 和 ticker
- 使用 `client.HasLabels` 做 label selector 查询，避免全量 List
- cluster 名称从环境变量 `CLUSTER_NAME` 获取（与其他 exporter 一致）

#### 3.2.6 部署

- 每个数据面集群部署一个实例（在 `primus-lens-apps-dataplane` chart 中添加）
- 管控面集群也部署一个实例（在 `primus-lens-apps-control-plane` chart 中添加）
- Pod 添加 Prometheus scrape annotations，被本集群的 VMAgent 抓取
- 指标存入本集群的 VMCluster

### 3.3 告警规则

复用现有的 VMRule 机制，但建议调整表达式：

```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: primus-component-health-alerts
  namespace: primus-lens
  labels:
    app: primus-lens
    component: alerts
    category: component
spec:
  groups:
    - name: component_health_alerts
      interval: 30s
      rules:
        - alert: ComponentUnhealthy
          expr: primus_component_healthy == 0
          for: 2m
          labels:
            severity: warning
            category: component
          annotations:
            summary: "Component {{ $labels.app_name }} is unhealthy"
            description: >-
              cluster={{ $labels.cluster }}
              platform={{ $labels.platform }}
              app_name={{ $labels.app_name }}
              namespace={{ $labels.namespace }}
              kind={{ $labels.kind }}
              has been unhealthy for 2m
```

该 VMRule 部署在每个集群中，由本集群的 VMAlert 评估。

### 3.4 API 层改造

`GET /components/probe` 不再直接查 K8s API，改为查 VictoriaMetrics：

```
请求: GET /api/v1/components/probe?cluster=xxx

处理逻辑:
1. 通过 ClusterManager.GetClusterClientsOrDefault(cluster) 获取目标集群的 ClientSet
2. 使用 ClientSet.StorageClientSet.PrometheusRead 查询 PromQL:
   - primus_component_healthy{cluster="xxx"}
   - primus_component_replicas{cluster="xxx"}  (如需详情)
3. 将 PromQL 结果转换为结构化 JSON 返回
```

若不传 cluster 参数，可遍历 `ClusterManager.ListAllClientSets()` 查询所有集群并聚合。

这样 API 的角色变成了一个**指标聚合代理**，不包含任何探测逻辑，数据来源唯一（VictoriaMetrics），与 `unified_node.go` 中查询 GPU 利用率等指标的模式完全一致。

### 3.5 Label 管理改进

将 `primus-lens-app-name` label 的添加方式从手动逐文件改为在 Helm `_helpers.tpl` 中统一注入：

```yaml
# charts/primus-lens-apps-dataplane/templates/_helpers.tpl
{{- define "lens.podLabels" -}}
primus-lens-app-name: {{ .appName }}
{{- include "lens.selectorLabels" . | nindent 0 }}
{{- end }}
```

各组件 template 引用 `{{ include "lens.podLabels" (dict "appName" "jobs" ...) }}`，确保新增组件时不会遗漏。

---

## 4. 需要修改的文件清单

### 4.1 需要删除或清空的文件

| 文件 | 操作 | 原因 |
|------|------|------|
| `Lens/modules/api/pkg/api/component_probe_metrics.go` | 删除 | 采集逻辑移到独立 exporter |
| `Lens/modules/api/pkg/api/component_probe.go` | 删除 | 探测逻辑移到独立 exporter |
| `Lens/docs/component-probe-and-alerts.md` | 删除或重写 | 架构已变化 |

### 4.2 需要修改的文件

| 文件 | 修改内容 |
|------|---------|
| `Lens/modules/api/pkg/api/unified_components_probe.go` | 重写 handler，改为查询 VictoriaMetrics 而非直接查 K8s |
| `Lens/modules/api/pkg/api/router.go` | 路由保持不变 |
| `Lens/deploy/metrics/rules/vmrule-component-health.yaml` | 更新告警表达式，适配新指标名 |
| `Lens/charts/primus-lens-apps-dataplane/templates/_helpers.tpl` | 添加统一 label 注入 helper |
| `Lens/charts/primus-lens-apps-control-plane/templates/_helpers.tpl` | 同上 |
| 各组件 YAML template (dataplane + control-plane) | 引用 helper 而非硬编码 label |
| 遗漏的组件 YAML (github-runners-exporter, primus-safe-adapter, skills-repository, app-web) | 补充 label |
| `Lens/bootstrap/manifests/app-api.yaml.tpl` | 移除 prometheus scrape annotations（API 不再暴露探测指标）|

### 4.3 需要新增的文件

| 文件 | 说明 |
|------|------|
| `Lens/modules/exporters/component-health-exporter/cmd/component-health-exporter/main.go` | exporter 入口 |
| `Lens/modules/exporters/component-health-exporter/pkg/bootstrap/bootstrap.go` | 初始化 |
| `Lens/modules/exporters/component-health-exporter/pkg/collector/collector.go` | Prometheus collector 实现 |
| `Lens/modules/exporters/component-health-exporter/pkg/collector/collector_test.go` | 单元测试 |
| `Lens/modules/exporters/component-health-exporter/go.mod` | Go module 定义 |
| `Lens/modules/exporters/component-health-exporter/installer/Dockerfile` | 镜像构建 |
| `Lens/charts/primus-lens-apps-dataplane/templates/app-component-health-exporter.yaml` | 数据面部署 |
| `Lens/charts/primus-lens-apps-control-plane/templates/app-component-health-exporter.yaml` | 管控面部署 |
| `Lens/bootstrap/manifests/app-component-health-exporter.yaml.tpl` | bootstrap 部署模板 |

### 4.4 需要删除的测试文件

| 文件 | 原因 |
|------|------|
| `Lens/modules/api/pkg/api/component_probe_test.go` | 原测试逻辑移到 exporter 中重写 |
| `Lens/modules/api/pkg/api/unified_components_probe_test.go` | handler 逻辑已变更，需重写 |

---

## 5. 总结

核心改动只有三件事：

1. **新建一个轻量 exporter**：查 workload controller（Deployment/DaemonSet/StatefulSet）的 desired vs ready，暴露 Prometheus gauge
2. **每个集群部署一个实例**：管控面集群和数据面集群各部署一个，指标存入本集群 VMCluster
3. **API 改为查 VM 聚合**：`GET /components/probe` 通过 `StorageClientSet.PrometheusRead` 查询 VictoriaMetrics，不再直接查 K8s
