# GPU Aggregation - Dimension Values API

## Overview

This document describes the newly added `dimension-values` API endpoint, which is used to retrieve all possible values for a specified dimension key. This interface complements the existing `dimension-keys` interface, allowing Agents to fully explore and traverse all dimension data.

## Use Cases

When an Agent needs to analyze the reasons for cluster utilization decline, it can follow these steps:

1. **Get available dimension keys**
   ```bash
   GET /v1/gpu-aggregation/dimension-keys?dimension_type=annotation&start_time=...&end_time=...
   ```

2. **Get all values for each key** (New feature)
   ```bash
   GET /v1/gpu-aggregation/dimension-values?dimension_type=annotation&dimension_key=primus-safe.user.name&start_time=...&end_time=...
   ```

3. **Query utilization trends for each value**
   ```bash
   GET /v1/gpu-aggregation/labels/hourly-stats?dimension_type=annotation&dimension_key=primus-safe.user.name&dimension_value=zhangsan&start_time=...&end_time=...
   ```

4. **Group statistics and identify low-utilization dimensions**

## API Endpoint

### Get Dimension Values

Retrieves a list of all possible values for a specific dimension key within the specified time range.

**Endpoint:** `GET /v1/gpu-aggregation/dimension-values`

**Query Parameters:**

| Parameter | Type | Required | Description |
|------|------|------|------|
| `cluster` | string | No | Cluster name (uses default cluster if not specified) |
| `dimension_type` | string | Yes | Dimension type: `label` or `annotation` |
| `dimension_key` | string | Yes | Dimension key (e.g., "team", "primus-safe.user.name") |
| `start_time` | string | Yes | Start time (RFC3339 format) |
| `end_time` | string | Yes | End time (RFC3339 format) |

**Response Example:**

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

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid parameters (e.g., invalid time format, dimension_type is not label/annotation)
- `500 Internal Server Error` - Database or server error

**Examples:**

```bash
# Get all usernames for annotation key "primus-safe.user.name"
curl -X GET "http://localhost:8080/v1/gpu-aggregation/dimension-values?dimension_type=annotation&dimension_key=primus-safe.user.name&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"

# Get all team names for label key "team"
curl -X GET "http://localhost:8080/v1/gpu-aggregation/dimension-values?dimension_type=label&dimension_key=team&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"

# Query with specific cluster
curl -X GET "http://localhost:8080/v1/gpu-aggregation/dimension-values?cluster=gpu-cluster-02&dimension_type=label&dimension_key=project&start_time=2025-11-01T00:00:00Z&end_time=2025-11-07T23:59:59Z"
```

## Python Agent Tool Usage

In the Python Agent, a new `get_available_dimension_values` tool method has been added:

```python
from gpu_usage_agent.tools import GPUAnalysisTools

# Initialize tools
tools = GPUAnalysisTools(api_base_url="http://localhost:8080")

# Get all values for an annotation key
result = tools.get_available_dimension_values(
    dimension_type="annotation",
    dimension_key="primus-safe.user.name",
    time_range_days=7,
    cluster="default"  # Optional
)

# Parse result
import json
data = json.loads(result)
print(f"Found {data['count']} values:")
for value in data['dimension_values']:
    print(f"  - {value}")
```

## Complete Root Cause Analysis Workflow Example

The following is a complete workflow that an Agent should follow to analyze the reasons for cluster utilization decline:

```python
# Step 1: Get cluster baseline utilization
cluster_trend = tools.query_gpu_usage_trend(
    dimension="cluster",
    granularity="day",
    time_range_days=7,
    metric_type="utilization"
)
# Analyze results and confirm that utilization is indeed declining

# Step 2: Get all namespaces
namespaces = tools.get_available_namespaces(time_range_days=7)

# Step 3: Query utilization for each namespace
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

# Step 4: Get all annotation keys
annotation_keys = tools.get_available_dimension_keys(
    dimension_type="annotation",
    time_range_days=7
)

# Step 5: For each key, get all values (new feature)
for key in json.loads(annotation_keys)['dimension_keys']:
    # Get all values for this key
    values_result = tools.get_available_dimension_values(
        dimension_type="annotation",
        dimension_key=key,
        time_range_days=7
    )
    
    # Query utilization for each value
    values = json.loads(values_result)['dimension_values']
    for value in values:
        trend = tools.query_gpu_usage_trend(
            dimension="annotation",
            dimension_value=f"{key}:{value}",
            granularity="day",
            time_range_days=7,
            metric_type="utilization"
        )
        # Analyze and record low-utilization key:value combinations

# Step 6: Group statistics and identify dimensions dragging down overall utilization
# - Sort by average utilization
# - Calculate GPU resource usage for each dimension
# - Analyze impact on overall utilization
```

## Implementation Details

### Backend Implementation (Go)

1. **Database Layer** (`Lens/modules/core/pkg/database/gpu_aggregation_facade.go`)
   - Added `GetDistinctDimensionValues` method
   - Uses GORM's `Distinct` and `Pluck` to query unique dimension values

2. **API Layer** (`Lens/modules/api/pkg/api/gpu_aggregation.go`)
   - Added `DimensionValuesRequest` request struct
   - Added `getDimensionValues` handler function
   - Supports time range filtering and cluster selection

3. **Route Registration** (`Lens/modules/api/pkg/api/router.go`)
   - Registered `/gpu-aggregation/dimension-values` route

### Frontend Implementation (Python)

In `Lens/modules/agents/gpu_usage_agent/tools.py`:
- Added `get_available_dimension_values` method
- Integrated into the tool list for automatic Agent invocation

## Database Query

The new method uses the following SQL logic (simplified representation):

```sql
SELECT DISTINCT dimension_value
FROM label_gpu_hourly_stats
WHERE dimension_type = ?
  AND dimension_key = ?
  AND stat_hour >= ?
  AND stat_hour <= ?
ORDER BY dimension_value
```

## Performance Considerations

- Query performance depends on time range and data volume
- Recommended time range should not exceed 30 days
- Database should have indexes on `dimension_type`, `dimension_key`, `stat_hour`
- Results are automatically sorted alphabetically

## Error Handling

**Common errors and solutions:**

1. **400 Bad Request - Invalid dimension_type**
   - Ensure `dimension_type` is "label" or "annotation"

2. **400 Bad Request - Invalid time format**
   - Ensure time parameters use RFC3339 format (e.g., `2025-11-05T00:00:00Z`)

3. **500 Internal Server Error**
   - Check database connection
   - Check detailed error information in logs

## Comparison with Existing APIs

| API Endpoint | Returns | Purpose |
|---------|---------|------|
| `/dimension-keys` | All keys for a dimension_type | Discover available label/annotation keys |
| `/dimension-values` (New) | All values for a key | Discover possible values for a key |
| `/labels/hourly-stats` | Detailed statistics | Get specific utilization trend data |

## Future Optimization Suggestions

1. **Add pagination support**
   - When the number of values for a key is too large (e.g., >1000), pagination parameters can be added

2. **Add caching**
   - Dimension values don't change much in a short time, caching can be considered

3. **Add filtering and search**
   - Support filtering values by prefix or regex

4. **Return additional metadata**
   - Can return sample count or last update time for each value

## Related Documentation

- [Complete GPU Aggregation API Documentation](./gpu-aggregation.md)
- [Agent Implementation Documentation](../../modules/agents/gpu_usage_agent/README.md)

