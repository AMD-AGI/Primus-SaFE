# Training Performance API 文档

## 概述

Training Performance API 提供了对训练性能数据的查询功能，支持获取可用指标列表和按条件查询指标数据。

## API 端点

### 1. 获取可用指标列表

获取指定 workload 的所有可用训练指标。

**端点：** `GET /api/v1/workloads/:uid/metrics/available`

#### 请求参数

| 参数 | 类型 | 位置 | 必需 | 说明 |
|------|------|------|------|------|
| uid | string | path | 是 | Workload UID |

#### 响应格式

```json
{
  "workload_uid": "workload-12345",
  "metrics": [
    {
      "name": "loss",
      "data_source": ["log", "wandb"],
      "count": 150
    },
    {
      "name": "accuracy",
      "data_source": ["wandb"],
      "count": 100
    },
    {
      "name": "learning_rate",
      "data_source": ["log"],
      "count": 150
    }
  ],
  "total_count": 3
}
```

#### 响应字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| workload_uid | string | Workload UID |
| metrics | array | 指标列表 |
| metrics[].name | string | 指标名称 |
| metrics[].data_source | array | 该指标的数据来源列表 |
| metrics[].count | integer | 该指标的数据点总数 |
| total_count | integer | 总指标数量 |

#### 示例

```bash
# 获取 workload 的所有可用指标
curl -X GET "http://localhost:8080/api/v1/workloads/workload-12345/metrics/available"
```

#### 响应示例

```json
{
  "workload_uid": "workload-12345",
  "metrics": [
    {
      "name": "train/loss",
      "data_source": ["log", "wandb"],
      "count": 500
    },
    {
      "name": "train/accuracy",
      "data_source": ["wandb"],
      "count": 500
    },
    {
      "name": "train/learning_rate",
      "data_source": ["log"],
      "count": 500
    },
    {
      "name": "gpu/utilization",
      "data_source": ["log"],
      "count": 500
    },
    {
      "name": "memory/used_gb",
      "data_source": ["log"],
      "count": 500
    }
  ],
  "total_count": 5
}
```

---

### 2. 获取指标数据

根据条件查询训练指标数据，支持按数据来源、指标名称、时间范围过滤。

**端点：** `GET /api/v1/workloads/:uid/metrics/data`

#### 请求参数

| 参数 | 类型 | 位置 | 必需 | 说明 |
|------|------|------|------|------|
| uid | string | path | 是 | Workload UID |
| data_source | string | query | 否 | 数据来源（如 "log", "wandb", "tensorflow"） |
| metrics | string | query | 否 | 指标名称列表，逗号分隔（不指定则返回所有指标） |
| start | int64 | query | 否 | 开始时间戳（毫秒） |
| end | int64 | query | 否 | 结束时间戳（毫秒） |

**注意：** `start` 和 `end` 必须同时提供或同时不提供。

#### 响应格式

```json
{
  "workload_uid": "workload-12345",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "loss",
      "value": 0.5234,
      "timestamp": 1704067200000,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "accuracy",
      "value": 0.8912,
      "timestamp": 1704067200000,
      "iteration": 100,
      "data_source": "wandb"
    }
  ],
  "total_count": 2
}
```

#### 响应字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| workload_uid | string | Workload UID |
| data_source | string | 查询时指定的数据来源（可能为空） |
| data | array | 指标数据点列表 |
| data[].metric_name | string | 指标名称 |
| data[].value | float64 | 指标值 |
| data[].timestamp | int64 | 时间戳（毫秒） |
| data[].iteration | int32 | 训练步数/迭代次数 |
| data[].data_source | string | 该数据点的数据来源 |
| total_count | integer | 返回的数据点总数 |

#### 使用场景

##### 场景 1: 获取所有指标数据

```bash
# 获取 workload 的所有指标数据
curl -X GET "http://localhost:8080/api/v1/workloads/workload-12345/metrics/data"
```

##### 场景 2: 获取指定来源的数据

```bash
# 只获取来自 wandb 的数据
curl -X GET "http://localhost:8080/api/v1/workloads/workload-12345/metrics/data?data_source=wandb"
```

##### 场景 3: 获取特定指标

```bash
# 只获取 loss 和 accuracy 两个指标
curl -X GET "http://localhost:8080/api/v1/workloads/workload-12345/metrics/data?metrics=loss,accuracy"
```

##### 场景 4: 按时间范围查询

```bash
# 获取指定时间范围内的数据
curl -X GET "http://localhost:8080/api/v1/workloads/workload-12345/metrics/data?start=1704067200000&end=1704153600000"
```

##### 场景 5: 组合查询

```bash
# 获取 wandb 来源的 loss 和 accuracy 指标，在指定时间范围内
curl -X GET "http://localhost:8080/api/v1/workloads/workload-12345/metrics/data?data_source=wandb&metrics=loss,accuracy&start=1704067200000&end=1704153600000"
```

#### 响应示例

```json
{
  "workload_uid": "workload-12345",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "train/loss",
      "value": 2.3456,
      "timestamp": 1704067200000,
      "iteration": 1,
      "data_source": "wandb"
    },
    {
      "metric_name": "train/accuracy",
      "value": 0.1234,
      "timestamp": 1704067200000,
      "iteration": 1,
      "data_source": "wandb"
    },
    {
      "metric_name": "train/loss",
      "value": 1.8765,
      "timestamp": 1704067260000,
      "iteration": 2,
      "data_source": "wandb"
    },
    {
      "metric_name": "train/accuracy",
      "value": 0.3456,
      "timestamp": 1704067260000,
      "iteration": 2,
      "data_source": "wandb"
    }
  ],
  "total_count": 4
}
```

---

## 数据源类型

当前支持的数据源类型：

| 数据源 | 说明 |
|--------|------|
| log | 从训练日志解析的数据 |
| wandb | 从 Weights & Biases API 获取的数据 |
| tensorflow | 从 TensorFlow/TensorBoard 获取的数据 |

---

## 错误响应

### 400 Bad Request

参数错误时返回：

```json
{
  "code": "RequestParameterInvalid",
  "message": "workload_uid is required"
}
```

### 500 Internal Server Error

服务器内部错误时返回：

```json
{
  "code": "InternalError",
  "message": "database query failed"
}
```

---

## 使用流程

### 典型工作流程

1. **获取可用指标**
   ```bash
   GET /api/v1/workloads/{uid}/metrics/available
   ```
   返回该 workload 所有可用的指标名称和数据来源。

2. **选择需要的指标**
   根据返回的指标列表，选择需要查询的指标。

3. **查询指标数据**
   ```bash
   GET /api/v1/workloads/{uid}/metrics/data?metrics=loss,accuracy&data_source=wandb
   ```
   获取具体的指标数据，包含时间戳和迭代次数。

### 数据可视化示例

```javascript
// 1. 获取可用指标
fetch('/api/v1/workloads/workload-12345/metrics/available')
  .then(res => res.json())
  .then(data => {
    console.log('可用指标:', data.metrics);
    
    // 2. 选择需要的指标
    const metrics = data.metrics
      .filter(m => m.name.includes('train'))
      .map(m => m.name)
      .join(',');
    
    // 3. 获取指标数据
    return fetch(`/api/v1/workloads/workload-12345/metrics/data?metrics=${metrics}&data_source=wandb`);
  })
  .then(res => res.json())
  .then(data => {
    // 4. 按指标分组
    const metricGroups = {};
    data.data.forEach(point => {
      if (!metricGroups[point.metric_name]) {
        metricGroups[point.metric_name] = [];
      }
      metricGroups[point.metric_name].push({
        x: point.iteration,
        y: point.value,
        timestamp: point.timestamp
      });
    });
    
    // 5. 绘制图表
    Object.entries(metricGroups).forEach(([name, points]) => {
      console.log(`指标 ${name}:`, points);
      // 使用 Chart.js, ECharts 等绘制图表
    });
  });
```

---

## 性能考虑

### 数据量限制

- 单次查询建议使用时间范围限制
- 对于大量数据，考虑分页或采样
- 建议使用 `metrics` 参数只查询需要的指标

### 优化建议

1. **使用时间范围**
   ```bash
   # 推荐：限制时间范围
   GET /api/v1/workloads/{uid}/metrics/data?start=1704067200000&end=1704153600000
   
   # 不推荐：查询全部历史数据
   GET /api/v1/workloads/{uid}/metrics/data
   ```

2. **只查询需要的指标**
   ```bash
   # 推荐：指定指标
   GET /api/v1/workloads/{uid}/metrics/data?metrics=loss,accuracy
   
   # 不推荐：查询所有指标
   GET /api/v1/workloads/{uid}/metrics/data
   ```

3. **过滤数据源**
   ```bash
   # 推荐：指定数据源
   GET /api/v1/workloads/{uid}/metrics/data?data_source=wandb
   ```

---

## 版本历史

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| 1.0 | 2025-01 | 初始版本，支持可用指标查询和数据查询 |

---

## 相关文档

- [Training Performance Model](../../core/pkg/database/model/training_performance.gen.go)
- [Database Facade](../../core/pkg/database/training_facade.go)
- [WandB Integration](../../../exporters/wandb-exporter/)

