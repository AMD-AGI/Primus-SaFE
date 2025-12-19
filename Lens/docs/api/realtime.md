# Real-time Status API Documentation

## Overview

The Real-time Status API provides optimized endpoints for monitoring cluster status in real-time with built-in caching for high performance. These APIs are designed for dashboard monitoring and real-time query scenarios.

**Base Path**: `/api/v1/realtime`

**Key Features**:
- 30-second caching for optimal performance
- Selective field inclusion to reduce payload size
- Comprehensive cluster status snapshot
- Running tasks monitoring

---

## Authentication

All endpoints require authentication using the existing Lens API authentication mechanism.

---

## Endpoints

### 1. Get Real-time Cluster Status

Get an optimized real-time snapshot of cluster status.

#### Request

```http
GET /api/v1/realtime/status
```

#### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| cluster | string | Yes | Cluster name |
| include | string[] | No | Optional fields to include: nodes, alerts, events |

#### Response

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "cluster": "prod-cluster",
    "timestamp": "2025-12-19T15:30:00Z",
    "current_gpu_usage": {
      "total_gpus": 80,
      "allocated_gpus": 64,
      "utilized_gpus": 56,
      "allocation_rate": 80.0,
      "utilization_rate": 75.5
    },
    "running_tasks": 32,
    "available_resources": {
      "available_gpus": 16,
      "available_nodes": 4,
      "max_contiguous_gpu": 8
    },
    "alerts": [],
    "nodes": [
      {
        "node_name": "gpu-node-1",
        "status": "Ready",
        "total_gpus": 8,
        "allocated_gpus": 6,
        "utilization": 78.3
      }
    ],
    "recent_events": [
      {
        "timestamp": "2025-12-19T15:29:00Z",
        "type": "PodCreated",
        "object": "ml-team/training-job-1",
        "message": "Pod created with 4 GPUs"
      }
    ]
  }
}
```

#### Field Descriptions

**current_gpu_usage**:
- `total_gpus`: Total GPU count in cluster
- `allocated_gpus`: Number of allocated GPUs
- `utilized_gpus`: Number of GPUs with >50% utilization
- `allocation_rate`: Percentage of allocated GPUs
- `utilization_rate`: Average GPU utilization percentage

**available_resources**:
- `available_gpus`: Total available (unallocated) GPUs
- `available_nodes`: Number of nodes with available GPUs
- `max_contiguous_gpu`: Largest contiguous GPU block available

#### Example Requests

**Basic request (minimal payload):**
```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Include nodes and alerts:**
```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster&include=nodes&include=alerts" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Include all optional fields:**
```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster&include=nodes&include=alerts&include=events" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

### 2. Get Running GPU Tasks

Get a list of currently running GPU tasks (pods).

#### Request

```http
GET /api/v1/realtime/running-tasks
```

#### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| cluster | string | Yes | Cluster name |
| namespace | string | No | Filter by namespace |

#### Response

```json
{
  "meta": {
    "code": 2000,
    "message": "OK"
  },
  "data": {
    "cluster": "prod-cluster",
    "timestamp": "2025-12-19T15:30:00Z",
    "total_tasks": 32,
    "tasks": [
      {
        "pod_uid": "abc-123-def-456",
        "pod_name": "training-job-1-worker-0",
        "namespace": "ml-team",
        "workload_type": "Job",
        "workload_name": "training-job-1",
        "node_name": "gpu-node-1",
        "allocated_gpus": 4,
        "running_time_seconds": 3600,
        "started_at": "2025-12-19T14:30:00Z",
        "owner": "job-abc-123"
      }
    ]
  }
}
```

#### Example Requests

**All running tasks:**
```bash
curl -X GET "http://lens-api/api/v1/realtime/running-tasks?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Filter by namespace:**
```bash
curl -X GET "http://lens-api/api/v1/realtime/running-tasks?cluster=prod-cluster&namespace=ml-team" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

## Caching Behavior

### Cache Strategy

Both endpoints implement intelligent caching:

- **Cache Duration**: 30 seconds
- **Cache Key**: Cluster-specific (separate cache per cluster)
- **Cache Invalidation**: Automatic TTL-based expiration

### Cache Benefits

1. **Reduced Latency**: Cached responses return in <100ms
2. **Lower Database Load**: Reduces query frequency by 30x
3. **Scalability**: Supports high-frequency dashboard polling

### Cache Considerations

- Data may be up to 30 seconds stale
- For critical real-time decisions, consider cache age
- Cache is cluster-specific (no cross-cluster contamination)

---

## Use Cases

### 1. Dashboard Real-time Monitoring

Poll cluster status every 30 seconds for dashboard updates:

```python
import requests
import time

def monitor_cluster(cluster, interval=30):
    while True:
        response = requests.get(
            f"{API_BASE}/api/v1/realtime/status",
            params={
                "cluster": cluster,
                "include": ["nodes", "alerts"]
            },
            headers=auth_headers
        )
        
        data = response.json()["data"]
        print(f"Cluster: {data['cluster']}")
        print(f"Allocation: {data['current_gpu_usage']['allocation_rate']}%")
        print(f"Available GPUs: {data['available_resources']['available_gpus']}")
        
        time.sleep(interval)

monitor_cluster("prod-cluster")
```

### 2. Resource Availability Check

Check if cluster has capacity for new job:

```python
def can_schedule_job(cluster, required_gpus):
    response = requests.get(
        f"{API_BASE}/api/v1/realtime/status",
        params={"cluster": cluster},
        headers=auth_headers
    )
    
    data = response.json()["data"]
    resources = data["available_resources"]
    
    return {
        "can_schedule": resources["max_contiguous_gpu"] >= required_gpus,
        "available_gpus": resources["available_gpus"],
        "available_nodes": resources["available_nodes"]
    }

result = can_schedule_job("prod-cluster", 8)
if result["can_schedule"]:
    print("Job can be scheduled")
else:
    print(f"Insufficient resources. Max contiguous: {result['max_contiguous_gpu']}")
```

### 3. Running Tasks Monitoring

Monitor long-running tasks:

```python
def find_long_running_tasks(cluster, threshold_hours=24):
    response = requests.get(
        f"{API_BASE}/api/v1/realtime/running-tasks",
        params={"cluster": cluster},
        headers=auth_headers
    )
    
    data = response.json()["data"]
    threshold_seconds = threshold_hours * 3600
    
    long_running = [
        task for task in data["tasks"]
        if task["running_time_seconds"] > threshold_seconds
    ]
    
    return long_running

long_tasks = find_long_running_tasks("prod-cluster", threshold_hours=48)
for task in long_tasks:
    hours = task["running_time_seconds"] / 3600
    print(f"{task['pod_name']}: {hours:.1f} hours")
```

### 4. Namespace Resource Usage

Monitor resource usage by namespace:

```python
def get_namespace_usage(cluster, namespace):
    response = requests.get(
        f"{API_BASE}/api/v1/realtime/running-tasks",
        params={
            "cluster": cluster,
            "namespace": namespace
        },
        headers=auth_headers
    )
    
    data = response.json()["data"]
    tasks = data["tasks"]
    
    total_gpus = sum(task["allocated_gpus"] for task in tasks)
    
    return {
        "namespace": namespace,
        "running_tasks": len(tasks),
        "total_gpus": total_gpus,
        "tasks": tasks
    }

usage = get_namespace_usage("prod-cluster", "ml-team")
print(f"Namespace: {usage['namespace']}")
print(f"Running tasks: {usage['running_tasks']}")
print(f"Total GPUs: {usage['total_gpus']}")
```

---

## Performance Characteristics

### Response Times

| Scenario | Response Time | Notes |
|----------|---------------|-------|
| Cache hit | < 100ms | Typical case |
| Cache miss | 500-1000ms | First request or after TTL |
| With all includes | 1000-1500ms | Cache miss with full data |

### Recommended Polling Intervals

| Use Case | Interval | Reason |
|----------|----------|--------|
| Dashboard | 30 seconds | Matches cache TTL |
| Monitoring | 60 seconds | Reduces API load |
| Alerting | 15 seconds | Faster detection |

### Rate Limiting

- Default: 500 requests per minute per endpoint
- Burst: 1000 requests
- Recommended: Poll at cache TTL interval (30s)

---

## Best Practices

### 1. Optimize Payload Size

Only include fields you need:

```bash
# Good - minimal payload
GET /api/v1/realtime/status?cluster=prod

# Better - only include needed fields
GET /api/v1/realtime/status?cluster=prod&include=nodes

# Avoid - unnecessary data transfer
GET /api/v1/realtime/status?cluster=prod&include=nodes&include=alerts&include=events
```

### 2. Respect Cache TTL

Poll at 30-second intervals to maximize cache benefits:

```python
# Good - aligns with cache TTL
time.sleep(30)

# Bad - too frequent, bypasses cache benefits
time.sleep(5)
```

### 3. Handle Stale Data

Be aware of 30-second cache window:

```python
def get_status_with_age(cluster):
    response = requests.get(
        f"{API_BASE}/api/v1/realtime/status",
        params={"cluster": cluster}
    )
    
    data = response.json()["data"]
    timestamp = datetime.fromisoformat(data["timestamp"].replace("Z", "+00:00"))
    age_seconds = (datetime.now(timezone.utc) - timestamp).total_seconds()
    
    if age_seconds > 60:
        print(f"Warning: Data is {age_seconds}s old")
    
    return data
```

### 4. Error Handling

Implement proper error handling:

```python
def safe_get_status(cluster, retries=3):
    for attempt in range(retries):
        try:
            response = requests.get(
                f"{API_BASE}/api/v1/realtime/status",
                params={"cluster": cluster},
                headers=auth_headers,
                timeout=5
            )
            response.raise_for_status()
            return response.json()["data"]
        except requests.exceptions.RequestException as e:
            if attempt == retries - 1:
                raise
            time.sleep(2 ** attempt)  # Exponential backoff
```

---

## Error Responses

Standard error responses:

```json
{
  "meta": {
    "code": 4000,
    "message": "Invalid parameter: cluster is required"
  },
  "data": null
}
```

### Common Error Codes

| Code | Description |
|------|-------------|
| 2000 | Success |
| 4000 | Invalid parameter |
| 4004 | Cluster not found |
| 5000 | Internal server error |

---

## Support

For issues or questions:
- Documentation: See implementation summary
- Issues: Contact Lens API Team

---

**Last Updated**: 2025-12-19  
**API Version**: v1.0

