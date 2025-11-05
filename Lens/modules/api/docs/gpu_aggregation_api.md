# GPU 聚合数据查询 API 文档

本文档介绍了用于查询 GPU 聚合统计数据的 API 接口。

## API 端点概览

所有 GPU 聚合 API 都在 `/api/v1/gpu-aggregation` 路径下。

| 端点 | 方法 | 描述 |
|------|------|------|
| `/cluster/hourly-stats` | GET | 查询集群级别小时统计 |
| `/namespaces/hourly-stats` | GET | 查询 Namespace 级别小时统计 |
| `/labels/hourly-stats` | GET | 查询 Label/Annotation 级别小时统计 |
| `/snapshots/latest` | GET | 获取最新的 GPU 分配快照 |
| `/snapshots` | GET | 查询历史快照列表 |

---

## 1. 查询集群级别小时统计

获取集群维度的 GPU 分配率和使用率小时统计数据。

### 请求

```
GET /api/v1/gpu-aggregation/cluster/hourly-stats
```

### 查询参数

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `cluster` | string | 否 | 集群名称（为空则使用默认集群） |
| `start_time` | string | 是 | 开始时间（RFC3339 格式，如：2025-11-05T00:00:00Z） |
| `end_time` | string | 是 | 结束时间（RFC3339 格式） |

### 响应示例

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": 1,
      "cluster_name": "cluster-1",
      "stat_hour": "2025-11-05T14:00:00Z",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 96.5,
      "allocation_rate": 75.39,
      "avg_utilization": 68.5,
      "max_utilization": 95.2,
      "min_utilization": 45.3,
      "p50_utilization": 70.1,
      "p95_utilization": 88.7,
      "sample_count": 12,
      "created_at": "2025-11-05T15:00:01Z",
      "updated_at": "2025-11-05T15:00:01Z"
    }
  ]
}
```

### 示例请求

```bash
# 查询最近24小时的集群统计
curl "http://localhost:8080/api/v1/gpu-aggregation/cluster/hourly-stats?start_time=2025-11-04T00:00:00Z&end_time=2025-11-05T00:00:00Z"

# 指定集群查询
curl "http://localhost:8080/api/v1/gpu-aggregation/cluster/hourly-stats?cluster=prod-cluster&start_time=2025-11-04T00:00:00Z&end_time=2025-11-05T00:00:00Z"
```

---

## 2. 查询 Namespace 级别小时统计

获取各个 Namespace 的 GPU 分配和使用情况。

### 请求

```
GET /api/v1/gpu-aggregation/namespaces/hourly-stats
```

### 查询参数

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `cluster` | string | 否 | 集群名称 |
| `namespace` | string | 否 | 命名空间名称（为空则查询所有 namespace） |
| `start_time` | string | 是 | 开始时间（RFC3339 格式） |
| `end_time` | string | 是 | 结束时间（RFC3339 格式） |

### 响应示例

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": 1,
      "cluster_name": "cluster-1",
      "namespace": "ml-training",
      "stat_hour": "2025-11-05T14:00:00Z",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 32.5,
      "avg_utilization": 72.3,
      "max_utilization": 89.5,
      "min_utilization": 55.2,
      "active_workload_count": 5,
      "created_at": "2025-11-05T15:00:01Z",
      "updated_at": "2025-11-05T15:00:01Z"
    },
    {
      "id": 2,
      "cluster_name": "cluster-1",
      "namespace": "inference",
      "stat_hour": "2025-11-05T14:00:00Z",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 48.0,
      "avg_utilization": 65.8,
      "max_utilization": 82.1,
      "min_utilization": 48.3,
      "active_workload_count": 12,
      "created_at": "2025-11-05T15:00:01Z",
      "updated_at": "2025-11-05T15:00:01Z"
    }
  ]
}
```

### 示例请求

```bash
# 查询所有 namespace 的统计
curl "http://localhost:8080/api/v1/gpu-aggregation/namespaces/hourly-stats?start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# 查询特定 namespace
curl "http://localhost:8080/api/v1/gpu-aggregation/namespaces/hourly-stats?namespace=ml-training&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

---

## 3. 查询 Label/Annotation 级别小时统计

获取按照 Label 或 Annotation 分组的 GPU 使用统计。

### 请求

```
GET /api/v1/gpu-aggregation/labels/hourly-stats
```

### 查询参数

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `cluster` | string | 否 | 集群名称 |
| `dimension_type` | string | 是 | 维度类型（`label` 或 `annotation`） |
| `dimension_key` | string | 是 | 维度 key（如：`team`、`project`） |
| `dimension_value` | string | 否 | 维度 value（为空则查询该 key 的所有 value） |
| `start_time` | string | 是 | 开始时间（RFC3339 格式） |
| `end_time` | string | 是 | 结束时间（RFC3339 格式） |

### 响应示例

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": 1,
      "cluster_name": "cluster-1",
      "dimension_type": "label",
      "dimension_key": "team",
      "dimension_value": "ai-research",
      "stat_hour": "2025-11-05T14:00:00Z",
      "allocated_gpu_count": 64.0,
      "avg_utilization": 78.5,
      "max_utilization": 92.3,
      "min_utilization": 62.1,
      "active_workload_count": 8,
      "created_at": "2025-11-05T15:00:01Z",
      "updated_at": "2025-11-05T15:00:01Z"
    },
    {
      "id": 2,
      "cluster_name": "cluster-1",
      "dimension_type": "label",
      "dimension_key": "team",
      "dimension_value": "cv-team",
      "stat_hour": "2025-11-05T14:00:00Z",
      "allocated_gpu_count": 32.5,
      "avg_utilization": 68.2,
      "max_utilization": 85.7,
      "min_utilization": 51.3,
      "active_workload_count": 4,
      "created_at": "2025-11-05T15:00:01Z",
      "updated_at": "2025-11-05T15:00:01Z"
    }
  ]
}
```

### 示例请求

```bash
# 查询 team label 的所有值的统计
curl "http://localhost:8080/api/v1/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# 查询特定 team 的统计
curl "http://localhost:8080/api/v1/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&dimension_value=ai-research&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"

# 查询 annotation 维度
curl "http://localhost:8080/api/v1/gpu-aggregation/labels/hourly-stats?dimension_type=annotation&dimension_key=cost-center&start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

---

## 4. 获取最新的 GPU 分配快照

获取最新一次采样的 GPU 分配快照，包含详细的 workload 信息。

### 请求

```
GET /api/v1/gpu-aggregation/snapshots/latest
```

### 查询参数

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `cluster` | string | 否 | 集群名称 |

### 响应示例

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": 12345,
    "cluster_name": "cluster-1",
    "snapshot_time": "2025-11-05T14:25:00Z",
    "dimension_type": "cluster",
    "dimension_key": "",
    "dimension_value": "",
    "total_gpu_capacity": 128,
    "allocated_gpu_count": 96,
    "allocation_details": {
      "namespaces": {
        "ml-training": {
          "allocated_gpu": 32,
          "utilization": 72.5,
          "workload_count": 5,
          "workloads": [
            {
              "uid": "abc-123",
              "name": "bert-training",
              "namespace": "ml-training",
              "kind": "PyTorchJob",
              "allocated_gpu": 8,
              "utilization": 85.3
            }
          ]
        },
        "inference": {
          "allocated_gpu": 48,
          "utilization": 65.8,
          "workload_count": 12,
          "workloads": []
        }
      },
      "annotations": {}
    },
    "created_at": "2025-11-05T14:25:01Z"
  }
}
```

### 示例请求

```bash
# 获取最新快照
curl "http://localhost:8080/api/v1/gpu-aggregation/snapshots/latest"

# 指定集群
curl "http://localhost:8080/api/v1/gpu-aggregation/snapshots/latest?cluster=prod-cluster"
```

---

## 5. 查询历史快照列表

查询指定时间范围内的历史快照。

### 请求

```
GET /api/v1/gpu-aggregation/snapshots
```

### 查询参数

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `cluster` | string | 否 | 集群名称 |
| `start_time` | string | 否 | 开始时间（RFC3339 格式，默认为24小时前） |
| `end_time` | string | 否 | 结束时间（RFC3339 格式，默认为当前时间） |

### 响应示例

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": 12340,
      "cluster_name": "cluster-1",
      "snapshot_time": "2025-11-05T14:00:00Z",
      "dimension_type": "cluster",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 94,
      "allocation_details": {},
      "created_at": "2025-11-05T14:00:01Z"
    },
    {
      "id": 12345,
      "cluster_name": "cluster-1",
      "snapshot_time": "2025-11-05T14:05:00Z",
      "dimension_type": "cluster",
      "total_gpu_capacity": 128,
      "allocated_gpu_count": 96,
      "allocation_details": {},
      "created_at": "2025-11-05T14:05:01Z"
    }
  ]
}
```

### 示例请求

```bash
# 查询最近24小时的快照（默认）
curl "http://localhost:8080/api/v1/gpu-aggregation/snapshots"

# 查询指定时间范围
curl "http://localhost:8080/api/v1/gpu-aggregation/snapshots?start_time=2025-11-05T00:00:00Z&end_time=2025-11-05T23:59:59Z"
```

---

## 错误响应

所有 API 在失败时返回统一的错误格式：

```json
{
  "code": 400,
  "message": "Invalid request parameters",
  "error": "Invalid start_time format"
}
```

常见错误码：
- `400` - 请求参数错误
- `404` - 数据不存在
- `500` - 服务器内部错误

---

## 使用场景示例

### 场景1：查看集群GPU利用率趋势

```bash
# 查询最近7天的集群统计
START_TIME=$(date -u -d '7 days ago' +"%Y-%m-%dT00:00:00Z")
END_TIME=$(date -u +"%Y-%m-%dT23:59:59Z")

curl "http://localhost:8080/api/v1/gpu-aggregation/cluster/hourly-stats?start_time=$START_TIME&end_time=$END_TIME"
```

### 场景2：对比不同团队的GPU使用情况

```bash
# 查询今天各团队的GPU使用统计
TODAY_START=$(date -u +"%Y-%m-%dT00:00:00Z")
TODAY_END=$(date -u +"%Y-%m-%dT23:59:59Z")

curl "http://localhost:8080/api/v1/gpu-aggregation/labels/hourly-stats?dimension_type=label&dimension_key=team&start_time=$TODAY_START&end_time=$TODAY_END"
```

### 场景3：监控特定namespace的GPU使用

```bash
# 实时查看最新快照中 ml-training namespace 的情况
curl "http://localhost:8080/api/v1/gpu-aggregation/snapshots/latest" | jq '.data.allocation_details.namespaces["ml-training"]'
```

### 场景4：生成GPU使用报告

```bash
# 查询本月所有namespace的统计数据
MONTH_START=$(date -u +"%Y-%m-01T00:00:00Z")
MONTH_END=$(date -u +"%Y-%m-%dT23:59:59Z")

curl "http://localhost:8080/api/v1/gpu-aggregation/namespaces/hourly-stats?start_time=$MONTH_START&end_time=$MONTH_END" > monthly_gpu_report.json
```

---

## 注意事项

1. **时间格式**：所有时间参数必须使用 RFC3339 格式（如：`2025-11-05T14:00:00Z`）
2. **时区**：建议使用 UTC 时区（以 `Z` 结尾）
3. **数据延迟**：小时统计数据在每小时结束后约1-2分钟内生成
4. **快照频率**：默认每5分钟采样一次
5. **数据保留**：建议定期清理历史数据以节省存储空间

---

## 相关文档

- [GPU 聚合方案设计](../../docs/gpu_report_solution_summary.md)
- [数据库表结构](../../modules/core/pkg/database/migrations/gpu_usage_aggregation.sql)
- [Job 实现](../../modules/jobs/pkg/jobs/gpu_aggregation/gpu_aggregation_job.go)

