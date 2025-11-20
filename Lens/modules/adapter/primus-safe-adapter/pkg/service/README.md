# Primus-SaFE Adapter Services

This package contains scheduled services that collect and synchronize data between Lens and SaFE databases.

## Services

### 1. WorkloadStatsService

Collects workload statistics from running workloads and stores them in the SaFE database.

- **Interval**: 30 seconds
- **Function**: Collects GPU metrics and runtime statistics from workloads
- **Target Table**: `workload_statistic` in SaFE database

### 2. NodeStatsService

Collects node GPU utilization statistics from Lens database and synchronizes them to SaFE database. **Supports multi-cluster environments**.

- **Interval**: 60 seconds
- **Function**: Reads GPU utilization from Lens `node` table across all clusters and updates SaFE `node_statistic` table
- **Multi-cluster**: Automatically processes all clusters managed by ClusterManager
- **Target Table**: `node_statistic` in SaFE database

## Architecture

```
┌─────────────────────────────────────────────┐
│           ClusterManager                    │
│  (Manages multiple clusters)                │
└───┬─────────────┬──────────────┬────────────┘
    │             │              │
    v             v              v
┌─────────┐  ┌─────────┐  ┌─────────┐
│Cluster-1│  │Cluster-2│  │Cluster-N│
│  Lens   │  │  Lens   │  │  Lens   │
│  (node) │  │  (node) │  │  (node) │
└────┬────┘  └────┬────┘  └────┬────┘
     │            │             │
     └────────────┴─────────────┘
                  │ Read GPU Utilization (Multi-cluster)
                  v
         ┌─────────────────────┐
         │ NodeStatsService    │
         │  (Scheduler Task)   │
         │  Multi-cluster      │
         └──────────┬──────────┘
                    │ Write Statistics (All clusters)
                    v
         ┌─────────────────────┐
         │  SaFE Database      │
         │  (node_statistic)   │
         │  cluster | node     │
         └─────────────────────┘
```

## Usage

### Basic Usage

The services are automatically initialized in the adapter bootstrap process:

```go
// In bootstrap.go
func initScheduledTasks(ctx context.Context, cfg *config.Config) error {
    // ... database initialization ...

    // Create node stats service
    nodeStatsService := service.NewNodeStatsService(safeDB)

    // Create and configure scheduler
    globalScheduler = scheduler.NewScheduler()

    // Add node stats collection task (runs every 60 seconds)
    globalScheduler.AddTask(nodeStatsService, 60*time.Second)

    // Start scheduler
    go globalScheduler.Start(ctx)

    return nil
}
```

### Standalone Usage

You can also use the service independently:

```go
package main

import (
    "context"
    "time"

    "github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/scheduler"
    "github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/service"
    safeclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func main() {
    ctx := context.Background()

    // Initialize SaFE database client
    safeDBClient := safeclient.NewClient()
    safeDB, _ := safeDBClient.GetGormDB()

    // Create node stats service
    nodeStatsService := service.NewNodeStatsService(safeDB)

    // Create scheduler
    sched := scheduler.NewScheduler()

    // Add task with custom interval (e.g., 5 minutes)
    sched.AddTask(nodeStatsService, 5*time.Minute)

    // Start scheduler
    go sched.Start(ctx)

    // Wait for context cancellation
    <-ctx.Done()
    sched.Stop()
}
```

### Manual Execution

You can also run the service manually without a scheduler:

```go
// Create service
nodeStatsService := service.NewNodeStatsService(safeDB)

// Run once
if err := nodeStatsService.Run(context.Background()); err != nil {
    log.Errorf("Failed to collect node stats: %v", err)
}
```

## Configuration

### Database Connection

The service uses:
- **Lens Database**: Accessed via `database.GetFacade().GetNode()`
- **SaFE Database**: Accessed via GORM DB instance passed to the service constructor

### Interval Configuration

You can adjust the collection interval when adding the task to the scheduler:

```go
// Default: 60 seconds
globalScheduler.AddTask(nodeStatsService, 60*time.Second)

// More frequent: 30 seconds
globalScheduler.AddTask(nodeStatsService, 30*time.Second)

// Less frequent: 5 minutes
globalScheduler.AddTask(nodeStatsService, 5*time.Minute)
```

## Data Flow

1. **Cluster Discovery Phase**:
   - Gets all cluster names via `clientsets.GetClusterManager().GetClusterNames()`
   - Iterates through each cluster

2. **Read Phase** (per cluster):
   - Service calls `database.GetFacadeForCluster(clusterName).GetNode().ListGpuNodes(ctx)`
   - Retrieves all GPU nodes from that cluster's Lens database
   - Extracts `Name` and `GpuUtilization` fields

3. **Transform Phase** (per node):
   - Prepares node statistic records with:
     - `Cluster`: Cluster name
     - `NodeName`: Node name from Lens
     - `GpuUtilization`: GPU utilization percentage

4. **Write Phase** (per node):
   - For each node, checks if record exists in `node_statistic` table
   - If exists: Updates `GpuUtilization` and `UpdatedAt`
   - If not exists: Creates new record

5. **Summary Phase**:
   - Logs total statistics across all clusters
   - Reports success/failure counts per cluster and overall

## Database Schema

### Source: Lens `node` table
```go
type Node struct {
    ID             int32
    Name           string
    GpuUtilization float64
    // ... other fields
}
```

### Target: SaFE `node_statistic` table
```go
type NodeStatistic struct {
    ID             int32
    Cluster        string         // Cluster identifier
    NodeName       string         // Node name
    GpuUtilization float64        // GPU utilization (0-100)
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      gorm.DeletedAt
}
```

## Logging

The service provides detailed logging for multi-cluster operations:

- **Info**: Task start/end, cluster discovery, per-cluster and overall statistics
- **Debug**: Individual node updates
- **Error**: Database errors, cluster processing failures

Example logs (multi-cluster):
```
[INFO] Starting node stats collection for all clusters
[INFO] Found 3 cluster(s) to process
[INFO] Processing cluster: cluster-1
[INFO] Found 10 GPU nodes in cluster: cluster-1
[DEBUG] Updated node statistic for node node-1: gpu_utilization=75.50
[DEBUG] Created node statistic for node node-2: gpu_utilization=82.30
[INFO] Cluster cluster-1 processed: success=10, failed=0
[INFO] Processing cluster: cluster-2
[INFO] Found 5 GPU nodes in cluster: cluster-2
[INFO] Cluster cluster-2 processed: success=5, failed=0
[INFO] Processing cluster: cluster-3
[INFO] No GPU nodes found in cluster: cluster-3
[INFO] Cluster cluster-3 processed: success=0, failed=0
[INFO] Node stats collection completed: clusters=3/3, total_nodes_success=15, total_nodes_failed=0, duration=456ms
```

## Error Handling

- If cluster name cannot be retrieved, the task exits gracefully
- Database errors are logged but don't stop the scheduler
- Individual node failures don't affect other nodes
- Task will retry on next scheduled interval

## Testing

Run the tests:

```bash
cd Lens/modules/adapter/primus-safe-adapter/pkg/service
go test -v
```

## Implementation Details

### Task Interface

The service implements the `scheduler.Task` interface:

```go
type Task interface {
    Name() string
    Run(ctx context.Context) error
}
```

### Thread Safety

- The service is designed to be called from a scheduler goroutine
- Database operations are context-aware and can be cancelled
- No shared state between runs (stateless)

### Performance Considerations

- Batch reads from Lens database
- Individual writes to SaFE database (upsert per node)
- Typical execution time: 100-500ms for 10-100 nodes
- Database connection pooling handled by GORM

## Troubleshooting

### No data in node_statistic table

1. Check if Lens database has nodes in any cluster:
   ```sql
   SELECT name, gpu_utilization FROM node;
   ```

2. Check scheduler logs:
   ```bash
   kubectl logs -n primus-safe <adapter-pod> | grep node-stats
   ```

3. Verify clusters are discovered:
   ```bash
   kubectl logs <pod> | grep "Found .* cluster(s) to process"
   ```

4. Check which clusters are being processed:
   ```bash
   kubectl logs <pod> | grep "Processing cluster:"
   ```

### Some clusters not being processed

1. Check ClusterManager initialization:
   ```bash
   kubectl logs <pod> | grep "Cluster manager initialized"
   ```

2. Verify cluster registration:
   ```bash
   kubectl logs <pod> | grep "Loaded cluster:"
   ```

3. Check for cluster-specific errors:
   ```bash
   kubectl logs <pod> | grep "Failed to process cluster"
   ```

### Stale data

- Check the `UpdatedAt` timestamp in `node_statistic` table by cluster:
  ```sql
  SELECT cluster, node_name, updated_at 
  FROM node_statistic 
  ORDER BY cluster, updated_at DESC;
  ```
- Verify scheduler is running: Look for "Scheduler started" log
- Confirm task interval is appropriate

### High CPU/Memory usage (Multi-cluster)

- Increase task interval to reduce frequency
- Check for database connection leaks in each cluster
- Review total number of nodes across all clusters
- Consider processing clusters sequentially vs. in parallel
- Monitor per-cluster processing time in logs

## Future Enhancements

Potential improvements:

- [x] Multi-cluster support (IMPLEMENTED)
- [ ] Parallel cluster processing for improved performance
- [ ] Batch upsert for better performance
- [ ] Configurable retention policy for old statistics
- [ ] Metrics export (Prometheus) - per cluster and aggregated
- [ ] Historical data aggregation
- [ ] Alert on significant utilization changes
- [ ] Cluster health monitoring and skip unhealthy clusters
- [ ] Configurable cluster filtering (process specific clusters only)

