# Training Performance API Documentation

This document describes the Training Performance APIs for retrieving training metrics and performance data for workloads.

## Table of Contents

- [Get Data Sources](#get-data-sources)
- [Get Available Metrics](#get-available-metrics)
- [Get Metrics Data](#get-metrics-data)
- [Get Iteration Times](#get-iteration-times)

---

## Get Data Sources

Retrieves all data sources for a specified workload.

### Endpoint

```
GET /workloads/:uid/metrics/sources
```

### Path Parameters

| Parameter | Type   | Required | Description                    |
|-----------|--------|----------|--------------------------------|
| `uid`     | string | Yes      | The unique identifier of the workload |

### Query Parameters

| Parameter | Type   | Required | Description                                                                 |
|-----------|--------|----------|-----------------------------------------------------------------------------|
| `cluster` | string | No       | Cluster name. Priority: specified cluster > default cluster > current cluster |

### Response

| Field         | Type   | Description                              |
|---------------|--------|------------------------------------------|
| `workload_uid`| string | The workload unique identifier           |
| `data_sources`| array  | List of data source information          |
| `total_count` | int    | Total number of data sources             |

#### DataSourceInfo Object

| Field   | Type   | Description                                |
|---------|--------|--------------------------------------------|
| `name`  | string | Data source name (e.g., "log", "wandb", "tensorflow") |
| `count` | int    | Number of data points for this data source |

### Example Request

```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/sources?cluster=my-cluster"
```

### Example Response

```json
{
  "workload_uid": "abc123",
  "data_sources": [
    {
      "name": "wandb",
      "count": 1500
    },
    {
      "name": "log",
      "count": 800
    }
  ],
  "total_count": 2
}
```

### Error Responses

| Status Code | Description                        |
|-------------|------------------------------------|
| 400         | `workload_uid` is required         |
| 500         | Internal server error              |

---

## Get Available Metrics

Retrieves all available metrics for a specified workload.

### Endpoint

```
GET /workloads/:uid/metrics/available
```

### Path Parameters

| Parameter | Type   | Required | Description                    |
|-----------|--------|----------|--------------------------------|
| `uid`     | string | Yes      | The unique identifier of the workload |

### Query Parameters

| Parameter     | Type   | Required | Description                                                                 |
|---------------|--------|----------|-----------------------------------------------------------------------------|
| `cluster`     | string | No       | Cluster name. Priority: specified cluster > default cluster > current cluster |
| `data_source` | string | No       | Filter by data source (e.g., "log", "wandb", "tensorflow")                  |

### Response

| Field         | Type   | Description                              |
|---------------|--------|------------------------------------------|
| `workload_uid`| string | The workload unique identifier           |
| `metrics`     | array  | List of metric information               |
| `total_count` | int    | Total number of available metrics        |

#### MetricInfo Object

| Field         | Type     | Description                                    |
|---------------|----------|------------------------------------------------|
| `name`        | string   | Metric name                                    |
| `data_source` | string[] | List of data sources that contain this metric  |
| `count`       | int      | Total number of data points for this metric    |

### Example Request

```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/available?data_source=wandb"
```

### Example Response

```json
{
  "workload_uid": "abc123",
  "metrics": [
    {
      "name": "loss",
      "data_source": ["wandb", "log"],
      "count": 2300
    },
    {
      "name": "accuracy",
      "data_source": ["wandb"],
      "count": 1500
    },
    {
      "name": "learning_rate",
      "data_source": ["wandb", "log"],
      "count": 2300
    }
  ],
  "total_count": 3
}
```

### Notes

- For `wandb` data source, metadata fields (`step`, `run_id`, `source`, `history`, `created_at`, `updated_at`) are automatically filtered out and not returned as metrics.
- For `log` and `tensorflow` data sources, all fields are treated as metrics.

### Error Responses

| Status Code | Description                        |
|-------------|------------------------------------|
| 400         | `workload_uid` is required         |
| 500         | Internal server error              |

---

## Get Metrics Data

Retrieves data for specified metrics.

### Endpoint

```
GET /workloads/:uid/metrics/data
```

### Path Parameters

| Parameter | Type   | Required | Description                    |
|-----------|--------|----------|--------------------------------|
| `uid`     | string | Yes      | The unique identifier of the workload |

### Query Parameters

| Parameter     | Type   | Required | Description                                                                                     |
|---------------|--------|----------|-------------------------------------------------------------------------------------------------|
| `cluster`     | string | No       | Cluster name. Priority: specified cluster > default cluster > current cluster                   |
| `data_source` | string | No       | Filter by data source (e.g., "log", "wandb", "tensorflow")                                      |
| `metrics`     | string | No       | Comma-separated list of metric names. Supports: `all` (return all metrics), specific metric names, or Grafana format `{metric1,metric2}`. Default: return all metrics |
| `start`       | int64  | No       | Start timestamp in milliseconds (must be used with `end`)                                       |
| `end`         | int64  | No       | End timestamp in milliseconds (must be used with `start`)                                       |

### Response

| Field         | Type   | Description                              |
|---------------|--------|------------------------------------------|
| `workload_uid`| string | The workload unique identifier           |
| `data_source` | string | The data source filter (if specified)    |
| `data`        | array  | List of metric data points               |
| `total_count` | int    | Total number of data points returned     |

#### MetricDataPoint Object

| Field         | Type    | Description                                |
|---------------|---------|--------------------------------------------|
| `metric_name` | string  | The name of the metric                     |
| `value`       | float64 | The metric value                           |
| `timestamp`   | int64   | Timestamp in milliseconds                  |
| `iteration`   | int32   | Training step/iteration number             |
| `data_source` | string  | Data source of this data point             |

### Example Requests

**Get all metrics:**
```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/data"
```

**Get specific metrics:**
```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/data?metrics=loss,accuracy"
```

**Get metrics with Grafana format:**
```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/data?metrics={loss,accuracy}"
```

**Get metrics within time range:**
```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/data?metrics=loss&start=1701734400000&end=1701820800000"
```

**Get metrics from specific data source:**
```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/data?data_source=wandb&metrics=loss"
```

### Example Response

```json
{
  "workload_uid": "abc123",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "loss",
      "value": 0.5432,
      "timestamp": 1701734400000,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "loss",
      "value": 0.4321,
      "timestamp": 1701734460000,
      "iteration": 200,
      "data_source": "wandb"
    },
    {
      "metric_name": "accuracy",
      "value": 0.8765,
      "timestamp": 1701734400000,
      "iteration": 100,
      "data_source": "wandb"
    }
  ],
  "total_count": 3
}
```

### Notes

- NaN values are automatically filtered out and not included in the response.
- For `wandb` data source, metadata fields are automatically excluded.
- The `metrics` parameter supports multiple formats for flexibility with different clients (e.g., Grafana).

### Error Responses

| Status Code | Description                        |
|-------------|------------------------------------|
| 400         | `workload_uid` is required         |
| 400         | Invalid start time format          |
| 400         | Invalid end time format            |
| 500         | Internal server error              |

---

## Get Iteration Times

Retrieves time information for each iteration. This endpoint returns iteration progress data in the same format as the metrics data endpoint.

### Endpoint

```
GET /workloads/:uid/metrics/iteration-times
```

### Path Parameters

| Parameter | Type   | Required | Description                    |
|-----------|--------|----------|--------------------------------|
| `uid`     | string | Yes      | The unique identifier of the workload |

### Query Parameters

| Parameter     | Type   | Required | Description                                                                 |
|---------------|--------|----------|-----------------------------------------------------------------------------|
| `cluster`     | string | No       | Cluster name. Priority: specified cluster > default cluster > current cluster |
| `data_source` | string | No       | Filter by data source (e.g., "log", "wandb", "tensorflow")                  |
| `start`       | int64  | No       | Start timestamp in milliseconds (must be used with `end`)                   |
| `end`         | int64  | No       | End timestamp in milliseconds (must be used with `start`)                   |

### Response

| Field         | Type   | Description                              |
|---------------|--------|------------------------------------------|
| `workload_uid`| string | The workload unique identifier           |
| `data_source` | string | The data source filter (if specified)    |
| `data`        | array  | List of metric data points               |
| `total_count` | int    | Total number of data points returned     |

#### Returned Metrics

This endpoint returns two types of metrics:

| Metric Name        | Description                                      |
|--------------------|--------------------------------------------------|
| `iteration`        | Current iteration/step number                    |
| `target_iteration` | Target iteration number (only if available)      |

### Example Request

```bash
curl -X GET "https://tw325.primus-safe.amd.com/lens/v1/workloads/abc123/metrics/iteration-times?data_source=wandb"
```

### Example Response

```json
{
  "workload_uid": "abc123",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "iteration",
      "value": 100,
      "timestamp": 1701734400000,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "target_iteration",
      "value": 10000,
      "timestamp": 1701734400000,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "iteration",
      "value": 200,
      "timestamp": 1701734460000,
      "iteration": 200,
      "data_source": "wandb"
    },
    {
      "metric_name": "target_iteration",
      "value": 10000,
      "timestamp": 1701734460000,
      "iteration": 200,
      "data_source": "wandb"
    }
  ],
  "total_count": 4
}
```

### Notes

- Duplicate iterations are automatically deduplicated, keeping only the earliest timestamp for each iteration.
- The `target_iteration` metric is only included if it exists in the performance data.
- This endpoint is useful for tracking training progress and estimating completion time.

### Error Responses

| Status Code | Description                        |
|-------------|------------------------------------|
| 400         | `workload_uid` is required         |
| 400         | Invalid start time format          |
| 400         | Invalid end time format            |
| 500         | Internal server error              |

---

## Common Data Types

### Data Sources

The following data sources are supported:

| Data Source  | Description                              |
|--------------|------------------------------------------|
| `log`        | Metrics parsed from training logs        |
| `wandb`      | Metrics from Weights & Biases integration |
| `tensorflow` | Metrics from TensorFlow/TensorBoard      |

### Filtered Metadata Fields (wandb only)

For `wandb` data source, the following fields are considered metadata and are automatically excluded from metrics:

- `step`
- `run_id`
- `source`
- `history`
- `created_at`
- `updated_at`

