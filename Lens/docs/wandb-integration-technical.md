# WandB Integration Technical Documentation

## Overview

This document provides detailed technical implementation of WandB (Weights & Biases) data reporting and metrics display in the Primus Lens system, covering the complete data flow from `wandb-exporter` to `telemetry-processor` to `api`.

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Training Container (Pod)                       │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  User Training Code                                          │   │
│  │    import wandb                                              │   │
│  │    wandb.init(project="my-project")                         │   │
│  │    wandb.log({"loss": 0.5, "accuracy": 0.95})              │   │
│  └────────────────────┬─────────────────────────────────────────┘   │
│                       │                                              │
│  ┌────────────────────▼─────────────────────────────────────────┐   │
│  │  primus-lens-wandb-exporter                                  │   │
│  │  - wandb_hook.py: Auto-intercept wandb.init() and wandb.log()│  │
│  │  - data_collector.py: Collect framework detection data      │   │
│  │  - api_reporter.py: Async report to telemetry-processor     │   │
│  └────────────────────┬─────────────────────────────────────────┘   │
└─────────────────────┬─│─────────────────────────────────────────────┘
                      │ │
          Framework   │ │ Metrics
          Detection   │ │ Data
                      │ │
          ┌───────────▼─▼─────────────┐
          │  telemetry-processor      │
          │  (Go Service)             │
          │                           │
          │  ┌─────────────────────┐  │
          │  │  WandB API Handler  │  │
          │  │  - Detection        │  │
          │  │  - Metrics          │  │
          │  │  - Logs/Training    │  │
          │  └──────────┬──────────┘  │
          │             │              │
          │  ┌──────────▼──────────┐  │
          │  │ wandb_detector.go   │  │
          │  │ (Framework Detection)│  │
          │  └──────────┬──────────┘  │
          │             │              │
          │  ┌──────────▼──────────┐  │
          │  │wandb_log_processor.go│ │
          │  │  (Metrics Storage)   │  │
          │  └──────────┬──────────┘  │
          └─────────────┼──────────────┘
                        │
                        │ Store to DB
                        │
          ┌─────────────▼──────────────┐
          │    PostgreSQL Database     │
          │                            │
          │  - training_performance    │
          │  - framework_detection     │
          │  - metrics_storage         │
          └─────────────┬──────────────┘
                        │
                        │ Query Data
                        │
          ┌─────────────▼──────────────┐
          │      Lens API Module       │
          │      (Go Service)          │
          │                            │
          │  ┌──────────────────────┐  │
          │  │ training_performance │  │
          │  │      _test.go        │  │
          │  │                      │  │
          │  │  API Endpoints:      │  │
          │  │  - GetDataSources    │  │
          │  │  - GetAvailableMetrics│ │
          │  │  - GetMetricsData    │  │
          │  │  - GetIterationTimes │  │
          │  └──────────┬───────────┘  │
          └─────────────┼──────────────┘
                        │
                        │ HTTP API
                        │
          ┌─────────────▼──────────────┐
          │   Frontend/Grafana/Clients │
          └────────────────────────────┘
```

## Part 1: wandb-exporter Data Collection Layer

### 1.1 Auto-Interception Mechanism

`wandb-exporter` implements zero-code-intrusion auto-interception through Python's import hook mechanism:

#### 1.1.1 Installation and Activation

```python
# setup.py automatically creates .pth file during installation
# Creates primus_lens_wandb_hook.pth in site-packages directory
# Content: import primus_lens_wandb_exporter.wandb_hook

# Python automatically loads wandb_hook.py on startup
# Registers WandbImportHook to sys.meta_path
```

#### 1.1.2 Intercepting wandb.init()

**Location**: `wandb_hook.py` lines 217-270

```python
def intercepted_init(*args, **kwargs):
    """Intercept wandb.init"""
    # 1. Get distributed training rank info
    rank_info = self._get_rank_info()  # RANK, LOCAL_RANK, NODE_RANK, WORLD_SIZE
    
    # 2. Setup metrics output path (optional local file saving)
    output_path = self._setup_metrics_output()
    
    # 3. Call original wandb.init()
    result = self.original_init(*args, **kwargs)
    
    # 4. Save run object and run_id
    self.wandb_run = result
    self.run_id = result.id
    
    # 5. Async report framework detection data
    if self.api_reporting_enabled:
        self._report_framework_detection(result)
    
    # 6. Re-intercept wandb.log (since wandb.init overwrites it)
    wandb.log = intercepted_log
    
    return result
```

**Key Environment Variables**:
- `WORKLOAD_UID`: Workload unique identifier
- `POD_UID`: Pod unique identifier
- `POD_NAME`: Pod name (required)
- `RANK`, `LOCAL_RANK`, `NODE_RANK`, `WORLD_SIZE`: Distributed training info

#### 1.1.3 Intercepting wandb.log()

**Location**: `wandb_hook.py` lines 272-338

```python
def intercepted_log(data: Dict[str, Any], step: Optional[int] = None, *args, **kwargs):
    """Intercept wandb.log"""
    # 1. Copy data, add Primus Lens marker
    enhanced_data = data.copy()
    enhanced_data["_primus_lens_enabled"] = True
    
    # 2. Optional: Add system metrics (CPU, memory, GPU)
    if enhance_metrics:
        enhanced_data["_primus_sys_cpu_percent"] = psutil.cpu_percent()
        enhanced_data["_primus_sys_memory_percent"] = psutil.virtual_memory().percent
        # GPU metrics...
    
    # 3. Save to local file (optional)
    if save_local:
        self._save_metrics(enhanced_data, step)
    
    # 4. Async report metrics to API
    if self.api_reporting_enabled:
        self._report_metrics(data, step)
    
    # 5. Call original wandb.log()
    return self.original_log(enhanced_data, step=step, *args, **kwargs)
```

### 1.2 Framework Detection and Data Collection

**Location**: `data_collector.py`

#### 1.2.1 Framework Detection Hierarchy

The system supports **dual-layer framework detection**:

- **Wrapper Frameworks**: Primus, PyTorch Lightning, Transformers Trainer
- **Base Frameworks**: Megatron, DeepSpeed, JAX, Transformers

#### 1.2.2 Data Collection Flow

```python
def collect_detection_data(self, wandb_run) -> Dict[str, Any]:
    """Collect framework detection data"""
    # 1. Collect raw evidence
    evidence = self._collect_raw_evidence(wandb_run)
    #    - WandB info (project, name, id, config, tags)
    #    - Environment variables (framework-related env vars)
    #    - PyTorch info (version, CUDA, imported modules)
    #    - Wrapper framework detection (via import)
    #    - Base framework detection (via import)
    #    - System info (Python version, platform)
    
    # 2. Generate detection hints
    hints = self._get_framework_hints(evidence)
    #    - wrapper_frameworks: []
    #    - base_frameworks: []
    #    - confidence: "low" / "medium" / "high"
    #    - primary_indicators: []  # Detection basis
    
    # 3. Construct complete report data
    detection_data = {
        "source": "wandb",
        "type": "framework_detection_raw",
        "version": "1.0",
        "workload_uid": os.environ.get("WORKLOAD_UID", ""),
        "pod_uid": os.environ.get("POD_UID", ""),
        "pod_name": os.environ.get("POD_NAME", ""),
        "evidence": evidence,
        "hints": hints,
        "timestamp": time.time(),
    }
    
    return detection_data
```

#### 1.2.3 Framework Detection Method Priority

1. **Import Detection** (confidence 0.90) - Strongest indicator
   - Detects actually loaded Python modules
   - `_detect_wrapper_by_import()`: Detects primus, lightning, transformers
   - `_detect_base_by_import()`: Detects megatron, deepspeed, jax

2. **Environment Variable Detection** (confidence 0.80)
   - `PRIMUS_CONFIG`, `PRIMUS_BACKEND`
   - `DEEPSPEED_CONFIG`, `DS_CONFIG`
   - `MEGATRON_CONFIG`, `MEGATRON_LM_PATH`
   - `JAX_BACKEND`, `JAX_PLATFORMS`

3. **WandB Config Detection** (confidence 0.70)
   - `config.framework`
   - `config.base_framework`
   - `config.trainer`

4. **PyTorch Module Detection** (confidence 0.60)
   - Framework modules in `sys.modules`

5. **Project Name Detection** (confidence 0.50)
   - Framework keywords in WandB project name

### 1.3 Async API Reporting

**Location**: `api_reporter.py`

#### 1.3.1 Architecture Design

```python
class AsyncAPIReporter:
    """Async API Reporter"""
    def __init__(self):
        # Data queues
        self.detection_queue = Queue(maxsize=100)
        self.metrics_queue = Queue(maxsize=1000)
        self.logs_queue = Queue(maxsize=1000)
        
        # Background worker thread
        self.worker_thread = threading.Thread(target=self._worker_loop, daemon=True)
        
        # Configuration
        self.api_base_url = os.environ.get(
            "PRIMUS_LENS_API_BASE_URL",
            "http://primus-lens-telemetry-processor:8080/api/v1"
        )
        self.batch_size = 10
        self.flush_interval = 5.0  # seconds
```

#### 1.3.2 Reporting Endpoints

1. **Framework Detection Reporting**
   ```
   POST {api_base_url}/wandb/detection
   Content-Type: application/json
   
   {
     "source": "wandb",
     "type": "framework_detection_raw",
     "workload_uid": "...",
     "pod_name": "...",
     "evidence": { ... },
     "hints": { ... }
   }
   ```

2. **Metrics Reporting**
   ```
   POST {api_base_url}/wandb/metrics
   Content-Type: application/json
   
   {
     "source": "wandb",
     "workload_uid": "...",
     "pod_name": "...",
     "run_id": "...",
     "metrics": [
       {
         "name": "loss",
         "value": 0.5,
         "step": 100,
         "timestamp": 1234567890.123,
         "tags": {}
       }
     ]
   }
   ```

#### 1.3.3 Batch Processing

- **Queue Mechanism**: Uses Python `Queue` for lock-free thread-safe queuing
- **Batch Sending**: Triggers batch send when queue reaches `batch_size` or exceeds `flush_interval`
- **Non-blocking**: If queue is full, drops data directly to avoid blocking training

## Part 2: telemetry-processor Processing Layer

### 2.1 API Handler

**Location**: `pkg/module/logs/wandb_api.go`

#### 2.1.1 Endpoint Definition

| Endpoint | Method | Function | Handler |
|------|------|------|---------|
| `/api/v1/wandb/detection` | POST | Framework detection reporting | `ReceiveWandBDetection` |
| `/api/v1/wandb/metrics` | POST | Metrics reporting | `ReceiveWandBMetrics` |
| `/api/v1/wandb/logs` | POST | Training data reporting | `ReceiveWandBLogs` |
| `/api/v1/wandb/batch` | POST | Batch reporting | `ReceiveWandBBatch` |

#### 2.1.2 WorkloadUID Resolution

```go
// getWorkloadUIDsFromPodName resolves WorkloadUID from PodName
func getWorkloadUIDsFromPodName(workloadUID string, podName string, apiName string) []string {
    // 1. If WorkloadUID is already provided, return directly
    if workloadUID != "" {
        return []string{workloadUID}
    }
    
    // 2. Query associated workloads from pod_cache by PodName
    if podName != "" {
        workloads := pods.GetWorkloadsByPodName(podName)
        // A Pod may belong to multiple Workloads
        // Return all associated WorkloadUIDs
    }
}
```

### 2.2 Framework Detection Processing

**Location**: `pkg/module/logs/wandb_detector.go`

#### 2.2.1 Detection Flow

```go
func (d *WandBFrameworkDetector) ProcessWandBDetection(
    ctx context.Context,
    req *WandBDetectionRequest,
) error {
    // 1. Resolve WorkloadUID
    workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
    
    // 2. Execute framework detection rules
    result := d.detectFramework(req)
    // Returns:
    // - Framework: Main framework
    // - FrameworkLayer: "wrapper" or "base"
    // - WrapperFramework: Wrapper framework (if any)
    // - BaseFramework: Base framework (if any)
    // - Confidence: Confidence level
    // - Method: Detection method
    
    // 3. Construct evidence
    evidence := map[string]interface{}{
        "method": result.Method,
        "framework_layer": result.FrameworkLayer,
        "wrapper_framework": result.WrapperFramework,
        "base_framework": result.BaseFramework,
        // ...more evidence
    }
    
    // 4. Report to FrameworkDetectionManager
    err = d.detectionManager.ReportDetection(
        ctx,
        workloadUID,
        "wandb",           // source
        result.Framework,  // framework
        "training",        // workload_type
        result.Confidence,
        evidence,
    )
}
```

#### 2.2.2 Detection Rule Priority

```go
func (d *WandBFrameworkDetector) detectFramework(req *WandBDetectionRequest) *DetectionResult {
    // 1. Import detection (confidence 0.90)
    if result := d.detectFromImportEvidence(req.Evidence); result != nil {
        return result
    }
    
    // 2. Environment variable detection (confidence 0.80)
    if result := d.detectFromEnvVars(req.Evidence.Environment); result != nil {
        return result
    }
    
    // 3. WandB Config detection (confidence 0.70)
    if result := d.detectFromWandBConfig(req.Evidence.WandB.Config); result != nil {
        return result
    }
    
    // 4. PyTorch module detection (confidence 0.60)
    if result := d.detectFromPyTorchModules(req.Evidence.PyTorch); result != nil {
        return result
    }
    
    // 5. WandB project name detection (confidence 0.50)
    if result := d.detectFromWandBProject(req.Evidence.WandB.Project); result != nil {
        return result
    }
    
    return nil
}
```

### 2.3 Metrics Data Processing

**Location**: `pkg/module/logs/wandb_log_processor.go`

#### 2.3.1 ProcessMetrics - Metrics Processing

```go
func (p *WandBLogProcessor) ProcessMetrics(
    ctx context.Context,
    req *WandBMetricsRequest,
) error {
    // 1. Resolve WorkloadUID
    workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
    
    // 2. Store to MetricsStorage (time-series database)
    for _, metric := range req.Metrics {
        storedMetric := &StoredMetric{
            WorkloadUID: workloadUID,
            Source:      constant.DataSourceWandB,
            RunID:       req.RunID,
            Name:        metric.Name,
            Value:       metric.Value,
            Step:        metric.Step,
            Timestamp:   time.Unix(0, int64(metric.Timestamp*1e9)),
        }
        p.metricsStorage.Store(ctx, storedMetric)
    }
    
    // 3. Aggregate metrics by step, store to training_performance table
    stepMetrics := make(map[int64]map[string]interface{})
    for _, metric := range req.Metrics {
        step := metric.Step
        if stepMetrics[step] == nil {
            stepMetrics[step] = make(map[string]interface{})
        }
        stepMetrics[step][metric.Name] = metric.Value
    }
    
    // 4. Store by step
    for step, data := range stepMetrics {
        p.storeTrainingData(ctx, workloadUID, req.PodUID, req.RunID, &WandBLog{
            Step: step,
            Data: data,
        }, timestamp)
    }
}
```

#### 2.3.2 storeTrainingData - Data Persistence

```go
func (p *WandBLogProcessor) storeTrainingData(
    ctx context.Context,
    workloadUID, podUID, runID string,
    data *WandBLog,
    timestamp time.Time,
) error {
    // 1. Prepare performance data
    newPerformanceData := map[string]interface{}{
        "source": constant.DataSourceWandB,
        "run_id": runID,
        "step":   data.Step,
    }
    
    // Merge all metrics
    for key, value := range data.Data {
        newPerformanceData[key] = value
    }
    
    // 2. Check if record already exists (same workload_uid + serial + iteration)
    existingPerf, err := database.GetFacade().GetTraining().
        GetTrainingPerformanceByWorkloadIdSerialAndIteration(
            ctx, workloadUID, serial, iteration)
    
    // 3. If exists, merge historical data
    if existingPerf != nil {
        // Put old data into history array
        historyEntry := existingPerf.Performance
        historyEntry["updated_at"] = existingPerf.CreatedAt.Format(time.RFC3339)
        
        // Get existing history
        var history []interface{}
        if existingData["history"] != nil {
            history = existingData["history"].([]interface{})
        }
        history = append(history, historyEntry)
        
        // Merge new data
        finalPerformanceData = merge(existingData, newPerformanceData)
        finalPerformanceData["history"] = history
    }
    
    // 4. Save/update record
    perfRecord := &dbModel.TrainingPerformance{
        ID:          recordID,
        PodUUID:     podUID,
        Performance: encoded,
        Iteration:   int32(iteration),
        Serial:      int32(serial),
        WorkloadUID: workloadUID,
        DataSource:  constant.DataSourceWandB,
    }
    
    if recordID > 0 {
        trainingFacade.UpdateTrainingPerformance(ctx, perfRecord)
    } else {
        trainingFacade.CreateTrainingPerformance(ctx, perfRecord)
    }
}
```

#### 2.3.3 Database Model

**training_performance table structure**:

```sql
CREATE TABLE training_performance (
    id SERIAL PRIMARY KEY,
    workload_uid VARCHAR(255) NOT NULL,
    pod_uuid VARCHAR(255),
    serial INTEGER DEFAULT 1,
    iteration INTEGER NOT NULL,
    data_source VARCHAR(50) NOT NULL,  -- 'wandb', 'log', 'tensorflow'
    performance JSONB NOT NULL,        -- Stores all metrics as JSON
    created_at TIMESTAMP NOT NULL,
    
    INDEX idx_workload_uid (workload_uid),
    INDEX idx_workload_uid_data_source (workload_uid, data_source),
    INDEX idx_workload_uid_serial_iteration (workload_uid, serial, iteration)
);
```

**performance JSONB field structure** (wandb data source):

```json
{
  "source": "wandb",
  "run_id": "abc123",
  "step": 100,
  "loss": 0.5,
  "accuracy": 0.95,
  "learning_rate": 0.001,
  "history": [
    {
      "source": "wandb",
      "run_id": "abc123",
      "step": 100,
      "loss": 0.6,
      "accuracy": 0.93,
      "updated_at": "2024-01-01T10:00:00Z"
    }
  ],
  "created_at": "2024-01-01T10:00:00Z",
  "updated_at": "2024-01-01T10:05:00Z"
}
```

### 2.4 Monitoring Metrics

**Location**: `pkg/module/logs/metrics.go`

The system provides rich Prometheus monitoring metrics:

```go
// Request count
IncWandBRequestCount("detection" | "metrics" | "logs")

// Request duration
ObserveWandBRequestDuration(requestType, duration)

// Error count
IncWandBRequestErrorCount(requestType, errorType)

// Data point count
ObserveWandBMetricsDataPointCount(workloadUID, count)
ObserveWandBLogsDataPointCount(workloadUID, count)

// Storage count
IncWandBMetricsStoreCount(workloadUID)
IncWandBMetricsStoreErrors(workloadUID)

// Training performance storage
IncTrainingPerformanceSaveCount(workloadUID, dataSource)
IncTrainingPerformanceSaveErrors(workloadUID, dataSource, errorReason)

// Framework detection
IncFrameworkDetectionCount(framework, method, source)
ObserveFrameworkDetectionConfidence(framework, method, confidence)
IncFrameworkDetectionErrors(source, errorReason)
```

## Part 3: API Metrics Query Layer

**Location**: `Lens/modules/api/pkg/api/training_performance.go`

### 3.1 API Endpoint Design

#### 3.1.1 Get Data Sources List

```
GET /api/v1/workloads/:uid/metrics/sources?cluster=<cluster_name>
```

**Response**:
```json
{
  "workload_uid": "abc-123",
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

#### 3.1.2 Get Available Metrics List

```
GET /api/v1/workloads/:uid/metrics/available?data_source=wandb&cluster=<cluster_name>
```

**Response**:
```json
{
  "workload_uid": "abc-123",
  "metrics": [
    {
      "name": "loss",
      "data_source": ["wandb"],
      "count": 1500
    },
    {
      "name": "accuracy",
      "data_source": ["wandb", "log"],
      "count": 2300
    }
  ],
  "total_count": 2
}
```

#### 3.1.3 Get Metrics Data

```
GET /api/v1/workloads/:uid/metrics/data?
    data_source=wandb&
    metrics=loss,accuracy&
    start=1704067200000&
    end=1704153600000&
    cluster=<cluster_name>
```

**Response**:
```json
{
  "workload_uid": "abc-123",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "loss",
      "value": 0.5,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "accuracy",
      "value": 0.95,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    }
  ],
  "total_count": 2
}
```

**Query Parameters**:
- `data_source`: Data source filter (optional)
- `metrics`: Metrics list, comma-separated (optional, supports "all" or omit to return all)
- `start`: Start timestamp (milliseconds) (optional)
- `end`: End timestamp (milliseconds) (optional)
- `cluster`: Cluster name (optional)

#### 3.1.4 Get Iteration Time Information

```
GET /api/v1/workloads/:uid/metrics/iteration-times?
    data_source=wandb&
    start=1704067200000&
    end=1704153600000&
    cluster=<cluster_name>
```

**Response**:
```json
{
  "workload_uid": "abc-123",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "iteration",
      "value": 100,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "target_iteration",
      "value": 10000,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    }
  ],
  "total_count": 2
}
```

### 3.2 Core Implementation

#### 3.2.1 Metrics Field Filtering

```go
// Metadata fields for wandb data source (not actual metrics)
var wandbMetadataFields = map[string]bool{
    "step":       true,
    "run_id":     true,
    "source":     true,
    "history":    true,
    "created_at": true,
    "updated_at": true,
}

// Determine if field is an actual metric
func isMetricField(fieldName string, dataSource string) bool {
    switch dataSource {
    case "wandb":
        return !wandbMetadataFields[fieldName]
    case "log", "tensorflow":
        return true
    default:
        return true
    }
}
```

#### 3.2.2 Data Query Logic

```go
func GetMetricsData(ctx *gin.Context) {
    // 1. Parse parameters
    workloadUID := ctx.Param("uid")
    dataSource := ctx.Query("data_source")
    metricsStr := ctx.Query("metrics")
    
    // 2. Parse metrics list
    var requestedMetrics []string
    var returnAllMetrics bool = true
    
    if metricsStr != "" && metricsStr != "all" {
        // Support Grafana format: {metric1,metric2}
        requestedMetrics = strings.Split(metricsStr, ",")
        returnAllMetrics = false
    }
    
    // 3. Query database
    performances, err := database.GetFacade().GetTraining().
        ListTrainingPerformanceByWorkloadUIDAndDataSource(
            ctx, workloadUID, dataSource,
        )
    
    // 4. Build data points
    for _, p := range performances {
        for metricName, value := range p.Performance {
            // Filter metadata fields
            if !isMetricField(metricName, p.DataSource) {
                continue
            }
            
            // Filter non-requested metrics
            if !returnAllMetrics && !metricsSet[metricName] {
                continue
            }
            
            dataPoints = append(dataPoints, MetricDataPoint{
                MetricName: metricName,
                Value:      convertToFloat(value),
                Timestamp:  p.CreatedAt.UnixMilli(),
                Iteration:  p.Iteration,
                DataSource: p.DataSource,
            })
        }
    }
}
```

### 3.3 Grafana Integration

#### 3.3.1 Configure SimpleJson Data Source

```json
{
  "name": "Primus Lens Metrics",
  "type": "simplejson",
  "url": "http://primus-lens-api:8080/api/v1",
  "access": "proxy",
  "jsonData": {
    "queryEndpoint": "/workloads/{workload_uid}/metrics/data"
  }
}
```

#### 3.3.2 Query Example

```json
{
  "targets": [
    {
      "target": "loss",
      "refId": "A",
      "type": "timeseries"
    },
    {
      "target": "accuracy",
      "refId": "B",
      "type": "timeseries"
    }
  ],
  "range": {
    "from": "2024-01-01T00:00:00Z",
    "to": "2024-01-02T00:00:00Z"
  },
  "variables": {
    "workload_uid": "abc-123",
    "data_source": "wandb"
  }
}
```

## Part 4: Configuration and Deployment

### 4.1 wandb-exporter Configuration

#### 4.1.1 Environment Variables

```bash
# Hook toggle
PRIMUS_LENS_WANDB_HOOK=true  # Enable Hook interception (default true)

# API reporting configuration
PRIMUS_LENS_WANDB_API_REPORTING=true  # Enable API reporting (default true)
PRIMUS_LENS_API_BASE_URL=http://primus-lens-telemetry-processor:8080/api/v1

# Local saving configuration
PRIMUS_LENS_WANDB_SAVE_LOCAL=true  # Enable local saving (default true)
PRIMUS_LENS_WANDB_OUTPUT_PATH=/mnt/output  # Local save path

# System metrics enhancement
PRIMUS_LENS_WANDB_ENHANCE_METRICS=false  # Add system metrics (default false)

# Workload identification (required)
WORKLOAD_UID=abc-123  # Workload unique identifier
POD_UID=pod-456       # Pod unique identifier
POD_NAME=training-pod-0  # Pod name (required)

# Distributed training info
RANK=0
LOCAL_RANK=0
NODE_RANK=0
WORLD_SIZE=8
```

#### 4.1.2 Installation Methods

**Method 1: Using installation script (recommended)**
```bash
# Download and execute installation script
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh | bash

# Or download then execute
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh -o install.sh
chmod +x install.sh
./install.sh
```

**Method 2: Manual pip installation**
```bash
# If you have a local wheel package or published to PyPI
pip install primus-lens-wandb-exporter
# After installation, .pth file is automatically created, Hook is auto-enabled
```

**Method 3: Manual import**
```python
# At the very beginning of training script
import primus_lens_wandb_exporter.wandb_hook
primus_lens_wandb_exporter.wandb_hook.install_wandb_hook()

# Then use wandb normally
import wandb
wandb.init(...)
wandb.log(...)
```

### 4.2 telemetry-processor Configuration

#### 4.2.1 Configuration File

```yaml
# config.yaml
server:
  port: 8080
  
database:
  host: postgres
  port: 5432
  database: primus_lens
  user: lens
  password: ${DB_PASSWORD}
  
wandb:
  enabled: true
  detection_enabled: true
  metrics_enabled: true
  
metrics:
  prometheus:
    enabled: true
    port: 9090
```

#### 4.2.2 Route Registration

```go
// router.go
func RegisterWandBRoutes(r *gin.Engine) {
    v1 := r.Group("/api/v1")
    {
        wandb := v1.Group("/wandb")
        {
            wandb.POST("/detection", logs.ReceiveWandBDetection)
            wandb.POST("/metrics", logs.ReceiveWandBMetrics)
            wandb.POST("/logs", logs.ReceiveWandBLogs)
            wandb.POST("/batch", logs.ReceiveWandBBatch)
        }
    }
}
```

### 4.3 API Module Configuration

#### 4.3.1 Route Registration

```go
// router.go
func RegisterMetricsRoutes(r *gin.Engine) {
    v1 := r.Group("/api/v1")
    {
        workloads := v1.Group("/workloads")
        {
            workloads.GET("/:uid/metrics/sources", api.GetDataSources)
            workloads.GET("/:uid/metrics/available", api.GetAvailableMetrics)
            workloads.GET("/:uid/metrics/data", api.GetMetricsData)
            workloads.GET("/:uid/metrics/iteration-times", api.GetIterationTimes)
        }
    }
}
```

## Part 5: Monitoring and Troubleshooting

### 5.1 Log Levels

#### 5.1.1 wandb-exporter Logs

```python
# logger.py
import os

DEBUG_ENABLED = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false").lower() == "true"

def debug_log(message):
    if DEBUG_ENABLED:
        print(f"[DEBUG] {message}", file=sys.stderr)
```

**Enable debug logs**:
```bash
export PRIMUS_LENS_WANDB_DEBUG=true
```

#### 5.1.2 telemetry-processor Logs

```bash
# Set log level
export LOG_LEVEL=debug  # debug, info, warn, error
```

**Key logs**:
- `[WandB Detection API] Received request` - Received detection request
- `[WandB Metrics API] Processing metrics for %d workload(s)` - Processing metrics
- `✓ WandB metrics stored to MetricsStorage: %d success` - Metrics stored successfully
- `✓ Updated WandB training data for workload %s` - Training data updated

### 5.2 Common Issues

#### 5.2.1 Data Not Reported

**Symptoms**: wandb-exporter running but data not reaching telemetry-processor

**Troubleshooting Steps**:

1. **Check if Hook is enabled**
   ```python
   import wandb
   print(hasattr(wandb, '_primus_lens_patched'))  # Should be True
   ```

2. **Check environment variables**
   ```bash
   echo $PRIMUS_LENS_API_BASE_URL
   echo $POD_NAME  # Must be set
   ```

3. **Check network connectivity**
   ```bash
   curl -X POST http://primus-lens-telemetry-processor:8080/api/v1/wandb/metrics \
     -H "Content-Type: application/json" \
     -d '{"source":"wandb","pod_name":"test","metrics":[]}'
   ```

4. **View wandb-exporter logs**
   ```bash
   export PRIMUS_LENS_WANDB_DEBUG=true
   python train.py
   # Check stderr output
   ```

#### 5.2.2 WorkloadUID Resolution Failed

**Symptoms**: `no valid workload found` error

**Cause**: PodName cannot be resolved to WorkloadUID

**Solutions**:
1. **Directly provide WorkloadUID**
   ```bash
   export WORKLOAD_UID=abc-123
   ```

2. **Ensure Pod is in pod_cache**
   ```bash
   # Check pod_cache table
   SELECT * FROM pod_cache WHERE pod_name = 'training-pod-0';
   ```

#### 5.2.3 Missing Metrics

**Symptoms**: Some metrics not displayed

**Troubleshooting**:

1. **Check field filtering**
   ```go
   // Confirm metric name is not in metadata fields list
   var wandbMetadataFields = map[string]bool{
       "step": true,
       "run_id": true,
       "source": true,
       // ...
   }
   ```

2. **Check data type**
   ```python
   # wandb.log() only supports numeric types
   wandb.log({
       "loss": 0.5,          # ✓ Supported
       "name": "training",   # ✗ Not supported, will be filtered
   })
   ```

3. **View database**
   ```sql
   SELECT iteration, performance
   FROM training_performance
   WHERE workload_uid = 'abc-123' AND data_source = 'wandb'
   ORDER BY iteration DESC
   LIMIT 1;
   ```

### 5.3 Performance Monitoring

#### 5.3.1 Prometheus Metrics Queries

```promql
# Reporting rate
rate(wandb_requests_total[5m])

# Error rate
rate(wandb_request_errors_total[5m]) / rate(wandb_requests_total[5m])

# Processing latency (P95)
histogram_quantile(0.95, rate(wandb_request_duration_seconds_bucket[5m]))

# Data point throughput
rate(wandb_metrics_data_points_total[5m])

# Storage success rate
rate(wandb_metrics_store_count[5m]) / rate(wandb_metrics_data_points_total[5m])
```

#### 5.3.2 Performance Tuning

**wandb-exporter**:
- `batch_size`: Batch send size (default 10)
- `flush_interval`: Flush interval in seconds (default 5.0)

```python
# Modify default configuration
reporter = AsyncAPIReporter(
    batch_size=50,      # Increase batch size
    flush_interval=10.0 # Increase flush interval
)
```

**telemetry-processor**:
- Increase worker count
- Enable database connection pool
- Use async writes

## Appendix

### A. Complete Data Format Examples

#### A.1 Framework Detection Request

```json
{
  "source": "wandb",
  "type": "framework_detection_raw",
  "version": "1.0",
  "workload_uid": "abc-123",
  "pod_uid": "pod-456",
  "pod_name": "training-pod-0",
  "namespace": "default",
  "evidence": {
    "wandb": {
      "project": "my-training",
      "name": "run-001",
      "id": "wandb-run-id",
      "config": {
        "framework": "primus",
        "base_framework": "megatron"
      },
      "tags": ["distributed", "gpu"]
    },
    "environment": {
      "PRIMUS_CONFIG": "/config/primus.yaml",
      "PRIMUS_BACKEND": "megatron",
      "WORLD_SIZE": "8",
      "RANK": "0"
    },
    "pytorch": {
      "available": true,
      "version": "2.1.0",
      "cuda_available": true,
      "cuda_version": "12.1",
      "detected_modules": {
        "deepspeed": false,
        "megatron": true,
        "transformers": true,
        "lightning": false
      }
    },
    "wrapper_frameworks": {
      "primus": {
        "detected": true,
        "version": "1.0.0",
        "initialized": true,
        "base_framework": "megatron"
      }
    },
    "base_frameworks": {
      "megatron": {
        "detected": true,
        "version": "unknown",
        "initialized": true
      }
    }
  },
  "hints": {
    "wrapper_frameworks": ["primus"],
    "base_frameworks": ["megatron"],
    "possible_frameworks": ["primus", "megatron"],
    "confidence": "high",
    "primary_indicators": [
      "import.primus",
      "PRIMUS env vars",
      "PRIMUS_BACKEND=megatron"
    ]
  },
  "timestamp": 1704067200.123
}
```

#### A.2 Metrics Reporting Request

```json
{
  "source": "wandb",
  "workload_uid": "abc-123",
  "pod_uid": "pod-456",
  "pod_name": "training-pod-0",
  "run_id": "wandb-run-id",
  "metrics": [
    {
      "name": "loss",
      "value": 0.5234,
      "step": 100,
      "timestamp": 1704067200.123,
      "tags": {}
    },
    {
      "name": "accuracy",
      "value": 0.9512,
      "step": 100,
      "timestamp": 1704067200.123,
      "tags": {}
    },
    {
      "name": "learning_rate",
      "value": 0.0001,
      "step": 100,
      "timestamp": 1704067200.123,
      "tags": {}
    }
  ],
  "timestamp": 1704067200.123
}
```

### B. Complete API Route Table

| Module | Method | Path | Function | Handler |
|------|------|------|------|---------|
| telemetry-processor | POST | `/api/v1/wandb/detection` | Framework detection reporting | `ReceiveWandBDetection` |
| telemetry-processor | POST | `/api/v1/wandb/metrics` | Metrics reporting | `ReceiveWandBMetrics` |
| telemetry-processor | POST | `/api/v1/wandb/logs` | Training data reporting | `ReceiveWandBLogs` |
| telemetry-processor | POST | `/api/v1/wandb/batch` | Batch reporting | `ReceiveWandBBatch` |
| api | GET | `/api/v1/workloads/:uid/metrics/sources` | Get data sources list | `GetDataSources` |
| api | GET | `/api/v1/workloads/:uid/metrics/available` | Get available metrics | `GetAvailableMetrics` |
| api | GET | `/api/v1/workloads/:uid/metrics/data` | Get metrics data | `GetMetricsData` |
| api | GET | `/api/v1/workloads/:uid/metrics/iteration-times` | Get iteration times | `GetIterationTimes` |

### C. Supported Frameworks List

#### C.1 Wrapper Frameworks

| Framework | Detection Method | Priority | Description |
|------|---------|--------|------|
| Primus | Import, ENV, Config | Highest | Enterprise training framework |
| PyTorch Lightning | Import, Modules | High | PyTorch high-level wrapper |
| Transformers Trainer | Import | Medium | Hugging Face trainer |

#### C.2 Base Frameworks

| Framework | Detection Method | Priority | Description |
|------|---------|--------|------|
| Megatron-LM | Import, ENV, Config | Highest | NVIDIA large model training |
| DeepSpeed | Import, ENV, Modules | High | Microsoft distributed optimization |
| JAX | Import, ENV | High | Google ML framework |
| Transformers | Import, Modules | Low | Hugging Face model library |

## Language Versions

- [English Documentation](./wandb-integration-technical.md) (Current)
- [中文文档](./wandb-integration-technical-ZH.md)

---

**Documentation Version**: 1.0  
**Last Updated**: 2024-12-03  
**Maintainer**: Primus Lens Team

