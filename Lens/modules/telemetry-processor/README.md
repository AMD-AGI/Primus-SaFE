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

### Log Metrics
- `log_consume_latency_seconds`: Summary of log processing latency by node
- `log_consume_latency_histogram_seconds`: Histogram of log processing latency
- `log_analysis_consume_count`: Counter of logs analyzed by type
- `event_stream_create_count`: Counter of event streams created
- `event_stream_create_error_count`: Counter of event stream creation errors
- `workload_event_create_count`: Counter of workload events created
- `workload_event_create_error_count`: Counter of workload event creation errors

### Metrics Processing
- `gpu_utilization`: GPU utilization metrics (example/test metric)

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

## Contributing

Please follow the project's contribution guidelines and coding standards when submitting changes.

