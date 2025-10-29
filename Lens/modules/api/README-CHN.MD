# Primus-Lens API 模块

## 概述

`api` 模块是 Primus-Lens 项目的 RESTful API 服务，提供了一套完整的 HTTP API 接口，用于集群监控、节点管理、工作负载跟踪和 GPU 资源管理。该模块基于 `core` 模块的基础设施构建，通过 RESTful 接口暴露监控数据和管理能力。

## 主要特性

- **集群管理 API**：集群概览、GPU 分配、利用率统计、消费者信息
- **节点管理 API**：GPU 节点列表、节点详情、GPU 设备信息、指标查询
- **工作负载管理 API**：工作负载列表、层次结构、详细信息、指标、训练性能
- **GPU 监控 API**：GPU 利用率、分配率、历史趋势、热力图
- **存储管理 API**：存储统计和使用信息
- **RESTful 架构**：标准 HTTP REST API，使用 JSON 请求/响应格式
- **中间件集成**：CORS、错误处理、分布式追踪、日志记录
- **数据库集成**：与 core 模块的数据库层无缝集成

## 项目结构

```
api/
├── cmd/
│   └── primus-lens-api/
│       └── main.go           # 应用程序入口
├── pkg/
│   ├── api/                  # API 处理器
│   │   ├── router.go         # 路由注册
│   │   ├── cluster.go        # 集群管理 API
│   │   ├── node.go           # 节点管理 API
│   │   ├── workload.go       # 工作负载管理 API
│   │   ├── gpu.go            # GPU 设备 API
│   │   ├── metrics.go        # 指标查询 API
│   │   └── storage.go        # 存储管理 API
│   └── bootstrap/
│       └── bootstrap.go      # 服务器初始化和配置
├── go.mod
└── go.sum
```

## API 接口

### 集群管理

#### GET /api/clusters/overview
获取集群概览，包含节点统计、GPU 分配、利用率和存储信息。

**响应示例：**
```json
{
  "code": 0,
  "data": {
    "totalNodes": 10,
    "healthyNodes": 9,
    "faultyNodes": 1,
    "fullyIdleNodes": 3,
    "partiallyIdleNodes": 4,
    "busyNodes": 2,
    "allocationRate": 0.75,
    "utilization": 0.62,
    "storageStat": { ... },
    "rdmaClusterStat": { ... }
  }
}
```

#### GET /api/clusters/consumers
获取 GPU 消费者（工作负载）列表，支持分页。

**查询参数：**
- `pageNum` (int)：页码（默认：1）
- `pageSize` (int)：每页大小（默认：10）

**响应示例：**
```json
{
  "code": 0,
  "data": {
    "data": [
      {
        "kind": "Job",
        "name": "training-job-1",
        "namespace": "default",
        "uid": "abc123",
        "stat": {
          "gpuRequest": 8,
          "gpuUtilization": 0.85
        }
      }
    ],
    "total": 50
  }
}
```

#### GET /api/clusters/gpuHeatmap
获取 GPU 热力图，包含 Top K GPU 功耗和利用率统计。

### 节点管理

#### GET /api/nodes
获取 GPU 节点列表，包含分配和利用率信息。

#### GET /api/nodes/:name
获取特定节点的详细信息。

**响应示例：**
```json
{
  "code": 0,
  "data": {
    "name": "gpu-node-1",
    "address": "192.168.1.100",
    "status": "Ready",
    "gpuModel": "AMD MI300X",
    "gpuCount": 8,
    "allocatedGpus": 6,
    "utilizationRate": 0.78
  }
}
```

#### GET /api/nodes/:name/gpuDevices
获取特定节点的 GPU 设备信息。

#### GET /api/nodes/:name/gpuMetrics
获取特定节点的 GPU 指标。

**查询参数：**
- `start` (int64)：开始时间戳（Unix 秒）
- `end` (int64)：结束时间戳（Unix 秒）
- `step` (int)：查询步长，单位秒（默认：60）

#### GET /api/nodes/:name/workloads
获取特定节点上运行的工作负载。

#### GET /api/nodes/:name/workloadsHistory
获取特定节点的历史工作负载信息。

#### GET /api/nodes/gpuAllocation
获取所有节点的 GPU 分配信息。

#### GET /api/nodes/gpuUtilization
获取集群的 GPU 利用率统计。

#### GET /api/nodes/gpuUtilizationHistory
获取历史 GPU 利用率数据。

**查询参数：**
- `start` (int64)：开始时间戳（Unix 秒）
- `end` (int64)：结束时间戳（Unix 秒）
- `step` (int)：查询步长，单位秒（默认：60）

### 工作负载管理

#### GET /api/workloads
列出工作负载，支持过滤和分页。

**查询参数：**
- `pageNum` (int)：页码
- `pageSize` (int)：每页大小
- `name` (string)：按工作负载名称过滤
- `kind` (string)：按工作负载类型过滤（Job、Deployment 等）
- `namespace` (string)：按命名空间过滤
- `status` (string)：按状态过滤
- `labels` (string)：按标签过滤

#### GET /api/workloads/:uid
获取特定工作负载的详细信息。

#### GET /api/workloads/:uid/hierarchy
获取工作负载层次结构（父子关系）。

#### GET /api/workloads/:uid/metrics
获取特定工作负载的指标。

**查询参数：**
- `start` (int64)：开始时间戳
- `end` (int64)：结束时间戳
- `step` (int)：查询步长，单位秒

#### GET /api/workloads/:uid/trainingPerformance
获取 AI/ML 工作负载的训练性能指标。

#### GET /api/workloadMetadata
获取所有工作负载的元数据。

### 存储管理

#### GET /api/storage/stat
获取存储统计信息，包括容量、使用情况和可用性。

## 快速开始

### 依赖要求

- Go 1.24.5+
- PostgreSQL 或 MySQL 数据库（在 core 模块中配置）
- 带有 GPU 节点的 Kubernetes 集群
- Primus-Lens core 模块
- Prometheus 或兼容的指标存储

### 安装

```bash
# 进入 api 模块目录
cd Lens/modules/api

# 安装依赖
go mod download

# 编译应用程序
go build -o primus-lens-api ./cmd/primus-lens-api
```

### 配置

API 模块使用 core 模块的配置。创建 `config.yaml` 配置文件：

```yaml
multiCluster: false
httpPort: 8080

# 数据库配置
database:
  type: postgres
  host: localhost
  port: 5432
  database: primus_lens
  username: postgres
  password: password

# 日志配置
logging:
  level: info
  format: json

# 指标配置
metrics:
  enabled: true
  port: 9090

# 追踪配置
tracing:
  enabled: true
  jaeger:
    endpoint: http://localhost:14268/api/traces
```

### 运行服务器

```bash
# 设置配置文件路径（可选，默认为 config.yaml）
export CONFIG_PATH=/path/to/config.yaml

# 运行服务器
./primus-lens-api
```

API 服务器将在配置的 HTTP 端口上启动（默认：8080）。

### 使用 API

```bash
# 获取集群概览
curl http://localhost:8080/api/clusters/overview

# 列出 GPU 节点
curl http://localhost:8080/api/nodes

# 获取节点详情
curl http://localhost:8080/api/nodes/gpu-node-1

# 列出工作负载
curl "http://localhost:8080/api/workloads?pageNum=1&pageSize=10"

# 获取工作负载详情
curl http://localhost:8080/api/workloads/{uid}

# 获取 GPU 利用率历史
curl "http://localhost:8080/api/nodes/gpuUtilizationHistory?start=1609459200&end=1609545600&step=300"
```

## 开发

### 添加新的 API

1. **定义处理器函数**：在适当的文件中创建处理器函数（cluster.go、node.go 等）

```go
func getMyResource(c *gin.Context) {
    // 查询数据
    data, err := myHelper.GetData(c)
    if err != nil {
        _ = c.Error(err)
        return
    }
    
    // 返回响应
    c.JSON(http.StatusOK, rest.SuccessResp(c, data))
}
```

2. **注册路由**：在 `router.go` 中添加路由

```go
func RegisterRouter(group *gin.RouterGroup) error {
    myGroup := group.Group("/myresources")
    {
        myGroup.GET("", getMyResourceList)
        myGroup.GET(":id", getMyResource)
    }
    return nil
}
```

3. **使用辅助函数**：利用 core 模块的辅助函数

```go
import (
    "github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
    "github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
)

data, err := gpu.GetGpuNodes(ctx, clientsets.GetCurrentClusterK8SClientSet(), metadata.GpuVendorAMD)
```

### 错误处理

所有 API 处理器应使用统一的错误处理机制：

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/errors"

// 通过 gin context 返回错误
if err != nil {
    _ = c.Error(errors.WrapError(err, "Failed to get data", errors.CodeDatabaseError))
    return
}

// 或创建自定义错误
if resource == nil {
    _ = c.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
    return
}
```

### 响应格式

使用 core 模块的标准响应格式：

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/model/rest"

// 成功响应
c.JSON(http.StatusOK, rest.SuccessResp(c, data))

// 错误响应由中间件处理
```

## 测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./pkg/api/...

# 运行测试并显示覆盖率
go test -cover ./...

# 运行集成测试（需要运行的数据库和 K8s 集群）
go test -tags=integration ./...
```

## 部署

### Docker

```dockerfile
FROM golang:1.24.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o primus-lens-api ./cmd/primus-lens-api

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/primus-lens-api .
COPY config.yaml .

EXPOSE 8080
CMD ["./primus-lens-api"]
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: primus-lens-api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: primus-lens-api
  template:
    metadata:
      labels:
        app: primus-lens-api
    spec:
      containers:
      - name: api
        image: primus-lens-api:latest
        ports:
        - containerPort: 8080
        env:
        - name: CONFIG_PATH
          value: /etc/primus-lens/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/primus-lens
      volumes:
      - name: config
        configMap:
          name: primus-lens-config
---
apiVersion: v1
kind: Service
metadata:
  name: primus-lens-api
spec:
  selector:
    app: primus-lens-api
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

## 依赖项

主要依赖包括：

- **Core 模块**：github.com/AMD-AGI/primus-lens/core
- **Web 框架**：gin-gonic/gin
- **Kubernetes 客户端**：k8s.io/client-go, k8s.io/api

该模块继承了 core 模块的所有依赖。完整依赖列表请查看 `go.mod` 文件。

## 最佳实践

1. **上下文传递**：始终从 gin.Context 传递 context 到辅助函数
2. **错误处理**：使用统一的错误处理机制和适当的错误码
3. **分页**：为列表 API 实现分页以避免大响应
4. **查询参数**：验证查询参数并提供合理的默认值
5. **响应格式**：在所有 API 中使用一致的响应格式
6. **日志记录**：添加上下文日志以便调试和监控
7. **指标**：暴露 API 性能监控指标

## 监控

API 模块暴露 Prometheus 指标，包括：

- `http_requests_total`：按方法、路径和状态统计的 HTTP 请求总数
- `http_request_duration_seconds`：HTTP 请求延迟直方图
- `http_requests_in_flight`：当前正在处理的 HTTP 请求数

访问指标：`http://localhost:9090/metrics`

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

