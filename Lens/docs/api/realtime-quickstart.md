# Real-time Status API Quickstart Guide

## Quick Start

This guide helps you quickly get started with the Real-time Status API for monitoring cluster status.

---

## Prerequisites

- Access to Lens API endpoint
- Valid authentication token
- Cluster name

---

## Basic Usage

### 1. Get Cluster Status (Minimal)

Get basic cluster status with core metrics only:

```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response:**
```json
{
  "meta": {"code": 2000, "message": "OK"},
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
    }
  }
}
```

---

### 2. Get Running Tasks

List all currently running GPU tasks:

```bash
curl -X GET "http://lens-api/api/v1/realtime/running-tasks?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response:**
```json
{
  "meta": {"code": 2000, "message": "OK"},
  "data": {
    "cluster": "prod-cluster",
    "timestamp": "2025-12-19T15:30:00Z",
    "total_tasks": 32,
    "tasks": [
      {
        "pod_uid": "abc-123",
        "pod_name": "training-job-1-worker-0",
        "namespace": "ml-team",
        "workload_type": "Job",
        "workload_name": "abc-123",
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

---

## Common Scenarios

### Scenario 1: Check Resource Availability

**Question**: Can I schedule a job that needs 8 GPUs?

```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN" | jq '.data.available_resources'
```

**Check:**
- If `max_contiguous_gpu >= 8`: Yes, you can schedule
- If `max_contiguous_gpu < 8`: No, need to wait

---

### Scenario 2: Monitor Cluster Utilization

**Question**: What's the current cluster utilization?

```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN" | jq '.data.current_gpu_usage'
```

**Key Metrics:**
- `allocation_rate`: % of GPUs allocated
- `utilization_rate`: % of actual GPU usage
- `utilized_gpus`: GPUs with >50% utilization

---

### Scenario 3: Find Long-Running Tasks

**Question**: Which tasks have been running for more than 24 hours?

```bash
curl -X GET "http://lens-api/api/v1/realtime/running-tasks?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN" | \
  jq '.data.tasks[] | select(.running_time_seconds > 86400)'
```

---

### Scenario 4: Monitor Namespace Usage

**Question**: How many GPUs is my team using?

```bash
curl -X GET "http://lens-api/api/v1/realtime/running-tasks?cluster=prod-cluster&namespace=ml-team" \
  -H "Authorization: Bearer YOUR_TOKEN" | \
  jq '[.data.tasks[].allocated_gpus] | add'
```

---

## Python Examples

### Example 1: Simple Status Check

```python
import requests

API_BASE = "http://lens-api"
TOKEN = "your-token"

def get_cluster_status(cluster):
    response = requests.get(
        f"{API_BASE}/api/v1/realtime/status",
        params={"cluster": cluster},
        headers={"Authorization": f"Bearer {TOKEN}"}
    )
    return response.json()["data"]

# Usage
status = get_cluster_status("prod-cluster")
print(f"Allocation: {status['current_gpu_usage']['allocation_rate']}%")
print(f"Available GPUs: {status['available_resources']['available_gpus']}")
```

---

### Example 2: Resource Availability Check

```python
def can_schedule_job(cluster, required_gpus):
    status = get_cluster_status(cluster)
    max_contiguous = status["available_resources"]["max_contiguous_gpu"]
    
    if max_contiguous >= required_gpus:
        return True, f"Can schedule on {status['available_resources']['available_nodes']} nodes"
    else:
        return False, f"Max contiguous GPUs: {max_contiguous}"

# Usage
can_schedule, message = can_schedule_job("prod-cluster", 8)
print(f"Can schedule 8-GPU job: {can_schedule}")
print(message)
```

---

### Example 3: Dashboard Monitor

```python
import time

def monitor_cluster(cluster, interval=30):
    """Monitor cluster status every 30 seconds"""
    while True:
        status = get_cluster_status(cluster)
        
        gpu_usage = status["current_gpu_usage"]
        resources = status["available_resources"]
        
        print(f"\n=== {status['cluster']} Status ===")
        print(f"Time: {status['timestamp']}")
        print(f"Allocation: {gpu_usage['allocation_rate']:.1f}%")
        print(f"Utilization: {gpu_usage['utilization_rate']:.1f}%")
        print(f"Available GPUs: {resources['available_gpus']}")
        print(f"Running Tasks: {status['running_tasks']}")
        
        time.sleep(interval)

# Usage
monitor_cluster("prod-cluster")
```

---

### Example 4: Find Long-Running Tasks

```python
def find_long_running_tasks(cluster, threshold_hours=24):
    response = requests.get(
        f"{API_BASE}/api/v1/realtime/running-tasks",
        params={"cluster": cluster},
        headers={"Authorization": f"Bearer {TOKEN}"}
    )
    
    data = response.json()["data"]
    threshold_seconds = threshold_hours * 3600
    
    long_running = [
        task for task in data["tasks"]
        if task["running_time_seconds"] > threshold_seconds
    ]
    
    return long_running

# Usage
long_tasks = find_long_running_tasks("prod-cluster", threshold_hours=48)
for task in long_tasks:
    hours = task["running_time_seconds"] / 3600
    print(f"{task['pod_name']}: {hours:.1f} hours, {task['allocated_gpus']} GPUs")
```

---

## Advanced Usage

### Include Optional Fields

Get detailed node information:

```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster&include=nodes" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Include all optional fields:

```bash
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster&include=nodes&include=alerts&include=events" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

### Filter by Namespace

Get running tasks for specific namespace:

```bash
curl -X GET "http://lens-api/api/v1/realtime/running-tasks?cluster=prod-cluster&namespace=ml-team" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

## Performance Tips

### 1. Respect Cache TTL

The API caches responses for 30 seconds. Poll at this interval for best performance:

```python
# Good - aligns with cache
time.sleep(30)

# Bad - too frequent
time.sleep(5)
```

### 2. Minimize Payload Size

Only request fields you need:

```bash
# Minimal - fastest
GET /api/v1/realtime/status?cluster=prod

# With nodes - moderate
GET /api/v1/realtime/status?cluster=prod&include=nodes

# Everything - slowest
GET /api/v1/realtime/status?cluster=prod&include=nodes&include=alerts&include=events
```

### 3. Handle Cache Age

Be aware of potential 30-second staleness:

```python
from datetime import datetime, timezone

status = get_cluster_status("prod-cluster")
timestamp = datetime.fromisoformat(status["timestamp"].replace("Z", "+00:00"))
age = (datetime.now(timezone.utc) - timestamp).total_seconds()

if age > 60:
    print(f"Warning: Data is {age}s old")
```

---

## Common Issues

### Issue 1: "Invalid parameter: cluster is required"

**Solution**: Always provide the `cluster` parameter:

```bash
# Wrong
curl -X GET "http://lens-api/api/v1/realtime/status"

# Correct
curl -X GET "http://lens-api/api/v1/realtime/status?cluster=prod-cluster"
```

---

### Issue 2: "Cluster not found"

**Solution**: Verify cluster name is correct:

```bash
# Check available clusters first
curl -X GET "http://lens-api/api/v1/clusters" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

### Issue 3: Empty tasks list

**Possible Reasons:**
1. No GPU tasks are currently running
2. All tasks have completed
3. Namespace filter excludes all tasks

**Solution**: Check without namespace filter:

```bash
curl -X GET "http://lens-api/api/v1/realtime/running-tasks?cluster=prod-cluster" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

---

## Next Steps

- **Full API Documentation**: See `realtime.md` for complete API reference
- **Implementation Details**: See `P2-Realtime-API-Implementation-Summary.md`
- **Agent Integration**: See agent documentation for crew integration

---

## Quick Reference

### Endpoints

| Endpoint | Purpose | Cache |
|----------|---------|-------|
| `GET /api/v1/realtime/status` | Cluster status snapshot | 30s |
| `GET /api/v1/realtime/running-tasks` | Running GPU tasks | 30s |

### Key Metrics

| Metric | Description | Range |
|--------|-------------|-------|
| `allocation_rate` | % of GPUs allocated | 0-100% |
| `utilization_rate` | Average GPU utilization | 0-100% |
| `max_contiguous_gpu` | Largest GPU block | 0-N |
| `running_time_seconds` | Task duration | 0-∞ |

---

**Need Help?** Contact Lens API Team

**Last Updated**: 2025-12-19

