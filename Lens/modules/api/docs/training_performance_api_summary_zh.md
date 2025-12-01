# Training Performance API 实现总结

## 📋 概述

本次实现为 Primus Lens API 添加了两个新的训练性能查询接口，支持：
1. 查询 workload 的所有可用指标
2. 根据条件（数据源、指标列表、时间范围）查询指标数据

## ✅ 已实现功能

### 1. 获取可用指标接口

**接口：** `GET /api/v1/workloads/:uid/metrics/available`

**功能：**
- 查询指定 workload 的所有可用训练指标
- 返回每个指标的数据来源列表
- 统计每个指标的数据点数量

**返回示例：**
```json
{
  "workload_uid": "workload-12345",
  "metrics": [
    {
      "name": "train/loss",
      "data_source": ["log", "wandb"],
      "count": 500
    }
  ],
  "total_count": 1
}
```

### 2. 查询指标数据接口

**接口：** `GET /api/v1/workloads/:uid/metrics/data`

**功能：**
- 支持按 `data_source` 过滤（如 log、wandb、tensorflow）
- 支持按 `metrics` 过滤（指标名称列表，逗号分隔）
- 支持按时间范围过滤（`start` 和 `end` 参数）
- **必须返回时间戳 (`timestamp`) 和步数 (`iteration`)**

**返回示例：**
```json
{
  "workload_uid": "workload-12345",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "train/loss",
      "value": 1.234,
      "timestamp": 1704067200000,
      "iteration": 100,
      "data_source": "wandb"
    }
  ],
  "total_count": 1
}
```

## 📁 文件结构

```
Lens/modules/
├── api/
│   └── pkg/
│       └── api/
│           ├── training_performance.go       # 新增：API Handler 实现
│           ├── training_performance_test.go  # 新增：单元测试
│           └── router.go                     # 修改：添加路由
└── core/
    └── pkg/
        └── database/
            └── training_facade.go            # 修改：添加数据库查询方法
```

## 🔧 实现细节

### 数据库层 (training_facade.go)

添加了 3 个新方法：

```go
// 1. 获取指定 workload 的所有训练性能数据
ListTrainingPerformanceByWorkloadUID(ctx, workloadUid) ([]*model.TrainingPerformance, error)

// 2. 按 workload 和 data_source 过滤
ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, workloadUid, dataSource) ([]*model.TrainingPerformance, error)

// 3. 按 workload、data_source 和时间范围过滤
ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(ctx, workloadUid, dataSource, start, end) ([]*model.TrainingPerformance, error)
```

**特性：**
- `dataSource` 参数为空时不过滤
- 按 `created_at` 升序排序
- 支持 GORM 查询

### API 层 (training_performance.go)

#### Handler 1: `GetAvailableMetrics`

**逻辑：**
1. 获取 workload 的所有训练性能数据
2. 遍历所有记录，统计每个指标的数据来源
3. 返回指标列表及统计信息

**数据结构：**
```go
type MetricInfo struct {
    Name       string   `json:"name"`
    DataSource []string `json:"data_source"`
    Count      int      `json:"count"`
}
```

#### Handler 2: `GetMetricsData`

**逻辑：**
1. 解析查询参数（data_source、metrics、start、end）
2. 根据参数调用相应的数据库查询方法
3. 过滤指定的指标（如果提供 metrics 参数）
4. 构建数据点列表，**包含 timestamp 和 iteration**
5. 返回符合条件的数据

**数据结构：**
```go
type MetricDataPoint struct {
    MetricName string  `json:"metric_name"`
    Value      float64 `json:"value"`
    Timestamp  int64   `json:"timestamp"`   // 毫秒时间戳
    Iteration  int32   `json:"iteration"`   // 训练步数
    DataSource string  `json:"data_source"`
}
```

### 路由配置 (router.go)

```go
workloadGroup.GET(":uid/metrics/available", GetAvailableMetrics)
workloadGroup.GET(":uid/metrics/data", GetMetricsData)
```

## 🎯 使用示例

### 示例 1：查看所有可用指标

```bash
curl -X GET "http://localhost:8080/api/v1/workloads/workload-123/metrics/available"
```

### 示例 2：获取所有指标数据

```bash
curl -X GET "http://localhost:8080/api/v1/workloads/workload-123/metrics/data"
```

### 示例 3：只获取 wandb 数据源

```bash
curl -X GET "http://localhost:8080/api/v1/workloads/workload-123/metrics/data?data_source=wandb"
```

### 示例 4：获取特定指标

```bash
curl -X GET "http://localhost:8080/api/v1/workloads/workload-123/metrics/data?metrics=loss,accuracy"
```

### 示例 5：按时间范围查询

```bash
curl -X GET "http://localhost:8080/api/v1/workloads/workload-123/metrics/data?start=1704067200000&end=1704153600000"
```

### 示例 6：组合查询

```bash
# 查询 wandb 来源的 loss 和 accuracy，时间范围为 1 月 1 日到 1 月 2 日
curl -X GET "http://localhost:8080/api/v1/workloads/workload-123/metrics/data?data_source=wandb&metrics=loss,accuracy&start=1704067200000&end=1704153600000"
```

## 📊 数据源支持

当前支持的 `data_source` 值：

| 值 | 说明 |
|---|---|
| `log` | 从训练日志解析的数据 |
| `wandb` | 从 Weights & Biases API 获取 |
| `tensorflow` | 从 TensorFlow/TensorBoard 获取 |

## ⚡ 性能优化建议

1. **使用时间范围限制**
   ```
   ?start=1704067200000&end=1704153600000
   ```

2. **只查询需要的指标**
   ```
   ?metrics=loss,accuracy
   ```

3. **指定数据源**
   ```
   ?data_source=wandb
   ```

4. **组合使用以减少数据量**
   ```
   ?data_source=wandb&metrics=loss&start=xxx&end=xxx
   ```

## 🧪 测试

已提供单元测试文件：`training_performance_test.go`

**测试覆盖：**
- ✅ 参数验证（缺失 UID、无效时间戳）
- ✅ 基本查询功能
- ✅ 数据源过滤
- ✅ 指标过滤
- ✅ 时间范围过滤
- ✅ 组合查询
- ✅ 类型转换函数

**运行测试：**
```bash
cd Lens/modules/api
go test ./pkg/api -v -run TestGetAvailableMetrics
go test ./pkg/api -v -run TestGetMetricsData
```

## 🔍 数据模型

### TrainingPerformance 表结构

```sql
CREATE TABLE training_performance (
    id           INT PRIMARY KEY AUTO_INCREMENT,
    pod_uuid     VARCHAR(255),
    workload_uid VARCHAR(255),
    performance  JSON,           -- 存储指标数据
    iteration    INT,            -- 训练步数
    created_at   TIMESTAMP,      -- 时间戳
    serial       INT,
    data_source  VARCHAR(50)     -- 数据来源
);
```

### Performance 字段结构

`performance` 是 JSONB 类型，存储格式：

```json
{
  "train/loss": 1.234,
  "train/accuracy": 0.891,
  "train/learning_rate": 0.001,
  "gpu/utilization": 85.5,
  "memory/used_gb": 12.3
}
```

## 🔐 错误处理

### 400 Bad Request

```json
{
  "code": "RequestParameterInvalid",
  "message": "workload_uid is required"
}
```

**触发条件：**
- 缺少 workload_uid
- 无效的时间戳格式

### 500 Internal Server Error

```json
{
  "code": "InternalError",
  "message": "database query failed"
}
```

**触发条件：**
- 数据库连接失败
- 查询执行错误

## 📚 相关文档

- [完整 API 文档](./training_performance_api.md)
- [Training Performance Model](../../core/pkg/database/model/training_performance.gen.go)
- [Database Facade](../../core/pkg/database/training_facade.go)

## ✨ 特性亮点

1. ✅ **完整的时间和步数信息**：每个数据点都包含 `timestamp` 和 `iteration`
2. ✅ **灵活的过滤选项**：支持多维度过滤（数据源、指标、时间）
3. ✅ **高性能查询**：数据库层优化，支持索引查询
4. ✅ **类型安全**：完整的类型定义和转换
5. ✅ **易于扩展**：清晰的分层架构
6. ✅ **完整测试**：提供单元测试覆盖

## 🚀 后续优化方向

1. **分页支持**：对于大量数据，添加分页功能
2. **数据聚合**：支持按时间窗口聚合（如每小时平均值）
3. **缓存机制**：对常用查询结果进行缓存
4. **异步查询**：对于大数据量查询，支持异步返回
5. **数据导出**：支持导出为 CSV、Excel 等格式

## 📝 版本信息

- **版本：** 1.0.0
- **日期：** 2025-01
- **作者：** Primus SaFE Team
- **状态：** ✅ 已完成并测试

