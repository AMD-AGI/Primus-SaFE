# Log API

Log query API provides log query capabilities for workloads and services.

## API List

### 1. Get Workload Logs

Query aggregated workload logs (based on OpenSearch).

**Endpoint**: `POST /api/v1/workloads/{WorkloadId}/logs`

**Authentication Required**: Yes

**Path Parameters**:
- `WorkloadId`: Workload ID

**Request Example**:
```json
{
  "since": "2025-01-15T10:00:00.000Z",
  "until": "2025-01-15T11:00:00.000Z",
  "offset": 0,
  "limit": 100,
  "order": "asc",
  "keywords": ["error", "timeout"],
  "podNames": "pod-a,pod-b",
  "dispatchCount": 1,
  "nodeNames": "node-1,node-2"
}
```

**Request Parameters**:

| Parameter | Type | Required | Description                                                                                    |
|-----------|------|----------|------------------------------------------------------------------------------------------------|
| since | string | No | Start time (RFC3339 with milliseconds); default depends on workload creation time or last 7 days |
| until | string | No | End time (RFC3339 with milliseconds); default now                                              |
| offset | int | No | Pagination offset, default 0; must be >= 0 and < max docs(10000)                               |
| limit | int | No | Page size, default 100; constrained by max docs-per-query                                      |
| order | string | No | Time sort order: asc/desc; default asc                                                         |
| keywords | []string | No | AND search                              |
| podNames | string | No | Filter by pod names (comma-separated, OR filter)                                               |
| dispatchCount | int | No | Filter by workload dispatch/run number; 0 means all                                            |
| nodeNames | string | No | Filter by node names (comma-separated, OR filter); ignored if podNames is set                  |

**Response Example**:
```json
{
  "took": 3,
  "hits": {
    "total": {
      "value": 1
    },
    "hits": [{
      "_id": "1miG6Y0BNjv7ZKdtSWBZ",
      "_source": {
        "@timestamp": "2024-02-27T07:45:08.221Z",
        "stream": "stdout",
        "message": "[2024-02-27 15:45:08] iteration 13 / 118 | consumed samples: 37440 | consumed tokens: 153354240 | elapsed time per iteration(ms): 17658.3 | learning rate: 7.350 | global batch size: 2880 | lm loss: 6.233628E-01 | loss scale: 1.0 |total grad norm: 2.773 | actual seqlen: 4096 | number of skipped iterations: 0 | number of nan iterations: 0 | samples per second: 163.096 | TFLOPs: 367.59 | ",
        "kubernetes": {
          "pod_name": "test-pretrain-master-0",
          "labels": {
            "primus_safe.workload.dispatch.count": "1"
          },
          "host": "tus1-p0-g1",
          "container_name": "pytorch"
        }
      }
    }]
  }
}
```

**Field Description**:

This is a typical OpenSearch search response example. The response structure includes:
- `took`: The time taken to execute the search query (in milliseconds)
- `hits`: Contains the search results
  - `total.value`: The total number of matching documents
  - `hits[]`: Array of actual search hits, each containing:
    - `_id`: The document ID
    - `_source`: The actual log document data including timestamp, stream type, message content, and Kubernetes metadata (pod name, labels, host, container name)
---

### 2. Get Workload Log Context

Get context (N lines before and after) for a specific log line.

**Endpoint**: `POST /api/v1/workloads/{WorkloadId}/logs/{DocumentId}/context`

**Authentication Required**: Yes

**Path Parameters**:
- `WorkloadId`: Workload ID
- `docId`: Log document ID

**Request Parameters**:

Query parameters are the same as above (Get Workload Logs).


### 3. Get Service Log

Query system service logs.

**Endpoint**: `POST /api/v1/service/{ServiceName}/logs`

**Authentication Required**: Yes

**Path Parameters**:
- `ServiceName`: `primus-safe-apiserver`, `primus-safe-resource-manager`, `primus-safe-job-manager`, `primus-safe-webhooks`

**Request Parameters**:

Query parameters are the same as above (Get Workload Logs).

---


## Query Description

### Keyword Search
- Case sensitive
- Supports `span_query` for OpenSearch: spaces between keywords enable proximity searches within the text.

### Limitations
- Single query returns maximum 10,000 log lines
- Recommend using time range to narrow query scope

## Notes

- Log query function depends on OpenSearch
- Logs are retained for 30 days by default
- Large log queries may impact performance
