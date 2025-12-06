# K8s 可观测性系统设计文档

## 1. 概述

### 1.1 背景
构建一个面向 Kubernetes 集群的统一可观测性平台，整合 Trace、Log、Metrics 三大支柱，提供服务拓扑感知、链路追踪、日志分析和 Pod 监控能力。

### 1.2 目标
- **统一监控**：一站式查看 K8s 应用的运行状态
- **链路追踪**：以 DAG 形式直观展示请求调用链路
- **拓扑感知**：自动发现和展示服务间依赖关系
- **日志关联**：通过 TraceID 关联分布式日志
- **多维度分析**：支持按 Product、App、Namespace 等维度聚合分析

### 1.3 服务标识规范
系统通过以下 K8s Label 来标识和组织服务：

| Label | 说明 | 示例 |
|-------|------|------|
| `product` | 产品线/业务域 | `ai-platform`, `data-service` |
| `app` | 应用/服务名称 | `inference-gateway`, `model-server` |

服务唯一标识 = `{product}/{app}`

---

## 2. 系统架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         可视化层 (Frontend)                          │
│   Trace DAG  │  服务拓扑图  │  日志查询  │  Pod 监控  │  告警面板   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         API 网关层 (Gateway)                         │
│      统一查询 API  │  认证鉴权  │  限流  │  数据聚合  │  告警管理    │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        数据处理层 (Processing)                       │
│   Trace处理器  │  拓扑分析引擎  │  日志处理器  │  指标聚合器  │  告警引擎 │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          存储层 (Storage)                            │
│   Trace Store  │  Topology DB  │  Log Store  │  Metrics TSDB        │
└─────────────────────────────────────────────────────────────────────┘
                                    ▲
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│                         采集层 (Collection)                          │
│               OpenTelemetry Collector (统一接入)                      │
│         Traces         │        Logs        │       Metrics          │
└─────────────────────────────────────────────────────────────────────┘
                                    ▲
                                    │
┌─────────────────────────────────────────────────────────────────────┐
│                        K8s 应用层 (Workloads)                        │
│    Pod (product=ai-platform, app=gateway)                           │
│    Pod (product=ai-platform, app=model-server)                      │
│    Pod (product=data-service, app=etl-worker)                       │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 3. 模块设计

### 3.1 数据采集层

#### 3.1.1 Trace 采集模块
| 项目 | 说明 |
|------|------|
| **功能** | 采集应用的分布式追踪数据，包括 Span、调用关系、耗时等 |
| **数据来源** | 应用内嵌 SDK 埋点 |
| **关键元数据** | TraceID, SpanID, ParentSpanID, product, app, namespace, pod_name, node_name |
| **技术栈** | OpenTelemetry SDK (Go/Java/Python) |
| **采样策略** | 支持头部采样、尾部采样、错误优先采样 |

#### 3.1.2 日志采集模块
| 项目 | 说明 |
|------|------|
| **功能** | 采集容器标准输出日志，注入 K8s 元数据和 TraceID |
| **数据来源** | 容器 stdout/stderr，应用日志文件 |
| **关键元数据** | timestamp, level, message, trace_id, product, app, namespace, pod_name |
| **技术栈** | Fluent Bit (DaemonSet 部署) |
| **日志格式** | JSON 结构化日志 |

#### 3.1.3 指标采集模块
| 项目 | 说明 |
|------|------|
| **功能** | 采集 Pod 资源使用、应用性能指标、K8s 组件指标 |
| **数据来源** | kubelet metrics, cAdvisor, 应用自定义 metrics |
| **关键维度** | product, app, namespace, pod, node, container |
| **技术栈** | Prometheus + kube-state-metrics |

#### 3.1.4 统一采集器
| 项目 | 说明 |
|------|------|
| **功能** | 统一接收 Trace/Log/Metrics，进行预处理和路由分发 |
| **部署方式** | DaemonSet (每节点一个) + Gateway (集中处理) |
| **技术栈** | OpenTelemetry Collector |
| **处理能力** | 数据过滤、采样、批处理、元数据注入 |

---

### 3.2 数据处理层

#### 3.2.1 Trace 处理器
| 项目 | 说明 |
|------|------|
| **功能** | Span 聚合、Trace 重建、DAG 结构生成、异常检测 |
| **输入** | 原始 Span 数据流 |
| **输出** | 完整 Trace、DAG 结构、服务调用统计 |
| **技术栈** | Go 自研服务 |
| **核心算法** | Span 树构建、关键路径分析、延迟分布计算 |

#### 3.2.2 拓扑分析引擎
| 项目 | 说明 |
|------|------|
| **功能** | 从 Trace 数据推断服务依赖关系，构建服务拓扑图 |
| **分析维度** | Product 级拓扑、App 级拓扑、Namespace 级拓扑 |
| **输出数据** | 服务节点列表、服务间调用边、调用统计（QPS/延迟/错误率） |
| **技术栈** | Go 自研服务 |
| **更新策略** | 增量更新 + 定期全量重建 |

#### 3.2.3 日志处理器
| 项目 | 说明 |
|------|------|
| **功能** | 日志解析、字段提取、TraceID 关联、日志分级 |
| **处理能力** | 正则解析、JSON 解析、Grok 模式匹配 |
| **技术栈** | Vector 或 Fluent Bit 内置处理 |

#### 3.2.4 指标聚合器
| 项目 | 说明 |
|------|------|
| **功能** | 指标预聚合、多维度汇总、SLI/SLO 计算 |
| **聚合维度** | product, app, namespace, node |
| **预计算指标** | P50/P95/P99 延迟、错误率、QPS、资源利用率 |
| **技术栈** | Prometheus Recording Rules / VictoriaMetrics |

#### 3.2.5 告警引擎
| 项目 | 说明 |
|------|------|
| **功能** | 告警规则评估、告警聚合、通知分发 |
| **告警类型** | 指标阈值告警、日志异常告警、Trace 异常告警 |
| **技术栈** | Prometheus Alertmanager |

---

### 3.3 存储层

#### 3.3.1 Trace 存储
| 项目 | 说明 |
|------|------|
| **功能** | 存储完整 Trace 数据，支持 TraceID 查询和条件检索 |
| **技术栈** | ClickHouse |
| **存储周期** | 7-30 天（可配置） |
| **索引字段** | trace_id, product, app, namespace, start_time, duration, status |
| **备选方案** | Jaeger + Elasticsearch |

#### 3.3.2 日志存储
| 项目 | 说明 |
|------|------|
| **功能** | 存储结构化日志，支持全文检索和字段过滤 |
| **技术栈** | Grafana Loki |
| **存储周期** | 15-30 天 |
| **标签字段** | product, app, namespace, pod, level |
| **备选方案** | Elasticsearch |

#### 3.3.3 指标存储
| 项目 | 说明 |
|------|------|
| **功能** | 时序指标存储，支持 PromQL 查询 |
| **技术栈** | VictoriaMetrics |
| **存储周期** | 90 天 |
| **备选方案** | Prometheus + Thanos |

#### 3.3.4 拓扑存储
| 项目 | 说明 |
|------|------|
| **功能** | 存储服务拓扑关系和统计数据 |
| **技术栈** | PostgreSQL + Redis 缓存 |
| **数据结构** | 服务节点表、服务边表、统计快照表 |
| **备选方案** | Neo4j (图数据库) |

---

### 3.4 API 网关层

#### 3.4.1 统一查询 API
| 项目 | 说明 |
|------|------|
| **功能** | 提供 RESTful API，统一访问 Trace、Log、Metrics、Topology 数据 |
| **核心接口** | Trace 查询、日志检索、拓扑获取、指标查询、Pod 状态 |
| **技术栈** | Go + Gin |

#### 3.4.2 主要 API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/traces/{traceId}` | GET | 获取 Trace 详情及 DAG 结构 |
| `/api/v1/traces` | GET | 搜索 Trace（按 product/app/时间范围） |
| `/api/v1/topology` | GET | 获取服务拓扑图 |
| `/api/v1/topology/{product}` | GET | 获取指定产品线的拓扑 |
| `/api/v1/logs` | GET | 日志检索 |
| `/api/v1/logs/trace/{traceId}` | GET | 获取 Trace 关联日志 |
| `/api/v1/services` | GET | 服务列表（按 product/app 组织） |
| `/api/v1/services/{product}/{app}/pods` | GET | 获取服务的 Pod 列表及状态 |
| `/api/v1/metrics` | POST | 指标查询（PromQL 代理） |

---

### 3.5 可视化层

#### 3.5.1 Trace DAG 视图
| 项目 | 说明 |
|------|------|
| **功能** | 以有向无环图形式展示请求调用链路 |
| **交互** | 节点点击查看 Span 详情，支持缩放/平移 |
| **技术栈** | React + D3.js + dagre (布局算法) |

#### 3.5.2 服务拓扑视图
| 项目 | 说明 |
|------|------|
| **功能** | 展示服务间依赖关系，显示调用量、延迟、错误率 |
| **层级** | Product 视图 → App 视图 → Pod 视图 |
| **技术栈** | React + vis.js 或 Cytoscape.js |

#### 3.5.3 日志查询视图
| 项目 | 说明 |
|------|------|
| **功能** | 日志搜索、过滤、实时流、上下文查看 |
| **过滤维度** | product, app, namespace, pod, level, 时间范围, 关键词 |
| **技术栈** | React + 虚拟滚动 |

#### 3.5.4 Pod 监控面板
| 项目 | 说明 |
|------|------|
| **功能** | Pod 列表、状态、资源使用、事件、日志入口 |
| **组织方式** | 按 Product → App 层级组织 |
| **技术栈** | React + ECharts |

#### 3.5.5 告警面板
| 项目 | 说明 |
|------|------|
| **功能** | 告警列表、告警详情、告警历史、静默管理 |
| **技术栈** | React |

---

## 4. 数据模型

### 4.1 核心标识字段

所有数据都包含以下统一标识字段：

```
┌──────────────────────────────────────────────────────────────┐
│                      统一元数据字段                           │
├──────────────────────────────────────────────────────────────┤
│  product      │  产品线标识 (来自 K8s Label: product)         │
│  app          │  应用标识 (来自 K8s Label: app)               │
│  namespace    │  K8s 命名空间                                 │
│  pod_name     │  Pod 名称                                     │
│  node_name    │  节点名称                                     │
│  container    │  容器名称                                     │
└──────────────────────────────────────────────────────────────┘
```

### 4.2 服务标识层级

```
Product (产品线)
    └── App (应用/服务)
            └── Namespace (环境/租户)
                    └── Pod (实例)
                            └── Container (容器)
```

### 4.3 数据关联

```
                    ┌─────────────┐
                    │   Trace     │
                    │  (TraceID)  │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌──────────┐    ┌──────────┐    ┌──────────┐
    │   Log    │    │  Metrics │    │ Topology │
    │(TraceID) │    │(product/ │    │(product/ │
    │          │    │   app)   │    │   app)   │
    └──────────┘    └──────────┘    └──────────┘
```

---

## 5. 技术栈汇总

| 层级 | 组件 | 技术选型 | 说明 |
|------|------|----------|------|
| 采集层 | 统一采集器 | OpenTelemetry Collector | CNCF 标准，统一接入 |
| 采集层 | Trace SDK | OpenTelemetry SDK | 多语言支持 |
| 采集层 | 日志采集 | Fluent Bit | 轻量级，K8s 原生 |
| 采集层 | 指标采集 | Prometheus | 生态成熟 |
| 处理层 | 核心服务 | Go | 高性能 |
| 存储层 | Trace 存储 | ClickHouse | 列式存储，查询快 |
| 存储层 | 日志存储 | Grafana Loki | 轻量级，成本低 |
| 存储层 | 指标存储 | VictoriaMetrics | 高性能，兼容 Prometheus |
| 存储层 | 拓扑存储 | PostgreSQL + Redis | 关系型 + 缓存 |
| API 层 | Web 框架 | Go + Gin | 高性能 REST API |
| 可视化 | 前端框架 | React + TypeScript | 组件化开发 |
| 可视化 | 图表库 | D3.js + ECharts | DAG + 图表 |
| 可视化 | 拓扑图 | Cytoscape.js | 网络图可视化 |
| 部署 | 容器编排 | Kubernetes | 云原生 |
| 部署 | 配置管理 | Helm | K8s 应用打包 |

---

## 6. 部署架构

### 6.1 组件部署方式

| 组件 | 部署方式 | 副本数 | 说明 |
|------|----------|--------|------|
| OTel Collector (Agent) | DaemonSet | 每节点1个 | 边缘采集 |
| OTel Collector (Gateway) | Deployment | 3+ | 集中处理 |
| Fluent Bit | DaemonSet | 每节点1个 | 日志采集 |
| 核心处理服务 | Deployment | 3+ | 高可用 |
| API 服务 | Deployment | 3+ | 高可用 |
| 前端服务 | Deployment | 2+ | 静态资源 |
| ClickHouse | StatefulSet | 3+ | 分布式集群 |
| Loki | StatefulSet | 3+ | 分布式模式 |
| VictoriaMetrics | StatefulSet | 3+ | 集群模式 |
| PostgreSQL | StatefulSet | 1 (主) + 1 (备) | 主从复制 |
| Redis | StatefulSet | 3 | Sentinel 模式 |

### 6.2 命名空间规划

```
observability-system/       # 可观测性系统组件
    ├── otel-collector-*
    ├── fluent-bit-*
    ├── processing-service-*
    ├── api-service-*
    └── frontend-*

observability-storage/      # 存储组件
    ├── clickhouse-*
    ├── loki-*
    ├── victoria-metrics-*
    ├── postgresql-*
    └── redis-*
```

---

## 7. 未来扩展

### 7.1 Phase 2 功能
- **智能根因分析**：基于 Trace 和 Metrics 的异常自动定位
- **SLO 管理**：服务级别目标定义和监控
- **成本分析**：按 Product/App 维度的资源成本分摊

### 7.2 Phase 3 功能
- **AIOps**：基于 ML 的异常检测和预测
- **混沌工程集成**：故障注入和恢复验证
- **多集群支持**：跨集群统一视图

---

## 8. 参考资料

- [OpenTelemetry 官方文档](https://opentelemetry.io/docs/)
- [Jaeger Architecture](https://www.jaegertracing.io/docs/architecture/)
- [ClickHouse 最佳实践](https://clickhouse.com/docs/)
- [Grafana Loki 设计文档](https://grafana.com/docs/loki/latest/)

