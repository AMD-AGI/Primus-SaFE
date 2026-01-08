# GitHub Workflow Metrics API Supplement

This document lists the APIs that need to be implemented to support the frontend design in `primus-lens-web/docs/workflow-metrics-collector-frontend-design.md`.

## Summary

| API | Method | Path | Priority |
|-----|--------|------|----------|
| Partial Update Config | PATCH | `/configs/:id` | High |
| Get Active Schema | GET | `/configs/:id/schemas/active` | High |
| List All Runs | GET | `/runs` | High |
| Retry Single Run | POST | `/runs/:id/retry` | Medium |
| Get Dimension Values | GET | `/configs/:id/dimensions/:dimension/values` | Medium |
| Export Metrics CSV | GET | `/configs/:id/export` | Low |

All paths are relative to `/v1/github-workflow-metrics/`.

---

## 1. Partial Update Config (PATCH)

**Purpose**: Enable/disable config without sending full object. Used by frontend toggle switch.

### Request

```
PATCH /v1/github-workflow-metrics/configs/:id
Content-Type: application/json
```

**Path Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | int64 | Yes | Config ID |

**Query Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| cluster | string | No | Cluster name (defaults to current) |

**Request Body**:
```json
{
  "enabled": true
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| enabled | boolean | No | Enable/disable collection |
| name | string | No | Update config name |
| description | string | No | Update description |
| file_patterns | string[] | No | Update file patterns |

### Response

**Success (200)**:
```json
{
  "meta": { "code": 2000 },
  "data": {
    "id": 1,
    "name": "MI325 Benchmark",
    "enabled": true,
    "updated_at": "2025-01-01T00:00:00Z"
  }
}
```

### Implementation Notes

```go
// router.go
configsGroup.PATCH("/:id", PatchGithubWorkflowConfig)

// Handler
func PatchGithubWorkflowConfig(ctx *gin.Context) {
    // Parse ID
    // Bind partial update struct
    // Get existing config
    // Apply only non-nil fields
    // Save and return
}
```

---

## 2. Get Active Schema

**Purpose**: Get the currently active schema for a config. Used by config detail page.

### Request

```
GET /v1/github-workflow-metrics/configs/:id/schemas/active
```

**Path Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | int64 | Yes | Config ID |

**Query Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| cluster | string | No | Cluster name (defaults to current) |

### Response

**Success (200)**:
```json
{
  "meta": { "code": 2000 },
  "data": {
    "id": 5,
    "config_id": 1,
    "name": "MI325_Benchmark_Schema",
    "version": 2,
    "fields": [
      { "name": "Op", "type": "string", "description": "Operation type" },
      { "name": "throughput", "type": "float", "unit": "TFLOPS", "description": "Performance" }
    ],
    "dimension_fields": ["Op", "Backend:Stage"],
    "metric_fields": ["throughput"],
    "is_active": true,
    "generated_by": "ai",
    "created_at": "2025-01-01T00:00:00Z"
  }
}
```

**Not Found (404)** - No active schema:
```json
{
  "meta": { "code": 4040 },
  "message": "no active schema found for this config"
}
```

### Implementation Notes

```go
// router.go
configsGroup.GET("/:id/schemas/active", GetActiveGithubWorkflowSchema)

// Handler - use existing facade method
schema, err := schemaFacade.GetActiveByConfig(ctx, configID)
```

---

## 3. List All Runs (Global)

**Purpose**: List workflow runs across all configs. Used by Runs page with cross-config filtering.

### Request

```
GET /v1/github-workflow-metrics/runs
```

**Query Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| cluster | string | No | Cluster name |
| config_id | int64 | No | Filter by config ID |
| status | string | No | Filter by status: pending, collecting, extracting, completed, failed |
| trigger_source | string | No | Filter by trigger: realtime, backfill, manual |
| start_date | string | No | Filter runs after this date (RFC3339) |
| end_date | string | No | Filter runs before this date (RFC3339) |
| offset | int | No | Pagination offset (default: 0) |
| limit | int | No | Pagination limit (default: 20, max: 100) |

### Response

**Success (200)**:
```json
{
  "meta": { "code": 2000 },
  "data": {
    "runs": [
      {
        "id": 123,
        "config_id": 1,
        "config_name": "MI325 Benchmark",
        "workload_uid": "abc-123",
        "workload_name": "runner-h2j8s",
        "workload_namespace": "tw-project2-control-plane",
        "status": "completed",
        "trigger_source": "realtime",
        "files_found": 2,
        "files_processed": 2,
        "metrics_count": 68,
        "workload_started_at": "2025-01-01T10:00:00Z",
        "workload_completed_at": "2025-01-01T10:30:00Z",
        "collection_started_at": "2025-01-01T10:31:00Z",
        "collection_completed_at": "2025-01-01T10:35:00Z",
        "error_message": null,
        "retry_count": 0,
        "created_at": "2025-01-01T10:31:00Z"
      }
    ],
    "total": 150,
    "offset": 0,
    "limit": 20
  }
}
```

### Implementation Notes

```go
// router.go - Note: This must be defined BEFORE /:id to avoid routing conflict
runsGroup.GET("", ListAllGithubWorkflowRuns)

// Handler
func ListAllGithubWorkflowRuns(ctx *gin.Context) {
    filter := &database.GithubWorkflowRunFilter{
        ConfigID: 0, // 0 means all configs
        // ... other filters from query params
    }
    // May need to join with configs table to get config_name
}
```

**Facade Method Needed**:
```go
// Add to GithubWorkflowRunFacade
func (f *GithubWorkflowRunFacade) ListAll(ctx context.Context, filter *GithubWorkflowRunFilter) ([]*RunWithConfigName, int64, error)

type RunWithConfigName struct {
    *model.GithubWorkflowRuns
    ConfigName string `json:"config_name"`
}
```

---

## 4. Retry Single Run

**Purpose**: Reset a single failed run to pending status for re-processing.

### Request

```
POST /v1/github-workflow-metrics/runs/:id/retry
```

**Path Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | int64 | Yes | Run ID |

**Query Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| cluster | string | No | Cluster name |

### Response

**Success (200)**:
```json
{
  "meta": { "code": 2000 },
  "data": {
    "message": "Run reset to pending",
    "run_id": 123,
    "previous_status": "failed",
    "new_status": "pending"
  }
}
```

**Bad Request (400)** - Run not in failed status:
```json
{
  "meta": { "code": 4000 },
  "message": "only failed runs can be retried"
}
```

### Implementation Notes

```go
// router.go
runsGroup.POST("/:id/retry", RetryGithubWorkflowRun)

// Handler
func RetryGithubWorkflowRun(ctx *gin.Context) {
    run, _ := facade.GetByID(ctx, id)
    if run.Status != "failed" {
        // return error
    }
    facade.ResetToPending(ctx, id)
}
```

---

## 5. Get Single Dimension Values

**Purpose**: Get available values for a specific dimension. Used by dimension filter dropdowns.

### Request

```
GET /v1/github-workflow-metrics/configs/:id/dimensions/:dimension/values
```

**Path Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | int64 | Yes | Config ID |
| dimension | string | Yes | Dimension field name (e.g., "Op", "Backend:Stage") |

**Query Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| cluster | string | No | Cluster name |
| start | string | No | Filter by time range start (RFC3339) |
| end | string | No | Filter by time range end (RFC3339) |
| limit | int | No | Max values to return (default: 100) |

### Response

**Success (200)**:
```json
{
  "meta": { "code": 2000 },
  "data": {
    "dimension": "Op",
    "values": [
      "Attention",
      "Attention-FP8",
      "GEMM",
      "GEMM-FP8"
    ],
    "total": 4
  }
}
```

### Implementation Notes

```go
// router.go
configsGroup.GET("/:id/dimensions/:dimension/values", GetSingleDimensionValues)

// Handler - reuse existing facade method
values, err := facade.GetDistinctDimensionValues(ctx, configID, dimension, start, end)
```

---

## 6. Export Metrics CSV

**Purpose**: Export filtered metrics data as CSV file for download.

### Request

```
GET /v1/github-workflow-metrics/configs/:id/export
```

**Path Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| id | int64 | Yes | Config ID |

**Query Parameters**:
| Name | Type | Required | Description |
|------|------|----------|-------------|
| cluster | string | No | Cluster name |
| start | string | No | Start time (RFC3339) |
| end | string | No | End time (RFC3339) |
| dimensions | string | No | JSON-encoded dimension filters |
| metric_fields | string | No | Comma-separated metric fields to include |
| limit | int | No | Max rows (default: 10000) |

### Response

**Success (200)**:
```
Content-Type: text/csv
Content-Disposition: attachment; filename="metrics-config-1-20250101.csv"

Op,Backend:Stage,throughput,collected_at,source_file
Attention,Aiter/CK:Fwd,496.64,2025-01-01T10:30:00Z,/wekafs/.../summary.csv
Attention,Aiter/CK:Bwd,244.02,2025-01-01T10:30:00Z,/wekafs/.../summary.csv
GEMM,Hipblaslt:Fwd,725.06,2025-01-01T10:30:00Z,/wekafs/.../summary.csv
```

### Implementation Notes

```go
// router.go
configsGroup.GET("/:id/export", ExportGithubWorkflowMetrics)

// Handler
func ExportGithubWorkflowMetrics(ctx *gin.Context) {
    // Build query from params
    // Fetch metrics (with higher limit)
    // Get schema to know field order
    
    // Set headers
    ctx.Header("Content-Type", "text/csv")
    ctx.Header("Content-Disposition", fmt.Sprintf(
        "attachment; filename=\"metrics-config-%d-%s.csv\"",
        configID,
        time.Now().Format("20060102"),
    ))
    
    // Write CSV
    writer := csv.NewWriter(ctx.Writer)
    // Write header row
    // Write data rows
    writer.Flush()
}
```

---

## Frontend Design Adjustments

The following frontend API calls should use POST instead of GET (already implemented in backend):

| Frontend Design | Backend Implementation | Action |
|-----------------|----------------------|--------|
| `GET /query` | `POST /configs/:id/metrics/query` | Update frontend to use POST |
| `GET /aggregate` | `POST /configs/:id/metrics/aggregate` | Update frontend to use POST |
| `GET /configs/:id/trends` | `POST /configs/:id/metrics/trends` | Update frontend to use POST |

Update `services/workflow-metrics/metrics.ts`:

```typescript
// Query metrics with filters - use POST
export const queryMetrics = (configId: number, params: MetricsQuery) =>
  request.post(`/github-workflow-metrics/configs/${configId}/metrics/query`, params)

// Get aggregated metrics - use POST
export const getAggregatedMetrics = (configId: number, params: MetricsQuery) =>
  request.post(`/github-workflow-metrics/configs/${configId}/metrics/aggregate`, params)

// Get metrics trends - use POST
export const getMetricsTrends = (configId: number, params: TrendsParams) =>
  request.post(`/github-workflow-metrics/configs/${configId}/metrics/trends`, params)
```

---

## Implementation Priority

1. **High Priority** (Required for basic frontend functionality):
   - Get Active Schema
   - List All Runs
   - Partial Update Config (PATCH)

2. **Medium Priority** (Improves UX):
   - Retry Single Run
   - Get Single Dimension Values

3. **Low Priority** (Nice to have):
   - Export Metrics CSV

---

## File Locations

**Router**: `modules/api/pkg/api/router.go`
- Add new routes to `githubWorkflowMetricsGroup`

**Handlers**: `modules/api/pkg/api/github_workflow_metrics.go`
- Add new handler functions

**Facade** (if needed): `modules/core/pkg/database/github_workflow_run_facade.go`
- Add `ListAll` method with config name join

