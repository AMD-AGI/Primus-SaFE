# Storage API

The Storage API provides operations for retrieving storage statistics and usage information for the cluster.

## Endpoints

### Get Storage Statistics

Retrieves comprehensive storage statistics including capacity, usage, and availability information for the cluster storage systems.

**Endpoint:** `GET /api/storage/stat`

**Query Parameters:** None

**Response:**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "totalCapacity": "1000TB",
    "usedCapacity": "450TB",
    "availableCapacity": "550TB",
    "utilizationRate": 0.45,
    "filesystems": [
      {
        "name": "juicefs-01",
        "type": "JuiceFS",
        "mountPath": "/mnt/juicefs",
        "totalCapacity": "500TB",
        "usedCapacity": "225TB",
        "availableCapacity": "275TB",
        "utilizationRate": 0.45,
        "status": "healthy",
        "inodeTotal": 1000000000,
        "inodeUsed": 450000000,
        "inodeAvailable": 550000000
      },
      {
        "name": "cephfs-01",
        "type": "CephFS",
        "mountPath": "/mnt/cephfs",
        "totalCapacity": "500TB",
        "usedCapacity": "225TB",
        "availableCapacity": "275TB",
        "utilizationRate": 0.45,
        "status": "healthy",
        "inodeTotal": 1000000000,
        "inodeUsed": 450000000,
        "inodeAvailable": 550000000
      }
    ],
    "performance": {
      "readThroughput": "10GB/s",
      "writeThroughput": "8GB/s",
      "readIOPS": 50000,
      "writeIOPS": 40000,
      "avgLatency": "2.5ms"
    }
  },
  "traceId": "trace-abc123"
}
```

**Response Fields:**

### Top Level Fields

| Field | Type | Description |
|-------|------|-------------|
| `totalCapacity` | string | Total storage capacity (human-readable) |
| `usedCapacity` | string | Used storage capacity (human-readable) |
| `availableCapacity` | string | Available storage capacity (human-readable) |
| `utilizationRate` | float | Storage utilization rate (0.0 to 1.0) |
| `filesystems` | array | Array of filesystem details |
| `performance` | object | Storage performance metrics |

### Filesystem Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Filesystem name/identifier |
| `type` | string | Filesystem type (JuiceFS, CephFS, NFS, etc.) |
| `mountPath` | string | Mount path on nodes |
| `totalCapacity` | string | Total capacity (human-readable) |
| `usedCapacity` | string | Used capacity (human-readable) |
| `availableCapacity` | string | Available capacity (human-readable) |
| `utilizationRate` | float | Utilization rate (0.0 to 1.0) |
| `status` | string | Filesystem health status |
| `inodeTotal` | int64 | Total inodes |
| `inodeUsed` | int64 | Used inodes |
| `inodeAvailable` | int64 | Available inodes |

### Performance Fields

| Field | Type | Description |
|-------|------|-------------|
| `readThroughput` | string | Read throughput (human-readable) |
| `writeThroughput` | string | Write throughput (human-readable) |
| `readIOPS` | integer | Read I/O operations per second |
| `writeIOPS` | integer | Write I/O operations per second |
| `avgLatency` | string | Average latency (human-readable) |

**Status Codes:**
- `200 OK` - Success
- `500 Internal Server Error` - Server error

**Example:**

```bash
curl -X GET http://localhost:8080/api/storage/stat
```

---

## Filesystem Status Values

| Status | Description | Recommended Action |
|--------|-------------|-------------------|
| `healthy` | Filesystem is operating normally | None |
| `warning` | Filesystem is experiencing minor issues | Monitor closely |
| `degraded` | Filesystem is degraded but operational | Investigate and resolve issues |
| `critical` | Filesystem has critical issues | Immediate attention required |
| `offline` | Filesystem is offline or unreachable | Restore connectivity |

---

## Data Models

### StorageStat

```go
type StorageStat struct {
    TotalCapacity      string            // Total capacity
    UsedCapacity       string            // Used capacity
    AvailableCapacity  string            // Available capacity
    UtilizationRate    float64           // Utilization rate
    Filesystems        []FilesystemInfo  // Filesystem details
    Performance        PerformanceMetrics // Performance metrics
}
```

### FilesystemInfo

```go
type FilesystemInfo struct {
    Name              string  // Filesystem name
    Type              string  // Filesystem type
    MountPath         string  // Mount path
    TotalCapacity     string  // Total capacity
    UsedCapacity      string  // Used capacity
    AvailableCapacity string  // Available capacity
    UtilizationRate   float64 // Utilization rate
    Status            string  // Health status
    InodeTotal        int64   // Total inodes
    InodeUsed         int64   // Used inodes
    InodeAvailable    int64   // Available inodes
}
```

### PerformanceMetrics

```go
type PerformanceMetrics struct {
    ReadThroughput  string // Read throughput
    WriteThroughput string // Write throughput
    ReadIOPS        int    // Read IOPS
    WriteIOPS       int    // Write IOPS
    AvgLatency      string // Average latency
}
```

---

## Supported Storage Types

The Storage API supports monitoring of various storage types:

### JuiceFS
High-performance distributed file system built on top of object storage and databases.
- **Features**: High throughput, POSIX compatible, cloud-native
- **Common Use Cases**: AI/ML training data, shared datasets

### CephFS
Distributed file system built on top of Ceph object storage.
- **Features**: Highly scalable, self-healing, production-ready
- **Common Use Cases**: General-purpose shared storage

### NFS
Network File System, a distributed file system protocol.
- **Features**: Simple, widely supported, easy to set up
- **Common Use Cases**: Legacy applications, simple file sharing

### Other Storage Types
The API can be extended to support additional storage types such as:
- GlusterFS
- HDFS (Hadoop Distributed File System)
- Lustre
- BeeGFS
- Local storage

---

## Usage Examples

### Basic Usage

```bash
# Get storage statistics
curl -X GET http://localhost:8080/api/storage/stat
```

### Monitoring Storage Utilization

```bash
# Get storage stats and extract utilization rate
curl -s http://localhost:8080/api/storage/stat | jq '.data.utilizationRate'

# Alert if utilization exceeds threshold
UTILIZATION=$(curl -s http://localhost:8080/api/storage/stat | jq '.data.utilizationRate')
if (( $(echo "$UTILIZATION > 0.8" | bc -l) )); then
    echo "WARNING: Storage utilization is above 80%"
fi
```

### Checking Filesystem Health

```bash
# Check status of all filesystems
curl -s http://localhost:8080/api/storage/stat | jq '.data.filesystems[] | {name, status}'

# List filesystems with issues
curl -s http://localhost:8080/api/storage/stat | \
  jq '.data.filesystems[] | select(.status != "healthy") | {name, status, utilizationRate}'
```

### Performance Monitoring

```bash
# Get storage performance metrics
curl -s http://localhost:8080/api/storage/stat | jq '.data.performance'
```

---

## Integration Examples

### Python

```python
import requests

def get_storage_stats():
    response = requests.get('http://localhost:8080/api/storage/stat')
    data = response.json()
    
    if data['code'] == 0:
        storage = data['data']
        print(f"Total Capacity: {storage['totalCapacity']}")
        print(f"Used Capacity: {storage['usedCapacity']}")
        print(f"Utilization: {storage['utilizationRate'] * 100:.2f}%")
        
        for fs in storage['filesystems']:
            print(f"\n{fs['name']} ({fs['type']}):")
            print(f"  Status: {fs['status']}")
            print(f"  Utilization: {fs['utilizationRate'] * 100:.2f}%")
    else:
        print(f"Error: {data['message']}")

get_storage_stats()
```

### Go

```go
package main

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
)

type StorageResponse struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    StorageStat `json:"data"`
}

type StorageStat struct {
    TotalCapacity     string           `json:"totalCapacity"`
    UsedCapacity      string           `json:"usedCapacity"`
    UtilizationRate   float64          `json:"utilizationRate"`
    Filesystems       []FilesystemInfo `json:"filesystems"`
}

type FilesystemInfo struct {
    Name            string  `json:"name"`
    Type            string  `json:"type"`
    Status          string  `json:"status"`
    UtilizationRate float64 `json:"utilizationRate"`
}

func main() {
    resp, err := http.Get("http://localhost:8080/api/storage/stat")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, _ := ioutil.ReadAll(resp.Body)
    
    var result StorageResponse
    json.Unmarshal(body, &result)
    
    fmt.Printf("Storage Utilization: %.2f%%\n", result.Data.UtilizationRate*100)
    
    for _, fs := range result.Data.Filesystems {
        fmt.Printf("%s (%s): %s - %.2f%%\n", 
            fs.Name, fs.Type, fs.Status, fs.UtilizationRate*100)
    }
}
```

---

## Monitoring and Alerting

### Recommended Alert Thresholds

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| Storage Utilization | > 80% | > 90% | Add capacity or clean up data |
| Inode Utilization | > 80% | > 90% | Clean up small files or increase inodes |
| Filesystem Status | warning | degraded/critical | Investigate and resolve issues |
| Write Throughput | < 50% expected | < 25% expected | Check for I/O bottlenecks |
| Average Latency | > 10ms | > 50ms | Investigate storage performance |

### Grafana Dashboard Example

Create a Grafana dashboard using the Infinity data source plugin:

1. **Storage Utilization Gauge**
   - Query: `GET /api/storage/stat`
   - Metric: `$.data.utilizationRate`
   - Visualization: Gauge (0-100%)

2. **Capacity Trend**
   - Query the API periodically
   - Store historical data in Prometheus
   - Visualize capacity growth over time

3. **Filesystem Status Table**
   - Query: `GET /api/storage/stat`
   - Metric: `$.data.filesystems[*]`
   - Visualization: Table with status indicators

---

## Notes

- Capacity values are returned in human-readable format (TB, GB, MB, etc.)
- Utilization rates are returned as floats between 0.0 and 1.0
- Performance metrics represent cluster-wide aggregated values
- Inode statistics are important for workloads with many small files
- Storage statistics are updated periodically (typically every 5 minutes)
- The API aggregates data from multiple storage backends if present

---

## Error Handling

If an error occurs retrieving storage statistics:

```json
{
  "code": 500,
  "message": "Failed to retrieve storage statistics: storage backend unreachable",
  "traceId": "trace-abc123"
}
```

**Common Errors:**
- Storage backend unreachable
- Insufficient permissions to query storage
- Storage metrics not available
- Timeout retrieving storage data

Use the `traceId` for debugging and log correlation.

---

## Future Enhancements

Planned features for future releases:

1. **Historical Storage Data**: Time-series data for capacity and utilization trends
2. **Per-Namespace Storage**: Storage usage breakdown by Kubernetes namespace
3. **Per-User Storage**: Storage usage breakdown by user or workload
4. **Storage Quotas**: Query and manage storage quotas
5. **Performance Trends**: Historical performance metrics and analysis
6. **Storage Predictions**: Capacity forecasting based on usage patterns
7. **Detailed I/O Statistics**: Per-volume I/O metrics
8. **Storage Events**: Alerts and notifications for storage issues

