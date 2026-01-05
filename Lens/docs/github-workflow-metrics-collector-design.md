# GitHub Workflow Metrics Collector Design

## Overview

This feature allows users to:
1. **Configuration Registration**: Register GitHub repositories, workflows, and result file paths (supporting glob patterns)
2. **Auto Collection**: Automatically access configured file paths after workflow runs complete
3. **Smart Extraction**: Leverage AI Agent to extract metric fields from markdown/csv/json files
4. **Data Storage**: Convert extracted data into time-series metrics stored in database
5. **Dashboard Display**: Support generating visualizations using these metrics

---

## Trigger Mechanism

### Design Principle

Instead of polling GitHub API to discover new workflow runs, we leverage Lens's existing workload tracking system and the native hierarchy of GitHub Actions Runner Controller:

1. **AutoscalingRunnerSet Binding**: Users bind a configuration to a specific `AutoscalingRunnerSet` (ARS) workload.
2. **EphemeralRunner Scanning**: The system scans all `EphemeralRunner` workloads that are children of the bound ARS.
3. **Completion-Driven**: When an EphemeralRunner completes, the system triggers metric collection.
4. **Historical Backfill**: Users can query and process all historical EphemeralRunners under an ARS.

### GitHub Actions Runner Controller Hierarchy

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  GitHub Actions Runner Controller Architecture           │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    AutoscalingRunnerSet (ARS)                    │    │
│  │                                                                  │    │
│  │  Kind: AutoscalingRunnerSet                                     │    │
│  │  Name: "benchmark-runners"                                      │    │
│  │  Namespace: "actions-runner"                                    │    │
│  │  → Manages a pool of runners for specific repository/workflow   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│                              │ creates/manages                           │
│                              ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    EphemeralRunner (Children)                    │    │
│  │                                                                  │    │
│  │  Kind: EphemeralRunner                                          │    │
│  │  Name: "benchmark-runners-xxxxx"                                │    │
│  │  Parent: benchmark-runners (AutoscalingRunnerSet)               │    │
│  │  Labels:                                                        │    │
│  │    - actions.github.com/runner-group-name                       │    │
│  │    - actions.github.com/scale-set-name                          │    │
│  │  Annotations:                                                   │    │
│  │    - actions.github.com/run-id                                  │    │
│  │    - actions.github.com/job-id                                  │    │
│  │  → Each runner executes one workflow job, then terminates       │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Matching Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Workload Matching Flow                           │
│                                                                          │
│  1. User creates config bound to AutoscalingRunnerSet                   │
│     └─ Specify: namespace + ARS name (or UID)                           │
│                                                                          │
│  2. System queries gpu_workloads for:                                   │
│     └─ kind = 'EphemeralRunner'                                         │
│     └─ parent_workload matches the bound ARS                            │
│     └─ status = 'Completed'                                             │
│                                                                          │
│  3. For each matched EphemeralRunner:                                   │
│     └─ Extract GitHub context from labels/annotations                  │
│     └─ Trigger file collection from GitHub                             │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Benefits

| Benefit | Description |
|---------|-------------|
| **Simple Configuration** | Just bind to an AutoscalingRunnerSet, no complex selectors |
| **No GitHub Polling** | Leverages existing workload tracking, avoids rate limiting |
| **Accurate Matching** | Uses native parent-child relationship, no false positives |
| **Historical Backfill** | Can process any historical EphemeralRunner under the ARS |
| **Rich Context** | Access to GitHub job info from runner labels/annotations |
| **Unified Tracking** | Workload lifecycle already managed by Lens |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          GitHub Workflow Metrics Collector               │
│                                                                          │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐  │
│  │   API Module      │    │   Jobs Module     │    │   AI Gateway     │  │
│  │                  │    │                  │    │                  │  │
│  │ - CRUD Configs   │    │ - Workload Scan   │    │ - Field Extract  │  │
│  │ - Query Metrics  │    │ - File Collector  │    │ - Schema Gen     │  │
│  │ - Backfill API   │    │ - AI Extractor    │    │                  │  │
│  └────────┬─────────┘    └────────┬─────────┘    └────────┬─────────┘  │
│           │                       │                       │            │
│  ┌────────▼───────────────────────▼───────────────────────▼─────────┐  │
│  │                          Core Module                              │  │
│  │                                                                   │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │  │
│  │  │ Database    │  │ AI Client   │  │ PVC Reader  │              │  │
│  │  │ Facades     │  │             │  │ (via node-  │              │  │
│  │  │             │  │             │  │  exporter)  │              │  │
│  │  │ + Workload  │  │             │  │             │              │  │
│  │  │   Facade    │  │             │  │             │              │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘              │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                          PostgreSQL                               │  │
│  │                                                                   │  │
│  │  - github_workflow_configs      (Configuration)                   │  │
│  │  - github_workflow_runs         (Run Records - linked to workload)│  │
│  │  - github_workflow_metrics      (Metric Data)                     │  │
│  │  - github_workflow_metric_schemas (Metric Schema Definitions)     │  │
│  │  - gpu_workloads                (Existing - source of trigger)    │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Data Model Design

### 1. `github_workflow_configs` - Configuration Table

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| name | VARCHAR(255) | Configuration name |
| description | TEXT | Configuration description |
| **AutoscalingRunnerSet Binding** | | |
| runner_set_namespace | VARCHAR(255) | Namespace of the AutoscalingRunnerSet |
| runner_set_name | VARCHAR(255) | Name of the AutoscalingRunnerSet |
| runner_set_uid | VARCHAR(255) | UID of the AutoscalingRunnerSet (optional, for precise matching) |
| **GitHub Configuration** | | |
| github_owner | VARCHAR(255) | Repository owner |
| github_repo | VARCHAR(255) | Repository name |
| workflow_filter | VARCHAR(255) | Workflow filename filter (optional, e.g., `benchmark.yml`) |
| branch_filter | VARCHAR(255) | Branch filter (optional, e.g., `main`) |
| **File Collection** | | |
| file_patterns | JSONB | File path pattern list, e.g., `["bench/**/*.json", "results/*.csv"]` |
| **AI Extraction** | | |
| metric_schema_id | BIGINT | Associated schema ID (optional, populated after AI generates) |
| **Status** | | |
| enabled | BOOLEAN | Whether enabled |
| last_processed_workload_uid | VARCHAR(255) | Last processed EphemeralRunner UID |
| last_checked_at | TIMESTAMP | Last check time |
| cluster_name | VARCHAR(255) | Cluster name |
| created_at | TIMESTAMP | Creation time |
| updated_at | TIMESTAMP | Update time |

**Configuration Example**:
```json
{
  "name": "LLM Benchmark Metrics",
  "runner_set_namespace": "actions-runner",
  "runner_set_name": "benchmark-runners",
  "github_owner": "AMD-AGI",
  "github_repo": "Primus-SaFE",
  "workflow_filter": "benchmark.yml",
  "file_patterns": ["Bench/results/**/*.json", "Bench/results/**/*.csv"]
}
```

**Unique Constraint**: `(runner_set_namespace, runner_set_name, cluster_name)`

### 2. `github_workflow_runs` - Run Records Table

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| config_id | BIGINT | Associated configuration ID |
| **Workload Reference** | | |
| workload_uid | VARCHAR(255) | Lens workload UID (from gpu_workloads table) |
| workload_name | VARCHAR(255) | Workload name |
| workload_namespace | VARCHAR(255) | Workload namespace |
| **GitHub Reference (Optional)** | | |
| github_run_id | BIGINT | GitHub workflow run ID (extracted from workload if available) |
| github_run_number | INT | Run number |
| head_sha | VARCHAR(64) | Commit SHA |
| head_branch | VARCHAR(255) | Branch name |
| **Status** | | |
| status | VARCHAR(50) | Status: pending, collecting, extracting, completed, failed, skipped |
| trigger_source | VARCHAR(50) | Trigger source: realtime, backfill, manual |
| **Collection Info** | | |
| files_found | INT | Number of files found |
| files_processed | INT | Number of files processed |
| metrics_count | INT | Number of metrics extracted |
| **Timestamps** | | |
| workload_started_at | TIMESTAMP | Workload start time (from gpu_workloads) |
| workload_completed_at | TIMESTAMP | Workload completion time (from gpu_workloads) |
| collection_started_at | TIMESTAMP | Collection start time |
| collection_completed_at | TIMESTAMP | Collection completion time |
| **Error Handling** | | |
| error_message | TEXT | Error message |
| retry_count | INT | Number of retries |
| created_at | TIMESTAMP | Creation time |
| updated_at | TIMESTAMP | Update time |

**Unique Constraint**: `(config_id, workload_uid)`

**Indexes**:
- `(config_id, status)` - For pending job queries
- `(workload_uid)` - For workload lookup
- `(workload_completed_at)` - For time-based queries

### 3. `github_workflow_metric_schemas` - Metric Schema Definitions

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| config_id | BIGINT | Associated configuration ID |
| name | VARCHAR(255) | Schema name |
| version | INT | Version number |
| fields | JSONB | Field definitions list |
| dimension_fields | JSONB | Dimension fields (for grouping/aggregation) |
| metric_fields | JSONB | Metric fields (numeric) |
| is_active | BOOLEAN | Whether active |
| created_at | TIMESTAMP | Creation time |
| updated_at | TIMESTAMP | Update time |

**Fields Example**:
```json
[
  {"name": "model_name", "type": "string", "description": "Model name"},
  {"name": "throughput", "type": "float", "unit": "tokens/s", "description": "Throughput"},
  {"name": "latency_p50", "type": "float", "unit": "ms", "description": "P50 latency"},
  {"name": "latency_p99", "type": "float", "unit": "ms", "description": "P99 latency"},
  {"name": "gpu_memory_used", "type": "float", "unit": "GB", "description": "GPU memory usage"}
]
```

### 4. `github_workflow_metrics` - Metric Data Table

| Column | Type | Description |
|--------|------|-------------|
| id | BIGSERIAL | Primary key |
| config_id | BIGINT | Configuration ID |
| run_id | BIGINT | Associated workflow run |
| schema_id | BIGINT | Schema used |
| timestamp | TIMESTAMP | Metric timestamp |
| source_file | VARCHAR(1024) | Source file path |
| dimensions | JSONB | Dimension values, e.g., `{"model_name": "llama-70b", "batch_size": 32}` |
| metrics | JSONB | Metric values, e.g., `{"throughput": 1234.5, "latency_p50": 10.2}` |
| raw_data | JSONB | Raw data (optional, for debugging) |
| created_at | TIMESTAMP | Creation time |

**Indexes**:
- `(config_id, timestamp DESC)` - For time-series queries
- `(run_id)` - For run-based queries
- GIN index on `dimensions` - For dimension filtering

---

## API Design

### Configuration Management API

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/github-workflow-configs` | Create configuration |
| GET | `/api/github-workflow-configs` | List configurations |
| GET | `/api/github-workflow-configs/:id` | Get configuration details |
| PUT | `/api/github-workflow-configs/:id` | Update configuration |
| DELETE | `/api/github-workflow-configs/:id` | Delete configuration |
| POST | `/api/github-workflow-configs/:id/enable` | Enable configuration |
| POST | `/api/github-workflow-configs/:id/disable` | Disable configuration |
| POST | `/api/github-workflow-configs/:id/test` | Test configuration (manually trigger collection) |

### EphemeralRunner Discovery API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/github-workflow-configs/:id/runners` | List EphemeralRunners under the bound AutoscalingRunnerSet |
| GET | `/api/github-workflow-configs/:id/runners/:runner_uid` | Get details of a specific EphemeralRunner |

**Query Parameters for runners**:
- `start_time` - Start time for query
- `end_time` - End time for query
- `status` - Status filter: `completed`, `running`, `failed`, `all`
- `processed` - Filter by processing status: `true`, `false`, `all`
- `workflow` - Filter by workflow name (from annotations)
- `page`, `page_size` - Pagination

### AutoscalingRunnerSet Discovery API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/autoscaling-runner-sets` | List all AutoscalingRunnerSets in the cluster |
| GET | `/api/autoscaling-runner-sets/:namespace/:name` | Get details of a specific AutoscalingRunnerSet |
| GET | `/api/autoscaling-runner-sets/:namespace/:name/stats` | Get statistics (total runs, success rate, etc.) |

### Backfill API

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/github-workflow-configs/:id/backfill` | Trigger backfill for historical workloads |
| GET | `/api/github-workflow-configs/:id/backfill/status` | Get backfill progress |
| POST | `/api/github-workflow-configs/:id/backfill/cancel` | Cancel ongoing backfill |

**Backfill Request Body**:
```json
{
  "start_time": "2024-01-01T00:00:00Z",
  "end_time": "2024-12-31T23:59:59Z",
  "workload_uids": ["uid1", "uid2"],  // Optional: specific workloads
  "dry_run": false                     // Preview without processing
}
```

### Run Records API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/github-workflow-configs/:id/runs` | Get run records list |
| GET | `/api/github-workflow-configs/:id/runs/:run_id` | Get run details |
| POST | `/api/github-workflow-configs/:id/runs/:run_id/retry` | Retry failed run |
| POST | `/api/github-workflow-configs/:id/runs/batch-retry` | Batch retry failed runs |

### Schema Management API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/github-workflow-configs/:id/schema` | Get current schema |
| PUT | `/api/github-workflow-configs/:id/schema` | Update schema |
| POST | `/api/github-workflow-configs/:id/schema/regenerate` | Regenerate schema (AI) |

### Metrics Query API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/github-workflow-metrics` | Query metrics data |
| GET | `/api/github-workflow-metrics/summary` | Get summary statistics |
| GET | `/api/github-workflow-metrics/dimensions` | Get available dimension values |
| GET | `/api/github-workflow-metrics/trends` | Get trend data |

**Metrics Query Parameters**:
- `config_id` - Configuration ID
- `start_time` - Start time
- `end_time` - End time
- `dimensions[key]=value` - Dimension filtering
- `metrics` - Metrics to return (comma-separated)
- `group_by` - Grouping fields
- `interval` - Aggregation interval (e.g., 1d, 1h)

---

## Job Design

### 1. `EphemeralRunnerScanJob` - EphemeralRunner Scanner

**Responsibility**: Scan recently completed EphemeralRunners under configured AutoscalingRunnerSets

**Schedule**: Every 1 minute

**Workflow**:
```
┌─────────────────────────────────────────────────────────────────────────┐
│                     EphemeralRunner Scan Flow                            │
│                                                                          │
│  1. Get all enabled configurations                                       │
│                                                                          │
│  2. For each configuration:                                              │
│     a. Query gpu_workloads for:                                         │
│        - kind = 'EphemeralRunner'                                       │
│        - parent matches (runner_set_namespace, runner_set_name)         │
│        - status = 'Completed'                                           │
│        - completed_at > last_checked_at                                 │
│                                                                          │
│     b. Apply optional filters:                                          │
│        - workflow_filter (from annotations)                             │
│        - branch_filter (from annotations)                               │
│                                                                          │
│     c. For each matched EphemeralRunner not yet processed:              │
│        - Create github_workflow_runs record (status=pending)            │
│                                                                          │
│     d. Update last_checked_at                                           │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Key Features**:
- Simple parent-child relationship query, no complex selector logic
- Filters by AutoscalingRunnerSet name/namespace
- Optional workflow/branch filtering from GitHub annotations
- Deduplicates already-processed runners

### 2. `GitHubWorkflowCollectorJob` - File Collector

**Responsibility**: Collect result files from pending workflow runs

**Schedule**: Every 1 minute

**File Access Method**: Since EphemeralRunner pods are deleted after completion, files are read by creating a temporary pod that mounts the same PVC as the original EphemeralRunner.

**Workflow**:
```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Collection Flow                                  │
│                                                                          │
│  1. Get workflow runs with status=pending (limit batch size)             │
│                                                                          │
│  2. For each run:                                                        │
│     a. Update status to 'collecting'                                    │
│     b. Get AutoscalingRunnerSet volume configuration                    │
│     c. Create temporary pod with same PVC mounts                        │
│     d. Wait for temp pod to be Running                                  │
│     e. List files in temp pod container matching file_patterns          │
│     f. Read file contents via node-exporter container FS API            │
│     g. Parse file contents (JSON, CSV, Markdown)                        │
│     h. Call AI Agent to extract metrics (if needed)                     │
│     i. Store metric data to github_workflow_metrics                     │
│     j. Delete temporary pod                                             │
│     k. Update status to 'completed' or 'failed'                         │
│                                                                          │
│  3. Handle errors with retry logic                                      │
│     - If PVC not available, mark as 'skipped'                           │
│                                                                          │
│  Note: EphemeralRunner pods are always deleted after completion.        │
│        Temporary pods are used to access the PVC for file reading.      │
└─────────────────────────────────────────────────────────────────────────┘
```

**PVC Reader Architecture**:
```
┌─────────────────────────────────────────────────────────────────────────┐
│                         PVC File Reading Flow                            │
│                                                                          │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────────────────┐ │
│  │ Collector    │────▶│ PVC Reader   │────▶│ Node-Exporter Client     │ │
│  │ Job          │     │              │     │                          │ │
│  └──────────────┘     └──────────────┘     └────────────┬─────────────┘ │
│                                                          │               │
│                                                          ▼               │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                      Node-Exporter (DaemonSet)                    │   │
│  │                                                                   │   │
│  │  /v1/container-fs/list  - List files in container directory      │   │
│  │  /v1/container-fs/read  - Read file content from container       │   │
│  │                                                                   │   │
│  │  Access container filesystem via /proc/{pid}/root                │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Temporary Pod for PVC Access**:

Since EphemeralRunner pods are always deleted after workflow completion,
the system creates a temporary pod to mount the PVC and read result files:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Temporary Pod Creation Flow                           │
│                                                                          │
│  1. Get AutoscalingRunnerSet (ARS) from configuration                   │
│     └─ namespace: config.runner_set_namespace                           │
│     └─ name: config.runner_set_name                                     │
│                                                                          │
│  2. Extract volume configuration from ARS:                              │
│     a. Parse spec.template.spec.volumes                                 │
│     b. Parse spec.template.spec.containers[0].volumeMounts              │
│     c. Filter to PVC-backed volumes only                                │
│                                                                          │
│  3. Create temporary pod:                                               │
│     - Name: lens-pvc-reader-{configID}-{runID}                         │
│     - Image: busybox:latest (minimal)                                   │
│     - Command: sleep 3600                                               │
│     - Volumes: PVC volumes from ARS template                            │
│     - VolumeMounts: Same mounts as ARS template                         │
│     - Labels: primus-lens.amd.com/temp-pvc-reader=true                 │
│     - TTL annotation for safety cleanup                                 │
│                                                                          │
│  4. Wait for pod to be Running (timeout: 2 minutes)                     │
│                                                                          │
│  5. Read files via node-exporter container FS API                       │
│                                                                          │
│  6. Delete temporary pod immediately after reading                      │
│                                                                          │
│  7. Cleanup job removes any orphaned temp pods (expired TTL)            │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

**Temporary Pod Specification**:
- **Image**: `busybox:latest` (minimal footprint)
- **Command**: `sleep 3600` (keeps pod alive during file reading)
- **Volumes**: PVC volumes extracted from AutoscalingRunnerSet template
- **TTL**: 10 minutes (auto-cleanup if not explicitly deleted)
- **Labels**: 
  - `primus-lens.amd.com/temp-pvc-reader=true`
  - `primus-lens.amd.com/config-id={configID}`
  - `primus-lens.amd.com/run-id={runID}`

### 3. `GitHubWorkflowBackfillJob` - Historical Backfill (On-Demand)

**Responsibility**: Process historical workloads triggered by user via API

**Schedule**: On-demand (triggered by backfill API)

**Workflow**:
```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Backfill Flow                                    │
│                                                                          │
│  1. User calls POST /api/github-workflow-configs/:id/backfill           │
│     with time range or specific workload UIDs                           │
│                                                                          │
│  2. Query historical workloads matching:                                │
│     - Configuration's selector rules                                    │
│     - Time range (start_time to end_time)                               │
│     - Not already processed                                             │
│                                                                          │
│  3. Create github_workflow_runs records with:                           │
│     - status = 'pending'                                                │
│     - trigger_source = 'backfill'                                       │
│                                                                          │
│  4. GitHubWorkflowCollectorJob picks up and processes                   │
│                                                                          │
│  5. User can track progress via backfill status API                     │
└─────────────────────────────────────────────────────────────────────────┘
```

**Benefits of Backfill**:
- Process any historical benchmark results
- Re-process failed runs with updated configuration
- Fill gaps when configuration was added later

---

## AI Agent Integration

### New AI Topic: `github.metrics.extract`

**Request Payload**:
- `config_id` - Configuration ID
- `file_type` - File type (json, csv, markdown)
- `file_content` - File content
- `file_path` - File path
- `custom_prompt` - User-defined prompt (optional)
- `existing_schema` - Existing schema (optional)

**Response Payload**:
- `schema` - Generated schema (if no existing schema)
- `metrics` - Extracted metric data list
- `explanation` - Extraction explanation (optional)

### AI Extraction Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         AI Metric Extraction Flow                        │
│                                                                          │
│  ┌──────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────┐  │
│  │ Raw File │───▶│ First Extract│───▶│ Generate     │───▶│ User     │  │
│  │ (csv/json│    │ AI Analyzes  │    │ Schema       │    │ Confirms │  │
│  │ /md)     │    │ Content      │    │ Define Types │    │ or Adjust│  │
│  └──────────┘    └──────────────┘    └──────────────┘    └──────────┘  │
│                                                                  │      │
│                                                                  ▼      │
│  ┌──────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────┐  │
│  │ Store    │◀───│ Extract Data │◀───│ Subsequent   │◀───│ Schema   │  │
│  │ Metrics  │    │ per Schema   │    │ Uses Existing│    │ Confirmed│  │
│  └──────────┘    └──────────────┘    └──────────────┘    └──────────┘  │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Code Structure

```
Lens/modules/  (Primus-SaFE/Lens)
├── core/pkg/
│   ├── database/
│   │   ├── migrations/
│   │   │   └── patch043-github_workflow_metrics.sql  # ✅ Database schema
│   │   ├── model/
│   │   │   ├── github_workflow_configs.gen.go        # ✅ Generated
│   │   │   ├── github_workflow_runs.gen.go           # ✅ Generated
│   │   │   ├── github_workflow_metrics.gen.go        # ✅ Generated
│   │   │   └── github_workflow_metric_schemas.gen.go # ✅ Generated
│   │   ├── github_workflow_config_facade.go          # ✅ Implemented
│   │   ├── github_workflow_run_facade.go             # ✅ Implemented
│   │   ├── github_workflow_schema_facade.go          # ✅ Implemented
│   │   └── github_workflow_metrics_facade.go         # ✅ Implemented
│   │
│   ├── aiclient/
│   │   ├── client.go                      # ✅ AI Client (with GetGlobalClient)
│   │   └── errors.go                      # ✅ Error types (with APIError)
│   │
│   └── aitopics/
│       ├── topics.go                      # ✅ Topic constants
│       ├── registry.go                    # ✅ Topic registry
│       └── github_metrics.go              # ✅ AI topic definition (Phase 3)
│
├── api/pkg/api/
│   ├── github_workflow_metrics.go         # ✅ Config CRUD + Run APIs + AI Schema APIs
│   └── router.go                          # ✅ Route registration
│
├── jobs/pkg/jobs/
│   ├── github_workflow_scanner/
│   │   └── scanner.go                     # ✅ EphemeralRunner scanner job
│   ├── github_workflow_collector/
│   │   ├── collector.go                   # ✅ File collector job (with AI integration)
│   │   ├── ai_extractor.go                # ✅ AI extraction handler (Phase 3)
│   │   ├── pvc_reader.go                  # ✅ PVC file reader via node-exporter
│   │   └── temp_pod_manager.go            # ✅ Temp pod lifecycle management
│   ├── github_workflow_backfill/
│   │   └── backfill_job.go                # ✅ Backfill job (Phase 5)
│   └── interface.go                       # Job registration (✅ Updated)
│
Primus-Conductor/Lens/api/                 # AI Agent Implementation
├── crews/
│   └── github_metrics_extractor/
│       ├── __init__.py                    # ✅ Package init
│       ├── github_metrics_crew.py         # ✅ Metrics extraction crew (Phase 3)
│       └── agents/
│           ├── __init__.py                # ✅ Package init
│           └── agents.py                  # ✅ CrewAI agents (Phase 3)
├── schemas/
│   └── github_metrics.py                  # ✅ Request/Response schemas (Phase 3)
└── server/
    ├── main.py                            # ✅ Route registration
    └── routes/
        └── github_metrics.py              # ✅ API routes (Phase 3)
```

---

## Complete Workflow

### Real-time Collection Flow

```
1. Discover Available AutoscalingRunnerSets
   GET /api/autoscaling-runner-sets
   → Returns list of ARS in the cluster with their namespaces

2. Create Configuration (Bind to an AutoscalingRunnerSet)
   POST /api/github-workflow-configs
   {
     "name": "LLM Benchmark Metrics",
     "runner_set_namespace": "actions-runner",
     "runner_set_name": "benchmark-runners",
     "github_owner": "AMD-AGI",
     "github_repo": "Primus-SaFE",
     "workflow_filter": "benchmark.yml",
     "file_patterns": ["Bench/results/**/*.json", "Bench/results/**/*.csv"],
     "file_types": ["json", "csv"]
   }

3. Preview EphemeralRunners (Optional)
   GET /api/github-workflow-configs/:id/runners?status=completed
   → Returns list of completed EphemeralRunners under the bound ARS

4. EphemeralRunnerScanJob (Every 1 minute)
   - Queries EphemeralRunners with parent = bound ARS
   - Filters by workflow_filter/branch_filter if configured
   - Creates pending run records for new completed runners

5. GitHubWorkflowCollectorJob (Every 1 minute)
   - Picks up pending run records
   - Extracts GitHub context from EphemeralRunner annotations:
     - actions.github.com/run-id → github_run_id
     - actions.github.com/job-id → job_id
   - Gets artifacts or repo files via GitHub API
   - Calls AI Agent to extract metrics
   - Stores metric data

6. User Queries Metrics
   GET /api/github-workflow-metrics?config_id=1&metrics=throughput&group_by=model_name
```

### Historical Backfill Flow

```
1. User Triggers Backfill
   POST /api/github-workflow-configs/:id/backfill
   {
     "start_time": "2024-01-01T00:00:00Z",
     "end_time": "2024-06-30T23:59:59Z"
   }

2. System Queries Historical EphemeralRunners
   - Finds all completed EphemeralRunners under the bound ARS
   - Filters by time range
   - Applies workflow_filter/branch_filter
   - Excludes already-processed runners
   - Creates pending run records with trigger_source='backfill'

3. GitHubWorkflowCollectorJob Processes Backfill Records
   - Same collection logic as real-time
   - Processes in batches to avoid overload

4. User Monitors Progress
   GET /api/github-workflow-configs/:id/backfill/status
   {
     "total": 150,
     "processed": 75,
     "failed": 2,
     "status": "in_progress"
   }
```

### EphemeralRunner Labels/Annotations (GitHub Actions Runner Controller)

GitHub Actions Runner Controller automatically adds these labels/annotations to EphemeralRunner pods:

| Label/Annotation | Example | Description |
|------------------|---------|-------------|
| `actions.github.com/scale-set-name` | `benchmark-runners` | Name of the AutoscalingRunnerSet |
| `actions.github.com/scale-set-namespace` | `actions-runner` | Namespace of the ARS |
| `actions.github.com/runner-group-name` | `default` | Runner group |
| `actions.github.com/organization` | `AMD-AGI` | GitHub organization |
| `actions.github.com/repository` | `Primus-SaFE` | Repository name |
| `actions.github.com/run-id` | `12345678` | Workflow run ID |
| `actions.github.com/job-id` | `98765432` | Job ID |
| `actions.github.com/workflow-name` | `benchmark.yml` | Workflow filename |

These are automatically populated by the runner controller, so no manual configuration is needed.

---

## Key Design Decisions

| Decision Point | Choice | Reason |
|----------------|--------|--------|
| **Trigger Mechanism** | AutoscalingRunnerSet binding | Simple, leverages native ARS→EphemeralRunner hierarchy |
| **Workload Matching** | Parent-child relationship | No complex selectors, accurate matching via native K8s ownership |
| **Runner Discovery** | Scan EphemeralRunners | ARS manages runners, we just observe completions |
| **File Retrieval** | Temp Pod + PVC mount | EphemeralRunner pods are deleted after completion; create temp pod to mount same PVC |
| **File Access** | Node-exporter container FS API | Reuse existing infrastructure for reading container files |
| **Metric Storage Format** | JSONB (dimensions + metrics) | Flexible to adapt to different benchmark output formats |
| **Schema Generation** | AI auto-generate + user confirmation | Reduces user configuration burden while retaining customization |
| **Job Separation** | Scan + Collector separated | Single responsibility, easier to extend and debug |
| **Backfill Support** | On-demand via API | Allows processing historical data without rerunning workflows |
| **Data Aggregation** | Query-time aggregation | Start simple, add pre-aggregation tables later for optimization |

---

## Future Extensions

1. **Webhook Integration**: Support GitHub/GitLab webhooks as additional trigger source
2. **Multi-Repository Monitoring**: Single configuration supports monitoring multiple repositories
3. **Alert Rules**: Set alerts based on metric change trends
4. **Comparison Analysis**: Support cross-run performance comparison (diff between runs)
5. **Custom Parsers**: Support user-written custom file parsing logic
6. **Auto-Discovery**: Automatically suggest configurations based on workload patterns
7. **Metric Anomaly Detection**: AI-powered detection of performance regressions

---

## Development Plan

### Phase 1: Foundation (Week 1-2) ✅ COMPLETED

**Goal**: Set up database models, basic CRUD APIs, and core infrastructure

| Task | Description | Effort | Status |
|------|-------------|--------|--------|
| 1.1 Database Schema | Create migration files for all 4 tables | 0.5d | ✅ Done |
| 1.2 Model Generation | Run gen tool to generate GORM models | 0.5d | ✅ Done |
| 1.3 Facades | Implement database facades for all tables | 1d | ✅ Done |
| 1.4 Config CRUD API | Implement configuration management APIs | 1d | ✅ Done |
| 1.5 ARS Discovery API | Implement AutoscalingRunnerSet listing API | 0.5d | ✅ Done |
| ~~1.6 GitHub Client~~ | ~~Implement GitHub API client wrapper~~ | ~~1d~~ | ⏭️ Skipped (using PVC reader instead) |
| 1.7 Unit Tests | Write unit tests for facades and APIs | 1d | ⏳ Pending |

**Deliverables**:
- [x] Database tables created and migrated (patch043-github_workflow_metrics.sql)
- [x] Configuration CRUD APIs working (github_workflow_metrics.go)
- [x] Can list AutoscalingRunnerSets from existing workload data
- [x] ~~GitHub client~~ → PVC Reader via temp pod implemented

---

### Phase 2: Core Collection (Week 3-4) ✅ COMPLETED

**Goal**: Implement EphemeralRunner scanning and file collection

| Task | Description | Effort | Status |
|------|-------------|--------|--------|
| 2.1 EphemeralRunner Query | Add workload facade methods to query EphemeralRunners by parent | 0.5d | ✅ Done |
| 2.2 Runner Discovery API | Implement EphemeralRunner listing API | 0.5d | ✅ Done |
| 2.3 Scan Job | Implement EphemeralRunnerScanJob | 1d | ✅ Done |
| 2.4 Run Records API | Implement run records listing and detail APIs | 0.5d | ✅ Done |
| 2.5 File Pattern Matching | Implement glob pattern matching for file paths | 0.5d | ✅ Done |
| 2.6 Collector Job | Implement GitHubWorkflowCollectorJob (without AI) | 1.5d | ✅ Done |
| 2.7 File Parsers | Implement JSON/CSV/Markdown basic parsers | 1d | ✅ Done |
| 2.8 Integration Tests | Write integration tests for jobs | 1d | ⏳ Pending |

**Implementation Notes**:
- File collection uses **temporary pod** to mount PVC (EphemeralRunner pods are deleted after completion)
- Implemented: `scanner.go`, `collector.go`, `pvc_reader.go`, `temp_pod_manager.go`

**Deliverables**:
- [x] Scan job discovers new completed EphemeralRunners
- [x] Collector job reads files via temp pod mounting same PVC
- [x] Basic file parsing works (JSON, CSV, Markdown tables)
- [x] Run records are created and updated correctly

---

### Phase 3: AI Integration (Week 5-6) ✅ COMPLETED

**Goal**: Integrate AI Agent for smart field extraction and schema generation

| Task | Description | Effort | Status |
|------|-------------|--------|--------|
| 3.1 AI Topic Definition | Define `github.metrics.extract` topic in aitopics | 0.5d | ✅ Done |
| 3.2 AI Request/Response | Implement request/response types for extraction | 0.5d | ✅ Done |
| 3.3 Schema Management | Implement schema CRUD and versioning | 1d | ✅ Done |
| 3.4 AI Extractor | Integrate AI client into collector job | 1d | ✅ Done |
| 3.5 Schema Generation Flow | Implement first-time schema generation via AI | 1d | ✅ Done |
| 3.6 Schema Confirmation API | Implement schema review and confirmation APIs | 0.5d | ✅ Done |
| 3.7 AI Agent Handler | Implement extraction handler in Primus-Conductor | 2d | ✅ Done |
| 3.8 Fallback Logic | Implement graceful degradation when AI unavailable | 0.5d | ✅ Done |

**Deliverables**:
- [x] AI can analyze file content and generate schema
- [x] Users can review and adjust generated schema via APIs
- [x] Subsequent extractions use confirmed schema
- [x] System works (with reduced functionality) when AI is unavailable

---

### Phase 4: Metrics & Query (Week 7-8)

**Goal**: Implement metrics storage and query APIs

| Task | Description | Effort | Status |
|------|-------------|--------|--------|
| 4.1 Metrics Storage | Store extracted metrics to github_workflow_metrics | 0.5d | ✅ Done |
| 4.2 Metrics Query API | Implement basic metrics query API | 1d | ✅ Done |
| 4.3 Dimension Filtering | Implement JSONB dimension filtering | 0.5d | ✅ Done |
| 4.4 Time-series Aggregation | Implement time-based grouping and aggregation | 1d | ✅ Done |
| 4.5 Summary API | Implement metrics summary/statistics API | 0.5d | ✅ Done |
| 4.6 Trends API | Implement trends data API | 0.5d | ✅ Done |
| 4.7 Query Optimization | Add indexes and optimize query performance | 1d | ✅ Done |
| 4.8 API Documentation | Document all APIs with examples | 0.5d | ✅ Done |

**Deliverables**:
- [x] Metrics are stored with dimensions and values
- [x] Query API supports filtering, grouping, aggregation
- [x] Trends and summary data available
- [x] Query performance is acceptable for dashboard use

---

### Phase 5: Backfill & Polish (Week 9-10)

**Goal**: Implement backfill functionality and production readiness

| Task | Description | Effort | Status |
|------|-------------|--------|--------|
| 5.1 Backfill API | Implement backfill trigger API | 1d | ✅ Done |
| 5.2 Backfill Progress | Implement backfill status tracking | 0.5d | ✅ Done |
| 5.3 Batch Processing | Implement batch processing for backfill | 0.5d | ✅ Done |
| 5.4 Retry Logic | Implement retry for failed collections | 0.5d | ✅ Done |
| 5.5 Error Handling | Improve error messages and logging | 0.5d | ✅ Done |
| 5.6 Metrics & Monitoring | Add Prometheus metrics for jobs | 0.5d | ✅ Done |
| 5.7 E2E Tests | Write end-to-end tests | 1.5d | ⏳ Pending |
| 5.8 Performance Testing | Load testing and optimization | 1d | ⏳ Pending |
| 5.9 Documentation | Update README and deployment docs | 0.5d | ⏳ Pending |

**Deliverables**:
- [x] Users can backfill historical data
- [x] Failed collections can be retried
- [x] Comprehensive monitoring and alerting
- [ ] Production-ready with documentation

---

### Development Timeline Summary

```
Week 1-2:   ████████████████  Phase 1: Foundation       ✅ COMPLETED
Week 3-4:   ████████████████  Phase 2: Core Collection  ✅ COMPLETED
Week 5-6:   ████████████████  Phase 3: AI Integration   ✅ COMPLETED
Week 7-8:   ████████████████  Phase 4: Metrics & Query  ✅ COMPLETED
Week 9-10:  ████████████░░░░  Phase 5: Backfill & Polish ✅ MOSTLY COMPLETED
```

**Total Estimated Effort**: ~10 weeks (1 developer)

**Current Progress**: Phase 1-5 core functionality completed. Remaining: E2E tests, performance testing, documentation

---

### Milestones

| Milestone | Target | Success Criteria | Status |
|-----------|--------|------------------|--------|
| **M1: MVP** | End of Week 4 | Can collect files when EphemeralRunner completes | ✅ Done |
| **M2: AI Ready** | End of Week 6 | AI can extract metrics from benchmark files | ✅ Done |
| **M3: Query Ready** | End of Week 8 | Users can query metrics via API | ✅ Done |
| **M4: Production** | End of Week 10 | Feature is production-ready with backfill support | ✅ Done (Core) |

---

### Dependencies

| Dependency | Required By | Status |
|------------|-------------|--------|
| ~~GitHub Token with repo access~~ | ~~Phase 1~~ | ⏭️ Not needed (using PVC reader) |
| AI Gateway deployed | Phase 3 | ✅ Required for AI features |
| Primus-Conductor with extraction handler | Phase 3 | ✅ Implemented |
| Workload tracking includes EphemeralRunner | Phase 2 | ✅ Verified |
| Node-exporter DaemonSet | Phase 2 | ✅ Available |

---

### Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| ~~GitHub API rate limiting~~ | ~~Collection delays~~ | ~~Implement caching, batch requests~~ → Not applicable (using PVC) |
| PVC not available after runner deletion | Cannot read files | Check PVC retention policy, process runs promptly |
| AI extraction accuracy | Poor data quality | Allow manual schema definition, provide sample data for testing |
| Large file handling | Memory issues | File size limits (5MB max per file) |
| Historical data volume | Long backfill time | Batch processing, progress tracking, cancellation support |
| Temp pod scheduling delays | Slow collection | Use tolerations to ensure scheduling, timeout handling |

