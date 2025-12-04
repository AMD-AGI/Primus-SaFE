# Telemetry Processor

## Overview

The Telemetry Processor is a core component of the Primus Lens system responsible for collecting, processing, and analyzing telemetry data from Kubernetes workloads. It receives metrics and logs from various sources, enriches them with workload context, and stores the processed data for analysis and monitoring.

## Features

### 1. Metrics Processing
- **Prometheus Protocol Support**: Accepts metrics in Prometheus remote write format
- **Workload Enrichment**: Automatically associates pod-level metrics with workload information
- **Device Mapping**: Maintains mappings between pods and hardware devices (GPU, IB, RDMA, ASIC)
- **Time Series Transformation**: Converts pod metrics to workload metrics with additional labels

### 2. Log Processing
- **HTTP Log Reception**: Receives structured logs via HTTP endpoints
- **Training Performance Extraction**: Parses training metrics from log messages using regex patterns
- **Log Latency Tracking**: Monitors the time between log generation and processing
- **Workload Association**: Links logs to their corresponding workloads

### 3. Caching Mechanisms
- **Device-Pod Cache**: Maintains real-time mappings of devices to pods
- **Pod-Workload Cache**: Tracks relationships between pods and their parent workloads
- **Automatic Refresh**: Periodically updates caches (default: 20 seconds)

### 4. Training Metrics Analysis
- **Performance Tracking**: Extracts detailed training performance metrics including:
  - Iteration progress
  - Memory usage
  - TFLOPS (throughput)
  - Tokens per GPU
  - Learning rate
  - Loss metrics
  - Gradient norms
- **Training Event Detection**: Identifies and records training start events
- **Multiple Format Support**: Handles legacy, standard, and HIP memory formats

### 5. Unified Alert System
- **Multi-Source Alert Reception**: Accepts alerts from metrics (VMAlert), logs, and traces
- **Alert Standardization**: Converts different alert formats into a unified model
- **Alert Enrichment**: Automatically enriches alerts with workload, pod, and node context
- **Correlation Analysis**: Detects relationships between alerts across sources and time
- **Alert Routing**: Routes alerts to appropriate notification channels based on rules
- **Multi-Channel Notifications**: Supports webhook, email, DingTalk, WeChat, Slack, and AlertManager
- **Alert Management**: Full CRUD operations for alert rules and silences
- **Statistics and Querying**: Historical alert data with filtering and aggregation

## Architecture

### Components

```
telemetry-processor/
├── cmd/
│   └── telemetry-processor/     # Main entry point
├── pkg/
│   ├── common/
│   │   └── bootstrap/            # Application initialization
│   └── module/
│       ├── alerts/               # Alert system
│       │   ├── model.go          # Alert data models
│       │   ├── receiver.go       # Alert reception endpoints
│       │   ├── processor.go      # Alert processing logic
│       │   ├── correlator.go     # Alert correlation analysis
│       │   ├── router.go         # Alert routing
│       │   ├── notifier.go       # Notification channels
│       │   └── api.go            # Alert management API
│       ├── logs/                 # Log processing
│       │   ├── receiver.go       # HTTP log endpoint
│       │   ├── training_log.go   # Training metrics extraction
│       │   ├── metrics.go        # Prometheus metrics
│       │   └── model.go          # Data models
│       ├── metrics/              # Metrics processing
│       │   ├── api.go            # HTTP metrics endpoint
│       │   └── processor.go      # Time series processing
│       ├── pods/                 # Cache management
│       │   └── pod_cache.go      # Device and workload caches
│       └── training/             # Training event handling
│           └── detail.go         # Training log processing
```

## API Endpoints

### 1. Metrics Ingestion
- **Endpoint**: `ANY /prometheus`
- **Description**: Receives Prometheus remote write data
- **Format**: Snappy-compressed Protocol Buffers
- **Processing**: Enriches metrics with workload labels and forwards to storage

### 2. Log Reception
- **Endpoint**: `POST /logs`
- **Description**: Receives structured logs from pods
- **Format**: JSON array of log entries
- **Features**:
  - Timestamp conversion
  - Kubernetes metadata extraction
  - Training performance parsing
  - Latency tracking

### 3. Cache Inspection

#### Pod Cache
- **Endpoint**: `GET /pods/cache`
- **Description**: Returns current device-to-pod mappings
- **Format**: `map[nodeName][deviceLabel][deviceName] -> [podName, podUID]`

#### Workload Cache
- **Endpoint**: `GET /pods/workload/cache`
- **Description**: Returns current pod-to-workload mappings
- **Format**: `map[podName] -> [[workloadName, workloadUID]]`

### 4. Alert System

#### Alert Reception
- **Endpoint**: `POST /alerts/metric`
- **Description**: Receives metric alerts from VMAlert
- **Format**: VMAlert webhook format (compatible with Prometheus AlertManager)

- **Endpoint**: `POST /alerts/log`
- **Description**: Receives log-based alerts
- **Format**: JSON with rule_name, severity, message, pattern, workload/pod/node context

- **Endpoint**: `POST /alerts/trace`
- **Description**: Receives trace-based alerts
- **Format**: JSON with rule_name, severity, trace_id, span_id, duration, service context

- **Endpoint**: `POST /alerts/webhook`
- **Description**: Generic webhook for custom alert sources
- **Format**: JSON with flexible schema

#### Alert Query
- **Endpoint**: `GET /alerts`
- **Description**: List alerts with filtering
- **Query Parameters**: source, alert_name, severity, status, workload_id, pod_name, node_name, cluster_name, starts_after, starts_before, offset, limit

- **Endpoint**: `GET /alerts/:id`
- **Description**: Get a single alert by ID
- **Response**: Full alert details with enriched context

- **Endpoint**: `GET /alerts/:id/correlations`
- **Description**: Get correlated alerts for a given alert
- **Response**: List of correlation groups with related alerts

- **Endpoint**: `GET /alerts/statistics`
- **Description**: Get alert statistics
- **Query Parameters**: date_from, date_to, alert_name, source, workload_id, cluster_name, group_by

#### Alert Rule Management
- **Endpoint**: `POST /alert-rules`
- **Description**: Create a new alert rule
- **Request Body**: Rule configuration with name, source, rule_type, rule_config, severity, labels, annotations

- **Endpoint**: `GET /alert-rules`
- **Description**: List all alert rules
- **Query Parameters**: source, enabled

- **Endpoint**: `GET /alert-rules/:id`
- **Description**: Get a single alert rule

- **Endpoint**: `PUT /alert-rules/:id`
- **Description**: Update an alert rule

- **Endpoint**: `DELETE /alert-rules/:id`
- **Description**: Delete an alert rule

#### Silence Management
- **Endpoint**: `POST /silences`
- **Description**: Create a silence to suppress alerts
- **Request Body**: Matchers, starts_at, ends_at, comment, created_by

- **Endpoint**: `GET /silences`
- **Description**: List active silences

- **Endpoint**: `DELETE /silences/:id`
- **Description**: Delete a silence

## Data Models

### Log Entry Structure

```go
{
  "date": 1234567890.123,           // Unix timestamp with fractional seconds
  "stream": "stdout",                // Stream type
  "logtag": "F",                     // Log tag
  "message": "log message",          // Log content
  "kubernetes": {
    "pod_name": "example-pod",
    "namespace_name": "default",
    "pod_id": "uuid",
    "host": "node-name",
    "container_name": "main",
    "labels": {
      "training.kubeflow.org/job-name": "job-1",
      "training.kubeflow.org/replica-type": "worker"
    }
  }
}
```

### Training Performance Metrics

The system extracts the following metrics from training logs:

- **CurrentIteration**: Current training iteration
- **TargetIteration**: Total target iterations
- **ConsumedSamples**: Number of samples processed
- **ElapsedTimePerIterationMS**: Time per iteration in milliseconds
- **MemUsage**: Memory usage in GB
- **MemFree**: Free memory in GB
- **MemTotal**: Total memory in GB
- **MemUsageRatio**: Memory usage percentage
- **TFLOPS**: Throughput in TFLOP/s per GPU
- **TokensPerGPU**: Tokens processed per second per GPU
- **LearningRate**: Current learning rate
- **GlobalBatchSize**: Batch size
- **LmLoss**: Language model loss
- **LossScale**: Loss scaling factor
- **GradNorm**: Gradient norm
- **NumZeros**: Number of zero gradients
- **SkippedIterationsNumber**: Iterations skipped
- **NanIterationsNumber**: Iterations with NaN values

## Configuration

The telemetry processor uses the core Lens configuration system. Configuration is typically provided through:

- Configuration files (YAML/JSON)
- Environment variables
- Command-line flags

Key configuration areas:
- Database connection settings
- Prometheus storage endpoints
- Cache refresh intervals
- Log parsing patterns

## Deployment

### Prerequisites

- Go 1.24.5 or later
- Access to Kubernetes cluster
- Database (PostgreSQL/MySQL)
- Prometheus-compatible storage backend

### Building

```bash
cd cmd/telemetry-processor
go build -o telemetry-processor main.go
```

### Running

```bash
./telemetry-processor --config /path/to/config.yaml
```

## Monitoring

The telemetry processor exposes its own Prometheus metrics for self-monitoring:

### Log Processing Metrics
- `log_consume_latency_seconds`: Summary of log processing latency by node
- `log_consume_latency_histogram_seconds`: Histogram of log processing latency
- `log_analysis_consume_count`: Counter of logs analyzed by type
- `event_stream_create_count`: Counter of event streams created
- `event_stream_create_error_count`: Counter of event stream creation errors
- `workload_event_create_count`: Counter of workload events created
- `workload_event_create_error_count`: Counter of workload event creation errors

### WandB Metrics (New)
- `wandb_request_total`: Total WandB requests by type (metrics/logs/detection)
- `wandb_request_error_total`: Total WandB request errors by type and error category
- `wandb_request_duration_seconds`: WandB request processing duration histogram
- `wandb_metrics_datapoint_count`: WandB metrics data point count distribution
- `wandb_metrics_store_total`: WandB metrics storage success count
- `wandb_metrics_store_error_total`: WandB metrics storage error count
- `wandb_logs_datapoint_count`: WandB logs data point count distribution

### Framework Detection Metrics (New)
- `log_framework_detection_total`: Framework detection count by framework, method, and source (from log processing)
- `framework_detection_confidence`: Framework detection confidence distribution
- `framework_detection_error_total`: Framework detection errors by source and error type

### Log Pattern Matching Metrics (New)
- `log_pattern_match_total`: Successful log pattern matches by framework and pattern type
- `log_pattern_match_error_total`: Log pattern match errors by framework and error type

### Training Performance Metrics (New)
- `training_performance_save_total`: Training performance data saves by workload and source
- `training_performance_save_error_total`: Training performance save errors by workload and source

### Checkpoint Metrics (New)
- `checkpoint_event_total`: Checkpoint events by event type and framework
- `checkpoint_event_error_total`: Checkpoint event errors by type and framework

### Container Event Metrics
- `primus_lens_telemetry_processor_container_event_recv_total`: Container events received
- `primus_lens_telemetry_processor_container_event_error_total`: Container event errors
- `primus_lens_telemetry_processor_container_event_processing_duration_seconds`: Event processing duration
- `primus_lens_telemetry_processor_container_event_batch_size`: Event batch size distribution

**📊 For detailed metrics documentation, PromQL queries, and usage examples, see:**
- [Metrics Documentation](pkg/module/logs/METRICS.md)
- [Metrics Usage Examples](pkg/module/logs/METRICS_USAGE_EXAMPLES.md)
- [Metrics Summary](pkg/module/logs/METRICS_SUMMARY.md)

## Performance Considerations

### Cache Management
- Caches refresh every 20 seconds by default
- In-memory caches for fast lookups
- Efficient database queries using workload UIDs

### Log Processing
- Asynchronous processing
- Batch operations where possible
- Regex compilation at startup for performance

### Metrics Processing
- Filters negative values
- Deduplicates metrics by iteration
- Adds workload context to reduce query complexity

## Training Log Format Support

The processor supports multiple training log formats:

1. **Primus Legacy Format**: Original Megatron-style logs
2. **Primus ROCm Format**: Logs with ROCm memory information
3. **Primus HIP Format**: Logs with HIP memory information

All formats extract comprehensive performance metrics including:
- Training progress
- Memory utilization
- Computational throughput
- Model training metrics

## Error Handling

The processor implements robust error handling:
- Invalid log data is logged but doesn't stop processing
- Negative metric values are filtered out
- Missing workload references are handled gracefully
- Database errors are logged with context

## Dependencies

Key dependencies:
- `github.com/VictoriaMetrics/VictoriaMetrics`: Prometheus protocol support
- `github.com/gin-gonic/gin`: HTTP server framework
- `github.com/prometheus/client_golang`: Prometheus client
- `k8s.io/client-go`: Kubernetes client
- `gorm.io/gorm`: Database ORM

See `go.mod` for complete dependency list.

## Development

### Project Structure
- Follow standard Go project layout
- Use the core Lens packages for common functionality
- Implement new log parsers by adding regex patterns in `training_log.go`

### Adding New Metrics
1. Define new fields in `model.TrainingPerformance`
2. Update regex patterns in `perfRegexps`
3. Update database schema if needed

### Testing
```bash
go test ./...
```

## Integration with Primus Lens

The telemetry processor integrates with other Lens components:

- **Core Module**: Provides configuration, logging, database access
- **API Module**: Shares REST API framework
- **Storage**: Writes metrics to Prometheus/VictoriaMetrics
- **Database**: Stores training events and performance data

## Troubleshooting

### Logs Not Being Processed
1. Check if logs are reaching the endpoint (check HTTP logs)
2. Verify log format matches expected structure
3. Check database connectivity
4. Review regex patterns for training log parsing

### Metrics Not Appearing
1. Verify Prometheus remote write is configured correctly
2. Check pod-device cache is populated (`GET /pods/cache`)
3. Verify workload cache is populated (`GET /pods/workload/cache`)
4. Check for negative values being filtered out

### High Latency
1. Monitor cache refresh performance
2. Check database query performance
3. Review log volume and consider batching
4. Check Prometheus write latency

## License

This project is part of the Primus Lens system developed by AMD-AGI.

## Alert System Details

### Alert Architecture

The unified alert system provides a centralized platform for receiving, processing, correlating, and routing alerts from multiple sources.

#### Alert Flow
```
1. Alert Sources (VMAlert/Logs/Traces)
   ↓
2. Reception Layer (standardization)
   ↓
3. Processing Layer (enrichment, deduplication)
   ↓
4. Storage Layer (PostgreSQL)
   ↓
5. Correlation Layer (relationship detection)
   ↓
6. Routing Layer (rule-based routing)
   ↓
7. Notification Layer (multi-channel delivery)
```

### Alert Sources

#### 1. Metric Alerts (from VMAlert)
VMAlert evaluates metric-based rules and sends webhooks to the telemetry processor.

**Configuration Example:**
```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMAlert
metadata:
  name: primus-lens-vmalert
spec:
  notifiers:
    - url: "http://primus-lens-telemetry-processor:8989/v1/alerts/metric"
```

**Webhook Payload:**
```json
{
  "alerts": [{
    "status": "firing",
    "labels": {
      "alertname": "GPUMemoryHigh",
      "severity": "warning",
      "workload_id": "training-job-123",
      "pod": "worker-0",
      "node": "gpu-node-01"
    },
    "annotations": {
      "summary": "GPU memory usage above 90%",
      "description": "GPU 0 on gpu-node-01 memory is at 95%"
    },
    "startsAt": "2025-11-03T10:00:00Z",
    "fingerprint": "abc123def456"
  }]
}
```

#### 2. Log Alerts
Log-based alerts are triggered by log pattern matching or log analysis.

**API Example:**
```bash
curl -X POST http://telemetry-processor:8989/v1/alerts/log \
  -H "Content-Type: application/json" \
  -d '{
    "rule_name": "OOMDetected",
    "severity": "critical",
    "message": "CUDA out of memory error detected",
    "pattern": "OutOfMemoryError|CUDA out of memory",
    "workload_id": "training-job-123",
    "pod_name": "worker-0",
    "pod_id": "uuid-456",
    "log_time": "2025-11-03T10:05:00Z",
    "labels": {
      "error_type": "oom"
    }
  }'
```

#### 3. Trace Alerts
Trace-based alerts are triggered by performance anomalies detected in distributed traces.

**API Example:**
```bash
curl -X POST http://telemetry-processor:8989/v1/alerts/trace \
  -H "Content-Type: application/json" \
  -d '{
    "rule_name": "SlowOperation",
    "severity": "warning",
    "message": "Data loading operation exceeded threshold",
    "trace_id": "trace-789",
    "span_id": "span-101",
    "service_name": "dataloader",
    "operation": "load_batch",
    "duration": 5000,
    "workload_id": "training-job-123"
  }'
```

### Alert Correlation

The system automatically correlates alerts based on:

1. **Time-based Correlation**: Alerts occurring within a time window (default: 5 minutes)
2. **Entity-based Correlation**: Alerts related to the same workload, pod, or node
3. **Cross-source Correlation**: Alerts from different sources (metric + log + trace)
4. **Causal Correlation**: Known cause-effect relationships (e.g., high memory → OOM)

**Example Correlation:**
```json
{
  "correlation_id": "corr-abc123",
  "alerts": [
    {
      "id": "alert-1",
      "source": "metric",
      "alert_name": "GPUMemoryHigh",
      "starts_at": "2025-11-03T10:00:00Z"
    },
    {
      "id": "alert-2",
      "source": "log",
      "alert_name": "OOMDetected",
      "starts_at": "2025-11-03T10:02:00Z"
    }
  ],
  "correlation_type": "causal",
  "correlation_score": 0.85,
  "reason": "high GPU memory leads to OOM; same workload; occurred within 2 minutes"
}
```

### Alert Routing and Notifications

#### Routing Rules
Alerts can be routed to different channels based on matchers:

```json
{
  "matchers": [
    {"name": "severity", "value": "critical"},
    {"name": "workload_id", "value": "prod-*", "is_regex": true}
  ],
  "channels": [
    {
      "type": "webhook",
      "config": {
        "url": "http://alert-handler/critical"
      }
    },
    {
      "type": "dingtalk",
      "config": {
        "webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=xxx"
      }
    }
  ],
  "group_by": ["alertname", "workload_id"],
  "group_wait": "30s",
  "repeat_interval": "4h"
}
```

#### Supported Channels
- **Webhook**: Generic HTTP POST
- **Email**: SMTP-based email notifications
- **DingTalk**: DingTalk bot webhooks
- **WeChat**: WeChat Work bot webhooks
- **Slack**: Slack incoming webhooks
- **AlertManager**: Forward to Prometheus AlertManager

### Alert Silences

Silences suppress notifications for alerts matching specific criteria:

**Create Silence Example:**
```bash
curl -X POST http://telemetry-processor:8989/v1/silences \
  -H "Content-Type: application/json" \
  -d '{
    "matchers": [
      {"name": "alertname", "value": "GPUMemoryHigh"},
      {"name": "workload_id", "value": "test-job-*"}
    ],
    "starts_at": "2025-11-03T10:00:00Z",
    "ends_at": "2025-11-03T12:00:00Z",
    "comment": "Maintenance window for test workloads",
    "created_by": "admin@example.com"
  }'
```

### Alert Rules

Dynamic alert rules can be created for log and trace-based alerting:

**Create Log Alert Rule:**
```bash
curl -X POST http://telemetry-processor:8989/v1/alert-rules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "NCCLError",
    "source": "log",
    "enabled": true,
    "rule_type": "pattern",
    "rule_config": {
      "pattern": "NCCL error|NCCL WARN",
      "threshold": {
        "count": 3,
        "window": "10m"
      }
    },
    "severity": "warning",
    "labels": {
      "category": "network"
    },
    "annotations": {
      "summary": "NCCL communication errors detected",
      "description": "Multiple NCCL errors indicate network issues"
    }
  }'
```

### Database Schema

The alert system uses the following tables:

1. **alert_events**: Stores all alert events with enriched context
2. **alert_correlations**: Stores relationships between alerts
3. **alert_statistics**: Aggregated alert statistics for fast querying
4. **alert_rules**: Dynamic alert rule configurations
5. **alert_silences**: Silence configurations
6. **alert_notifications**: Notification history and status

### Querying Alerts

**List Recent Critical Alerts:**
```bash
curl "http://telemetry-processor:8989/v1/alerts?severity=critical&limit=20"
```

**Get Alerts for a Workload:**
```bash
curl "http://telemetry-processor:8989/v1/alerts?workload_id=training-job-123&status=firing"
```

**Get Alert Statistics:**
```bash
curl "http://telemetry-processor:8989/v1/alerts/statistics?date_from=2025-11-01&date_to=2025-11-03&group_by=day"
```

### Integration with VMAlert

To configure VMAlert to send alerts to the telemetry processor:

1. Deploy VMAlert with operator
2. Configure notifier URL to point to telemetry processor
3. Create VMRule resources with alert rules
4. Alerts will be automatically sent to the processor

**Example VMRule:**
```yaml
apiVersion: operator.victoriametrics.com/v1beta1
kind: VMRule
metadata:
  name: gpu-alerts
spec:
  groups:
    - name: gpu_health
      interval: 30s
      rules:
        - alert: GPUMemoryHigh
          expr: gpu_memory_used / gpu_memory_total > 0.9
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "GPU memory usage high"
            description: "GPU {{ $labels.gpu_id }} memory is at {{ $value }}%"
```

### Performance Considerations

- **Async Processing**: Correlation and notification are performed asynchronously
- **Batch Operations**: Statistics are updated in batches
- **Index Optimization**: Database indexes on frequently queried fields
- **Cache Usage**: In-memory caching for active alerts and rules
- **Time-based Cleanup**: Old alert events are automatically archived

## Contributing

Please follow the project's contribution guidelines and coding standards when submitting changes.

