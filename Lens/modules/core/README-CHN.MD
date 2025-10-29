# Primus-Lens Core 模块

## 概述

`core` 模块是 Primus-Lens 项目的核心基础库，提供了一套完整的、可复用的基础设施组件。该模块封装了常用的功能模块，包括数据库访问、日志记录、监控指标、HTTP 服务器、Kubernetes 客户端集成等，为上层应用提供统一的基础能力支撑。

## 主要特性

- **配置管理**：基于 YAML 的配置加载和管理
- **数据库访问层**：基于 GORM 的 ORM 框架和自动生成的 DAL 层
- **日志系统**：集成 Logrus 和 Zap，支持结构化日志和链路追踪
- **指标收集**：基于 Prometheus 的监控指标收集和上报
- **HTTP 服务器**：基于 Gin 框架的 RESTful API 服务器
- **Kubernetes 集成**：封装的 K8s 客户端工具集
- **分布式追踪**：集成 Jaeger 的分布式链路追踪
- **错误处理**：统一的错误码和错误处理机制
- **工具库**：常用的工具函数集合

## 项目结构

```
pkg/
├── clientsets/           # Kubernetes 客户端集合
│   ├── clientset.go     # 主客户端
│   ├── k8s.go           # K8s 客户端
│   ├── kubelet.go       # Kubelet 客户端
│   ├── multicluster.go  # 多集群支持
│   ├── node_exporter.go # Node Exporter 客户端
│   └── storage.go       # 存储客户端
├── config/              # 配置管理
│   └── config.go        # 配置加载和结构定义
├── constant/            # 常量定义
│   ├── device.go        # 设备相关常量
│   ├── k8s.go           # K8s 相关常量
│   ├── lens.go          # Lens 相关常量
│   ├── reconciler.go    # 协调器常量
│   └── training.go      # 训练相关常量
├── context/             # 上下文管理
│   └── context.go       # 上下文工具函数
├── controller/          # 控制器管理
│   └── manager.go       # 控制器管理器
├── database/            # 数据库访问层
│   ├── dal/             # 数据访问层（自动生成）
│   ├── facade.go        # 数据库门面模式接口
│   ├── filter/          # 查询过滤器
│   ├── gen/             # 代码生成工具
│   └── model/           # 数据模型
├── errors/              # 错误处理
│   ├── error.go         # 错误定义
│   └── error_code.go    # 错误码定义
├── helper/              # 辅助工具
│   ├── fault/           # 故障处理
│   ├── gpu/             # GPU 相关工具
│   ├── kubelet/         # Kubelet 工具
│   ├── metadata/        # 元数据工具
│   ├── node/            # 节点工具
│   ├── prom/            # Prometheus 查询工具
│   ├── rdma/            # RDMA 统计工具
│   ├── storage/         # 存储工具
│   └── workload/        # 工作负载工具
├── logger/              # 日志系统
│   ├── common/          # 通用工具
│   ├── conf/            # 配置定义
│   ├── formatter/       # 格式化器
│   ├── hook/            # 钩子
│   ├── log/             # 日志实现
│   └── logrus/          # Logrus 封装
├── metrics/             # 监控指标
│   ├── counter.go       # 计数器
│   ├── gauge.go         # 仪表盘
│   ├── histogram.go     # 直方图
│   ├── timer.go         # 计时器
│   └── opts.go          # 选项配置
├── model/               # 业务模型
│   ├── amd-smi.go       # AMD SMI 模型
│   ├── cluster.go       # 集群模型
│   ├── gpu.go           # GPU 模型
│   ├── nodes.go         # 节点模型
│   ├── workload.go      # 工作负载模型
│   ├── storage.go       # 存储模型
│   └── rest/            # REST 请求/响应模型
├── pb/                  # Protocol Buffers
│   └── exporter/        # Exporter gRPC 定义
├── router/              # HTTP 路由
│   ├── middleware/      # 中间件
│   │   ├── cors.go      # CORS 中间件
│   │   ├── handle-error.go  # 错误处理中间件
│   │   └── tracing.go   # 追踪中间件
│   └── router.go        # 路由初始化
├── server/              # HTTP 服务器
│   ├── server.go        # 服务器初始化
│   └── health.go        # 健康检查
├── sql/                 # SQL 数据库工具
│   ├── callbacks/       # 回调
│   ├── conn.go          # 连接管理
│   ├── drivers.go       # 数据库驱动
│   ├── gen/             # 代码生成
│   ├── logger.go        # SQL 日志
│   ├── metrics/         # SQL 指标
│   └── opts.go          # 选项配置
├── trace/               # 分布式追踪
│   ├── trace.go         # 追踪实现
│   └── caller.go        # 调用者信息
└── utils/               # 工具函数集
    ├── goroutineUtil/   # Goroutine 工具
    ├── k8sUtil/         # K8s 工具
    ├── mapUtil/         # Map 工具
    ├── netUtil/         # 网络工具
    ├── regexUtil/       # 正则表达式工具
    ├── sliceUtil/       # Slice 工具
    └── stringUtil/      # 字符串工具
```

## 快速开始

### 依赖要求

- Go 1.24.5+
- PostgreSQL 或 MySQL 数据库
- Kubernetes 集群（可选，用于 K8s 集成功能）

### 安装

```bash
go get github.com/AMD-AGI/primus-lens/core
```

### 基础使用

#### 1. 配置文件

创建 `config.yaml` 配置文件：

```yaml
multiCluster: false
httpPort: 8080

controller:
  namespace: default
  leaderElectionId: primus-lens-leader
  metricsPort: 19191
  healthzPort: 19192
  pprofPort: 19193

nodeExporter:
  containerd_socket_path: /run/containerd/containerd.sock
  grpc_server: localhost:9090

jobs:
  grpc_port: 9091

netflow:
  scan_port_listen_interval_seconds: 2
  policy_config_path: /etc/netflow/policy.yaml
```

#### 2. 初始化服务器

```go
package main

import (
    "context"
    "github.com/AMD-AGI/primus-lens/core/pkg/server"
)

func main() {
    ctx := context.Background()
    if err := server.InitServer(ctx); err != nil {
        panic(err)
    }
}
```

#### 3. 使用数据库

```go
import (
    "context"
    "github.com/AMD-AGI/primus-lens/core/pkg/database"
    "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
)

// 创建节点记录
func CreateNode(ctx context.Context) error {
    node := &model.Node{
        Name:    "node-1",
        Address: "192.168.1.100",
        Status:  "Ready",
    }
    return database.CreateNode(ctx, node)
}

// 查询节点
func GetNode(ctx context.Context, name string) (*model.Node, error) {
    return database.GetNodeByName(ctx, name)
}
```

#### 4. 使用日志

```go
import (
    "github.com/AMD-AGI/primus-lens/core/pkg/logger"
    "github.com/AMD-AGI/primus-lens/core/pkg/logger/conf"
)

func Example() {
    log := logger.NewLogger()
    config := &conf.LogConfig{
        Level: conf.LevelInfo,
    }
    log.InitLogger(config)
    
    log.Infof("Server started on port %d", 8080)
    log.WithField("user_id", 123).Info("User logged in")
}
```

#### 5. 使用指标

```go
import (
    "github.com/AMD-AGI/primus-lens/core/pkg/metrics"
)

func Example() {
    // 创建计数器
    counter := metrics.NewCounter("requests_total", "Total requests")
    counter.Inc()
    
    // 创建仪表盘
    gauge := metrics.NewGauge("active_connections", "Active connections")
    gauge.Set(42)
    
    // 创建直方图
    histogram := metrics.NewHistogram("request_duration", "Request duration")
    histogram.Observe(0.5)
}
```

#### 6. 使用上下文

```go
import (
    "context"
    appContext "github.com/AMD-AGI/primus-lens/core/pkg/context"
)

func Example() {
    ctx := context.Background()
    
    // 存储对象到上下文
    ctx = appContext.WithObject(ctx, "user_id", "123")
    
    // 从上下文中获取对象
    if userID, ok := appContext.GetValue(ctx, "user_id"); ok {
        println(userID)
    }
}
```

## 核心组件

### 配置管理

配置模块支持从 YAML 文件加载配置，通过环境变量 `CONFIG_PATH` 指定配置文件路径（默认为 `config.yaml`）。

### 数据库访问层

- 使用 GORM 作为 ORM 框架
- 支持 PostgreSQL 和 MySQL
- 自动生成的 DAL 层提供类型安全的数据库操作
- 提供 Facade 模式的高级数据库操作接口

### 日志系统

- 支持多种日志级别：Trace、Debug、Info、Warning、Error、Fatal
- 支持结构化日志
- 集成分布式追踪（trace_id、span_id）
- 支持日志轮转（lumberjack）

### 监控指标

- 基于 Prometheus 的指标收集
- 支持 Counter、Gauge、Histogram、Summary 等指标类型
- 提供统一的指标命名和标签管理

### HTTP 服务器

- 基于 Gin 框架
- 内置 CORS、错误处理、追踪等中间件
- 支持健康检查端点
- 支持优雅关闭

### Kubernetes 集成

- 封装的 K8s 客户端，支持多集群
- Kubelet 客户端，用于节点级操作
- 支持 CRI（容器运行时接口）
- 提供 K8s 资源操作的工具函数

## 工具函数库

### goroutineUtil

提供安全的 goroutine 执行，支持 panic 恢复。

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/utils/goroutineUtil"

goroutineUtil.SafeGo(func() {
    // 你的代码
})
```

### k8sUtil

提供 K8s 资源操作的工具函数。

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"

// 获取 Pod 的 Owner
owner := k8sUtil.GetPodOwner(pod)

// 解析 GVK
gvk := k8sUtil.ParseGVK(obj)
```

### stringUtil、sliceUtil、mapUtil

提供常用的字符串、切片、Map 操作函数。

## 开发指南

### 代码生成

数据库模型和 DAL 层使用代码生成工具自动生成：

```bash
# 生成数据库 DAL 层
cd pkg/database/gen
go run main.go

# 生成 SQL 相关代码
cd pkg/sql/gen
go run main.go
```

### 添加新的数据模型

1. 在 `pkg/database/model/` 中定义模型结构
2. 运行代码生成工具生成 DAL 层
3. 在 `pkg/database/facade.go` 中添加高级操作接口

### 添加新的指标

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/metrics"

var myCounter = metrics.NewCounter(
    "my_metric",
    "Description of my metric",
    metrics.WithNamespace("my_namespace"),
    metrics.WithLabels(map[string]string{"env": "prod"}),
)
```

## 依赖项

主要依赖包括：

- **Web 框架**：gin-gonic/gin
- **ORM**：gorm.io/gorm
- **数据库驱动**：gorm.io/driver/postgres, gorm.io/driver/mysql
- **日志**：sirupsen/logrus, go.uber.org/zap
- **监控**：prometheus/client_golang
- **追踪**：uber/jaeger-client-go
- **Kubernetes**：k8s.io/client-go, k8s.io/api
- **Controller Runtime**：sigs.k8s.io/controller-runtime

完整依赖列表请查看 `go.mod` 文件。

## 最佳实践

1. **使用上下文传递**：所有数据库操作和 HTTP 请求都应该传递 context
2. **结构化日志**：使用 `WithField` 或 `WithFields` 添加日志字段
3. **错误处理**：使用统一的错误处理机制，包含错误码和错误信息
4. **指标命名**：遵循 Prometheus 命名规范，使用小写和下划线
5. **安全的 Goroutine**：使用 `goroutineUtil.SafeGo` 启动 goroutine

## 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./pkg/database/...

# 运行测试并显示覆盖率
go test -cover ./...
```

## 贡献

欢迎贡献代码！请遵循以下步骤：

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

## 许可证

请查看项目根目录的 LICENSE 文件。

## 联系方式

如有问题或建议，请提交 Issue 或联系项目维护者。

