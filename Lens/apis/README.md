# Lens API - Multi-Cluster Support

## Overview

Lens API supports multi-cluster query functionality, allowing flexible access to data from different clusters through default cluster configuration and query parameters.

## Cluster Selection Priority

When calling the API, the system selects clusters based on the following priority:

1. **Specified Cluster** - Specified via the `cluster` query parameter
2. **Default Cluster** - Configured via the `DEFAULT_CLUSTER_NAME` environment variable
3. **Current Cluster** - The currently running cluster (configured via the `CLUSTER_NAME` environment variable)

## Configuration Methods

### 1. Environment Variable Configuration

```bash
# Current cluster name (required)
export CLUSTER_NAME=local-cluster

# Default cluster name (optional)
# If configured, this cluster will be used when API requests don't specify the cluster parameter
export DEFAULT_CLUSTER_NAME=prod-cluster-1
```

### 2. Kubernetes Deployment Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: lens-api
spec:
  template:
    spec:
      containers:
      - name: lens-api
        image: lens-api:latest
        env:
        - name: CLUSTER_NAME
          value: "local-cluster"
        - name: DEFAULT_CLUSTER_NAME
          value: "prod-cluster-1"  # Configure default cluster
```

### 3. Programmatic Configuration

```go
import "github.com/AMD-AGI/primus-lens/core/pkg/clientsets"

// Get ClusterManager
cm := clientsets.GetClusterManager()

// Set default cluster
cm.SetDefaultClusterName("prod-cluster-1")

// Get currently configured default cluster
defaultCluster := cm.GetDefaultClusterName()
fmt.Printf("Default cluster: %s\n", defaultCluster)
```

## Usage Scenarios

### Scenario 1: No Default Cluster Configured

```bash
# Environment variables
export CLUSTER_NAME=local-cluster
# DEFAULT_CLUSTER_NAME not set
```

**API Behavior:**
- `GET /api/nodes/gpuUtilization` → Uses `local-cluster`
- `GET /api/nodes/gpuUtilization?cluster=prod-cluster-1` → Uses `prod-cluster-1`

### Scenario 2: Default Cluster Configured

```bash
# Environment variables
export CLUSTER_NAME=local-cluster
export DEFAULT_CLUSTER_NAME=prod-cluster-1
```

**API Behavior:**
- `GET /api/nodes/gpuUtilization` → Uses `prod-cluster-1` (default cluster)
- `GET /api/nodes/gpuUtilization?cluster=prod-cluster-2` → Uses `prod-cluster-2` (specified cluster)
- `GET /api/nodes/gpuUtilization?cluster=local-cluster` → Uses `local-cluster` (explicitly specified current cluster)

## Usage Examples

### Python Example

```python
import requests
import os

API_BASE = "http://api-server/api"

# 1. Use default cluster (if DEFAULT_CLUSTER_NAME is configured)
response = requests.get(f"{API_BASE}/nodes/gpuUtilization")
print(f"Default cluster utilization: {response.json()}")

# 2. Specify a particular cluster
response = requests.get(f"{API_BASE}/nodes/gpuUtilization?cluster=prod-cluster-2")
print(f"Prod cluster 2 utilization: {response.json()}")

# 3. Dynamically select cluster
cluster = os.getenv("TARGET_CLUSTER", "")  # If not set, use default cluster
params = {"cluster": cluster} if cluster else {}
response = requests.get(f"{API_BASE}/nodes/gpuUtilization", params=params)
print(f"Selected cluster utilization: {response.json()}")
```

### Go Example

```go
package main

import (
    "fmt"
    "net/http"
    "net/url"
    "os"
)

func getGPUUtilization(cluster string) error {
    baseURL := "http://api-server/api/nodes/gpuUtilization"
    u, _ := url.Parse(baseURL)
    
    // Only add cluster parameter when a cluster is specified
    if cluster != "" {
        q := u.Query()
        q.Set("cluster", cluster)
        u.RawQuery = q.Encode()
    }
    
    resp, err := http.Get(u.String())
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // Process response...
    return nil
}

func main() {
    // Use default cluster
    getGPUUtilization("")
    
    // Use specified cluster
    getGPUUtilization("prod-cluster-1")
    
    // Read from environment variable
    targetCluster := os.Getenv("TARGET_CLUSTER")
    getGPUUtilization(targetCluster)
}
```

### cURL Example

```bash
# Use default cluster (or current cluster if no default cluster is configured)
curl "http://api-server/api/clusters/overview"

# Use specified cluster
curl "http://api-server/api/clusters/overview?cluster=prod-cluster-1"

# Use default cluster to get data from multiple endpoints
curl "http://api-server/api/nodes/gpuUtilization"
curl "http://api-server/api/storage/stat"
curl "http://api-server/api/workloads"
```

## Multi-Cluster Supported API Endpoints

All the following API endpoints support the `cluster` query parameter:

### Node Related
- `GET /api/nodes/gpuAllocation`
- `GET /api/nodes/gpuUtilization`
- `GET /api/nodes/gpuUtilizationHistory`
- `GET /api/nodes/:name/gpuMetrics`

### Cluster Related
- `GET /api/clusters/overview`
- `GET /api/clusters/consumers`
- `GET /api/clusters/gpuHeatmap`

### Workload Related
- `GET /api/workloads/:uid/metrics`

### Storage Related
- `GET /api/storage/stat`

## Best Practices

### 1. Production Environment Configuration

**Control Plane (managing multiple clusters):**
```bash
export CLUSTER_NAME=control-plane
export DEFAULT_CLUSTER_NAME=prod-main-cluster
```

**Data Plane (single cluster):**
```bash
export CLUSTER_NAME=prod-cluster-1
# Do not set DEFAULT_CLUSTER_NAME
```

### 2. Development Environment Configuration

```bash
export CLUSTER_NAME=dev-local
export DEFAULT_CLUSTER_NAME=dev-shared-cluster
```

### 3. Monitoring and Alerting

```python
# Monitoring script example
import requests

CLUSTERS = ["prod-cluster-1", "prod-cluster-2", "prod-cluster-3"]

for cluster in CLUSTERS:
    resp = requests.get(
        "http://api-server/api/nodes/gpuUtilization",
        params={"cluster": cluster}
    )
    data = resp.json()
    utilization = data['data']['utilization']
    
    if utilization > 90:
        print(f"WARNING: {cluster} GPU utilization is {utilization}%")
```

### 4. Failover

```go
// Try primary cluster, fallback to backup cluster on failure
func getClusterData(primaryCluster, backupCluster string) (interface{}, error) {
    data, err := fetchFromCluster(primaryCluster)
    if err != nil {
        log.Warnf("Primary cluster %s failed, trying backup", primaryCluster)
        return fetchFromCluster(backupCluster)
    }
    return data, nil
}
```

## Error Handling

### Default Cluster Unavailable

If the configured default cluster is unavailable, the system will automatically fall back to the current cluster and log a warning:

```
WARN: Failed to get default cluster 'prod-cluster-1', falling back to current cluster: ClientSet for cluster prod-cluster-1 not found
```

### Specified Cluster Does Not Exist

If the cluster specified via query parameter does not exist, the API will return an error:

```json
{
  "code": 1001,
  "message": "ClientSet for cluster invalid-cluster not found",
  "traceId": "trace-abc123"
}
```

## Monitoring and Logging

### Startup Logs

```
INFO: Initialized current cluster: local-cluster (K8S: true, Storage: true)
INFO: Default cluster configured: prod-cluster-1
INFO: Cluster manager initialized successfully
```

### Runtime Logs

```
INFO: Default cluster name set to: prod-cluster-1
WARN: Failed to get default cluster 'prod-cluster-1', falling back to current cluster: connection refused
```

## Troubleshooting

### Issue 1: API Using Wrong Cluster

**Troubleshooting Steps:**
1. Verify environment variable configuration: `echo $DEFAULT_CLUSTER_NAME`
2. Check ClusterManager configuration:
   ```go
   cm := clientsets.GetClusterManager()
   fmt.Printf("Default cluster: %s\n", cm.GetDefaultClusterName())
   fmt.Printf("Available clusters: %v\n", cm.GetClusterNames())
   ```

### Issue 2: Default Cluster Connection Failed

**Solutions:**
- System will automatically fall back to current cluster
- Check default cluster configuration and connection status
- If default cluster is frequently unavailable, consider not configuring a default cluster or switching to a different default cluster

### Issue 3: How to Temporarily Disable Default Cluster

**Method 1: Clear environment variable**
```bash
unset DEFAULT_CLUSTER_NAME
# Restart service
```

**Method 2: Programmatic clear**
```go
cm := clientsets.GetClusterManager()
cm.SetDefaultClusterName("")
```

**Method 3: Explicitly specify current cluster**
```bash
# Assuming current cluster is local-cluster
curl "http://api-server/api/nodes/gpuUtilization?cluster=local-cluster"
```

## Architecture

```
API Request
    ↓
Parse cluster parameter
    ↓
GetClusterClientsOrDefault()
    ↓
    ├─ Has cluster parameter? → Use specified cluster
    ├─ Has default cluster? → Use default cluster (if available, otherwise fall back to current cluster)
    └─ Otherwise → Use current cluster
    ↓
Return ClientSet for corresponding cluster
    ↓
Execute query
```

## Related Files

- `Lens/modules/core/pkg/clientsets/cluster_manager.go` - Cluster management core logic
- `Lens/modules/api/pkg/api/` - API layer implementation

## Changelog

- 2025-11-01: Added default cluster support feature
- 2025-11-01: Added `GetClusterClientsOrDefault()` method
- 2025-11-01: Updated all API endpoints to use unified cluster selection logic
