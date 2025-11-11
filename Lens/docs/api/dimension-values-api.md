# GPU Aggregation - Dimension Values API

## 概述

本文档描述了新增的 `dimension-values` API 端点，用于获取指定 dimension key 的所有可能的 values 列表。这个接口是对现有 `dimension-keys` 接口的补充，使得 Agent 可以完整地探索和遍历所有维度数据。

## 使用场景

当 Agent 需要分析集群使用率下降的原因时，可以按照以下步骤进行：

1. **获取可用的 dimension keys**
   ```bash
   GET /v1/gpu-aggregation/dimension-keys?dimension_type=annotation&start_time=...&end_time=...
   ```

2. **获取每个 key 的所有 values**（新增功能）
   ```bash
   GET /v1/gpu-aggregation/dimension-values?dimension_type=annotation&dimension_key=primus-safe.user.name&start_time=...&end_time=...
   ```

3. **查询每个 value 的使用率趋势**
   ```bash
   GET /v1/gpu-aggregation/labels/hourly-stats?dimension_type=annotation&dimension_key=primus-safe.user.name&dimension_value=zhangsan&start_time=...&end_time=...
   ```

4. **分组统计并找出低使用率的维度**

## API 端点

### Get Dimension Values

获取指定时间范围内某个 dimension key 的所有可能值列表。

**端点:** `GET /v1/gpu-aggregation/dimension-values`

**查询参数:**

| 参数 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `cluster` | string | 否 | 集群名称（不指定则使用默认集群） |
| `dimension_type` | string | 是 | 维度类型：`label` 或 `annotation` |
| `dimension_key` | string | 是 | 维度 key（如 "team", "primus-safe.user.name"） |
| `start_time` | string | 是 | 开始时间（RFC3339 格式） |
| `end_time` | string | 是 | 结束时间（RFC3339 格式） |

**响应示例:**

```json
{
  "code": 2000,
  "message": "success",
  "data": [
    "zhangsan",
    "lisi",
    "wangwu",
    "zhaoliu"
  ],
  "traceId": "trace-xyz789"
}
```

**状态码:**
- `200 OK` - 成功
- `400 Bad Request` - 无效参数（如时间格式错误、dimension_type 不是 label/annotation）
- `500 Internal Server Error` - 数据库或服务器错误

**示例:**

```bash
# 获取 annotation key "primus-safe.user.name" 的所有用户名
curl -X GET "http://localhost:8080/v1/gpu-aggregation/dimension-values?dimension_type=annotation&dimension_key=primus-safe.user.name&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"

# 获取 label key "team" 的所有团队名称
curl -X GET "http://localhost:8080/v1/gpu-aggregation/dimension-values?dimension_type=label&dimension_key=team&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"

# 指定集群查询
curl -X GET "http://localhost:8080/v1/gpu-aggregation/dimension-values?cluster=gpu-cluster-02&dimension_type=label&dimension_key=project&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"
```

## Python Agent 工具使用

在 Python Agent 中，新增了 `get_available_dimension_values` 工具方法：

```python
from gpu_usage_agent.tools import GPUAnalysisTools

# 初始化工具
tools = GPUAnalysisTools(api_base_url="http://localhost:8080")

# 获取某个 annotation key 的所有 values
result = tools.get_available_dimension_values(
    dimension_type="annotation",
    dimension_key="primus-safe.user.name",
    time_range_days=7,
    cluster="default"  # 可选
)

# 解析结果
import json
data = json.loads(result)
print(f"Found {data['count']} values:")
for value in data['dimension_values']:
    print(f"  - {value}")
```

## 完整的根因分析流程示例

以下是 Agent 应该遵循的完整流程来分析集群使用率下降的原因：

```python
# 步骤 1: 获取集群基线使用率
cluster_trend = tools.query_gpu_usage_trend(
    dimension="cluster",
    granularity="day",
    time_range_days=7,
    metric_type="utilization"
)
# 分析结果，确认使用率确实在下降

# 步骤 2: 获取所有 namespaces
namespaces = tools.get_available_namespaces(time_range_days=7)

# 步骤 3: 查询每个 namespace 的使用率
namespace_stats = {}
for ns in json.loads(namespaces)['namespaces']:
    trend = tools.query_gpu_usage_trend(
        dimension="namespace",
        dimension_value=ns,
        granularity="day",
        time_range_days=7,
        metric_type="utilization"
    )
    namespace_stats[ns] = json.loads(trend)

# 步骤 4: 获取所有 annotation keys
annotation_keys = tools.get_available_dimension_keys(
    dimension_type="annotation",
    time_range_days=7
)

# 步骤 5: 对于每个 key，获取所有 values（新增功能）
for key in json.loads(annotation_keys)['dimension_keys']:
    # 获取该 key 的所有 values
    values_result = tools.get_available_dimension_values(
        dimension_type="annotation",
        dimension_key=key,
        time_range_days=7
    )
    
    # 查询每个 value 的使用率
    values = json.loads(values_result)['dimension_values']
    for value in values:
        trend = tools.query_gpu_usage_trend(
            dimension="annotation",
            dimension_value=f"{key}:{value}",
            granularity="day",
            time_range_days=7,
            metric_type="utilization"
        )
        # 分析并记录低使用率的 key:value 组合

# 步骤 6: 分组统计，找出拉低整体利用率的维度
# - 按平均使用率排序
# - 计算每个维度占用的 GPU 资源量
# - 分析对整体利用率的影响
```

## 实现细节

### 后端实现（Go）

1. **数据库层** (`Lens/modules/core/pkg/database/gpu_aggregation_facade.go`)
   - 添加了 `GetDistinctDimensionValues` 方法
   - 使用 GORM 的 `Distinct` 和 `Pluck` 查询不重复的 dimension values

2. **API 层** (`Lens/modules/api/pkg/api/gpu_aggregation.go`)
   - 添加了 `DimensionValuesRequest` 请求结构体
   - 添加了 `getDimensionValues` 处理函数
   - 支持时间范围过滤和集群选择

3. **路由注册** (`Lens/modules/api/pkg/api/router.go`)
   - 注册了 `/gpu-aggregation/dimension-values` 路由

### 前端实现（Python）

在 `Lens/modules/agents/gpu_usage_agent/tools.py` 中：
- 添加了 `get_available_dimension_values` 方法
- 集成到工具列表中，Agent 可以自动调用

## 数据库查询

新方法使用以下 SQL 逻辑（简化表示）：

```sql
SELECT DISTINCT dimension_value
FROM label_gpu_hourly_stats
WHERE dimension_type = ?
  AND dimension_key = ?
  AND stat_hour >= ?
  AND stat_hour <= ?
ORDER BY dimension_value
```

## 性能考虑

- 查询性能取决于时间范围和数据量
- 建议时间范围不超过 30 天
- 数据库中 `dimension_type`, `dimension_key`, `stat_hour` 应该有索引
- 结果会自动按字母顺序排序

## 错误处理

**常见错误及解决方案:**

1. **400 Bad Request - Invalid dimension_type**
   - 确保 `dimension_type` 是 "label" 或 "annotation"

2. **400 Bad Request - Invalid time format**
   - 确保时间参数使用 RFC3339 格式（如 `2025-11-05T00:00:00Z`）

3. **500 Internal Server Error**
   - 检查数据库连接
   - 检查日志中的详细错误信息

## 与现有 API 的对比

| API 端点 | 返回内容 | 用途 |
|---------|---------|------|
| `/dimension-keys` | 某个 dimension_type 的所有 keys | 发现有哪些 label/annotation keys |
| `/dimension-values`（新） | 某个 key 的所有 values | 发现某个 key 有哪些可能的值 |
| `/labels/hourly-stats` | 详细的统计数据 | 获取具体的使用率趋势数据 |

## 后续优化建议

1. **添加分页支持**
   - 当某个 key 的 values 数量过多时（如 >1000），可以添加分页参数

2. **添加缓存**
   - dimension values 在短时间内变化不大，可以考虑缓存

3. **添加过滤和搜索**
   - 支持通过前缀或正则表达式过滤 values

4. **返回额外元数据**
   - 可以返回每个 value 的样本数量或最后更新时间

## 相关文档

- [GPU Aggregation API 完整文档](./gpu-aggregation.md)
- [Agent 实现文档](../../modules/agents/gpu_usage_agent/README.md)

