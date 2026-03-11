# GitHub Workflow Metrics Collector Frontend Design

## Overview

This document describes the frontend design for the GitHub Workflow Metrics Collector feature in Primus Lens Web. The feature enables users to:

1. **Manage Collection Configs** - Create, edit, enable/disable metric collection configurations
2. **View Run Records** - Monitor workflow run collection status and history
3. **Explore Metrics** - Query, visualize, and analyze collected benchmark metrics

## Technology Stack

Based on existing project patterns:

- **Vue 3** with Composition API
- **Element Plus** for UI components
- **ECharts** for data visualization
- **Pinia** for state management
- **Vue Router** for navigation
- **Axios** for API calls
- **dayjs** for date formatting

---

## Navigation Structure

Add new menu section under existing navigation:

```
├── Statistics
├── Workloads
├── Weekly Reports
├── Workflow Metrics    ← NEW
│   ├── Configs
│   ├── Runs
│   └── Explorer
├── Management
└── Agent
```

### Route Configuration

```typescript
// router/index.ts additions
{
  path: '/workflow-metrics',
  name: 'WorkflowMetrics',
  component: () => import('@/pages/WorkflowMetrics/index.vue'),
  redirect: '/workflow-metrics/configs',
  children: [
    {
      path: 'configs',
      name: 'WorkflowMetricsConfigs',
      component: () => import('@/pages/WorkflowMetrics/Configs.vue'),
    },
    {
      path: 'configs/:id',
      name: 'WorkflowMetricsConfigDetail',
      component: () => import('@/pages/WorkflowMetrics/ConfigDetail.vue'),
    },
    {
      path: 'runs',
      name: 'WorkflowMetricsRuns',
      component: () => import('@/pages/WorkflowMetrics/Runs.vue'),
    },
    {
      path: 'runs/:id',
      name: 'WorkflowMetricsRunDetail',
      component: () => import('@/pages/WorkflowMetrics/RunDetail.vue'),
    },
    {
      path: 'explorer',
      name: 'WorkflowMetricsExplorer',
      component: () => import('@/pages/WorkflowMetrics/Explorer.vue'),
    },
  ]
}
```

---

## File Structure

```
src/pages/WorkflowMetrics/
├── index.vue                    # Tab navigation layout
├── Configs.vue                  # Config list page
├── ConfigDetail.vue             # Config detail & schema management
├── ConfigFormDialog.vue         # Create/Edit config dialog
├── Runs.vue                     # Run records list
├── RunDetail.vue                # Run detail with metrics preview
├── Explorer.vue                 # Metrics query & visualization
├── components/
│   ├── SchemaEditor.vue         # Schema field editor
│   ├── MetricsTable.vue         # Reusable metrics data table
│   ├── MetricsChart.vue         # Time-series chart component
│   ├── DimensionFilter.vue      # Dimension filter selector
│   └── TrendChart.vue           # Trends visualization
└── composables/
    └── useWorkflowMetrics.ts    # Shared state and utilities

src/services/workflow-metrics/
├── index.ts                     # API exports
├── configs.ts                   # Config CRUD APIs
├── runs.ts                      # Run records APIs
└── metrics.ts                   # Metrics query APIs
```

---

## Page Designs

### 1. Main Layout (`index.vue`)

Tab-based navigation following the Management page pattern.

```
┌─────────────────────────────────────────────────────────────────┐
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                        │
│  │ Configs  │ │  Runs    │ │ Explorer │                        │
│  └──────────┘ └──────────┘ └──────────┘                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│                     <router-view />                             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 2. Configs Page (`Configs.vue`)

List and manage collection configurations.

```
┌─────────────────────────────────────────────────────────────────┐
│ Workflow Metrics Configurations                    [+ New Config]│
├─────────────────────────────────────────────────────────────────┤
│ Filters:                                                        │
│ ┌──────────────┐ ┌──────────────┐ ┌────────┐ ┌────────┐        │
│ │ Name         │ │ Repository   │ │ Search │ │ Reset  │        │
│ └──────────────┘ └──────────────┘ └────────┘ └────────┘        │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Name           │ Repository    │ Namespace │ Status │ Actions│ │
│ ├─────────────────────────────────────────────────────────────┤ │
│ │ MI325 Bench    │ Primus-Turbo  │ tw-proj2  │ ✓ Active│ ⚙ ✏ 🗑│ │
│ │ LLM Inference  │ Primus-LLM    │ x-flannel │ ✗ Disabled│ ...│ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                    Page: 1 of 3 │
└─────────────────────────────────────────────────────────────────┘
```

#### Table Columns

| Column | Description |
|--------|-------------|
| Name | Config name (link to detail) |
| Repository | GitHub owner/repo |
| Runner Set | ARS name |
| Namespace | Kubernetes namespace |
| File Patterns | Glob patterns (truncated) |
| Schema | Schema status (confirmed/pending) |
| Runs | Total run count |
| Metrics | Total metrics count |
| Status | Enabled/Disabled toggle |
| Last Run | Last collection time |
| Actions | View, Edit, Delete |

#### Actions

- **New Config**: Opens dialog to create new config
- **Toggle Status**: Enable/disable collection
- **View Schema**: Jump to schema management
- **Edit**: Open edit dialog
- **Delete**: Confirm and delete

### 3. Config Form Dialog (`ConfigFormDialog.vue`)

Modal dialog for creating/editing configs.

```
┌─────────────────────────────────────────────────────────────────┐
│ Create New Configuration                                    ✕   │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Basic Information                                               │
│ ┌──────────────────────────────────────────────────────────┐   │
│ │ Name *              │ MI325 Benchmark Collection        │   │
│ ├──────────────────────────────────────────────────────────┤   │
│ │ Description         │ Collect metrics from MI325 runs   │   │
│ └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│ GitHub Repository                                               │
│ ┌──────────────────────────────────────────────────────────┐   │
│ │ Owner *             │ AMD-AGI                           │   │
│ ├──────────────────────────────────────────────────────────┤   │
│ │ Repository *        │ Primus-Turbo                      │   │
│ ├──────────────────────────────────────────────────────────┤   │
│ │ Workflow Filter     │ benchmark*.yml (optional)         │   │
│ ├──────────────────────────────────────────────────────────┤   │
│ │ Branch Filter       │ main (optional)                   │   │
│ └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Runner Configuration                                            │
│ ┌──────────────────────────────────────────────────────────┐   │
│ │ Cluster *           │ [tw-proj2 ▼]                      │   │
│ ├──────────────────────────────────────────────────────────┤   │
│ │ Namespace *         │ tw-project2-control-plane         │   │
│ ├──────────────────────────────────────────────────────────┤   │
│ │ Runner Set Name *   │ turbo-pt-bench-gfx942-2510-vsskr  │   │
│ └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│ File Patterns                                                   │
│ ┌──────────────────────────────────────────────────────────┐   │
│ │ Pattern 1   │ /wekafs/primus_turbo/**/summary.csv   [✕] │   │
│ │ Pattern 2   │ **/results.json                       [✕] │   │
│ │                                           [+ Add Pattern]│   │
│ └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│ ☑ Enable collection immediately                                │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                    [Cancel]  [Create Config]    │
└─────────────────────────────────────────────────────────────────┘
```

### 4. Config Detail Page (`ConfigDetail.vue`)

Detailed view with schema management.

```
┌─────────────────────────────────────────────────────────────────┐
│ ← Back to Configs                                               │
│                                                                 │
│ MI325 Benchmark Collection                         [Edit] [⚙]  │
│ Collect benchmark metrics from Primus-Turbo MI325 runs         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ ┌─────────────────────┐  ┌─────────────────────┐               │
│ │ Repository          │  │ Runner Set          │               │
│ │ AMD-AGI/Primus-Turbo│  │ turbo-pt-bench-...  │               │
│ └─────────────────────┘  └─────────────────────┘               │
│                                                                 │
│ ┌─────────────────────┐  ┌─────────────────────┐               │
│ │ Total Runs          │  │ Total Metrics       │               │
│ │      48             │  │     3,264           │               │
│ └─────────────────────┘  └─────────────────────┘               │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ Schema Management                          [Regenerate Schema]  │
├─────────────────────────────────────────────────────────────────┤
│ Schema: MI325_Benchmark_Schema v1         Status: ✓ Confirmed   │
│                                                                 │
│ Dimension Fields:                                               │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ Name          │ Type   │ Description                       ││
│ ├─────────────────────────────────────────────────────────────┤│
│ │ Op            │ string │ Operation type (Attention, GEMM)  ││
│ │ Backend:Stage │ string │ Backend and computation stage     ││
│ └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│ Metric Fields:                                                  │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ Name          │ Type   │ Unit   │ Description              ││
│ ├─────────────────────────────────────────────────────────────┤│
│ │ throughput    │ float  │ TFLOPS │ Performance measurement  ││
│ └─────────────────────────────────────────────────────────────┘│
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ Recent Runs                                    [View All Runs]  │
├─────────────────────────────────────────────────────────────────┤
│ (Mini table showing last 5 runs)                                │
└─────────────────────────────────────────────────────────────────┘
```

### 5. Runs Page (`Runs.vue`)

List workflow run collection records.

```
┌─────────────────────────────────────────────────────────────────┐
│ Workflow Run Records                                            │
├─────────────────────────────────────────────────────────────────┤
│ Filters:                                                        │
│ ┌────────────┐ ┌────────────┐ ┌──────────┐ ┌──────────────────┐│
│ │ Config ▼   │ │ Status ▼   │ │ Source ▼ │ │ Date Range      ││
│ └────────────┘ └────────────┘ └──────────┘ └──────────────────┘│
│                                             [Search] [Reset]    │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ID│ Config      │ Workload         │Files│Metrics│Status│Time│ │
│ ├─────────────────────────────────────────────────────────────┤ │
│ │1 │ MI325 Bench │ runner-h2j8s     │ 2  │  68   │ ✓    │ 5m │ │
│ │2 │ MI325 Bench │ runner-7f75n     │ 2  │  68   │ ✓    │ 5m │ │
│ │3 │ LLM Infer   │ runner-abc123    │ 1  │  45   │ ⚠    │ 2m │ │
│ │4 │ MI325 Bench │ runner-xyz789    │ 0  │   0   │ ✗    │ 1m │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                                                    Page: 1 of 10│
└─────────────────────────────────────────────────────────────────┘
```

#### Status Indicators

| Status | Icon | Color | Description |
|--------|------|-------|-------------|
| pending | ⏳ | gray | Waiting to be processed |
| collecting | 🔄 | blue | Reading files from PVC |
| extracting | 🤖 | purple | AI extracting metrics |
| completed | ✓ | green | Successfully collected |
| failed | ✗ | red | Collection failed |

### 6. Run Detail Page (`RunDetail.vue`)

Detailed view of a single run with extracted metrics.

```
┌─────────────────────────────────────────────────────────────────┐
│ ← Back to Runs                                                  │
│                                                                 │
│ Run #1: turbo-pt-bench-gfx942-2510-vsskr-gm5lg-runner-h2j8s    │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────┐        │
│ │ Status    │ │ Files     │ │ Metrics   │ │ Duration  │        │
│ │ ✓ Done    │ │  2 / 2    │ │    68     │ │   5m 23s  │        │
│ └───────────┘ └───────────┘ └───────────┘ └───────────┘        │
│                                                                 │
│ Timeline:                                                       │
│ ●──────────●──────────●──────────●                             │
│ Started   Collecting Extracting  Completed                      │
│ 09:30:00  09:30:05   09:30:45   09:35:23                        │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ Files Processed                                                 │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ Path                                    │ Records │ Status  ││
│ ├─────────────────────────────────────────────────────────────┤│
│ │ /wekafs/.../20251230/MI325/summary.csv  │   34    │ ✓       ││
│ │ /wekafs/.../20251230/MI325/summary.csv  │   34    │ ✓       ││
│ └─────────────────────────────────────────────────────────────┘│
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ Extracted Metrics Preview                    [View in Explorer] │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ Op              │ Backend:Stage    │ Throughput (TFLOPS)    ││
│ ├─────────────────────────────────────────────────────────────┤│
│ │ Attention       │ Aiter/CK:Fwd     │ 496.64                 ││
│ │ Attention       │ Aiter/CK:Bwd     │ 244.02                 ││
│ │ GEMM            │ Hipblaslt:Fwd    │ 725.06                 ││
│ │ ...             │ ...              │ ...                    ││
│ └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

### 7. Metrics Explorer (`Explorer.vue`)

Interactive query and visualization interface.

```
┌─────────────────────────────────────────────────────────────────┐
│ Metrics Explorer                                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Query Builder                                                   │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ Config:    [MI325 Benchmark ▼]                              ││
│ │ Time Range: [2025-12-01] to [2025-12-31]                    ││
│ │                                                              ││
│ │ Dimensions:                                                  ││
│ │ ┌──────────────┐ ┌────────────────────────────────────────┐ ││
│ │ │ Op         ▼ │ │ ☑ Attention ☑ GEMM ☐ Attention-FP8    │ ││
│ │ └──────────────┘ └────────────────────────────────────────┘ ││
│ │ ┌──────────────┐ ┌────────────────────────────────────────┐ ││
│ │ │ Backend    ▼ │ │ ☑ All                                  │ ││
│ │ └──────────────┘ └────────────────────────────────────────┘ ││
│ │                                                              ││
│ │ Metrics: ☑ throughput ☐ latency ☐ memory_usage              ││
│ │                                                              ││
│ │ Group By: [Op ▼]  Aggregation: [avg ▼]                      ││
│ └─────────────────────────────────────────────────────────────┘│
│                                              [Clear] [Query]    │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ Results                                                         │
├─────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────────────┐│
│ │                   Throughput by Operation                   ││
│ │   ┌────────────────────────────────────────────────────┐   ││
│ │ T │    ████████████████████  725 TFLOPS (GEMM)         │   ││
│ │ F │    ██████████████  497 TFLOPS (Attention)          │   ││
│ │ L │    ████████  201 TFLOPS (Attention-FP8)            │   ││
│ │ O │                                                     │   ││
│ │ P ├────────────────────────────────────────────────────┤   ││
│ │ S │  GEMM       Attention      Attention-FP8           │   ││
│ │   └────────────────────────────────────────────────────┘   ││
│ └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│ ┌───────────┐ ┌───────────┐                                    │
│ │ Bar Chart │ │ Line Chart│ │ Table │                          │
│ └───────────┘ └───────────┘                                    │
│                                                                 │
│ Data Table:                                                     │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │ Op          │ Avg Throughput │ Max  │ Min  │ Count          ││
│ ├─────────────────────────────────────────────────────────────┤│
│ │ GEMM        │     725.06     │ 1103 │ 157  │   48           ││
│ │ Attention   │     370.33     │ 497  │ 244  │   24           ││
│ └─────────────────────────────────────────────────────────────┘│
│                                               [Export CSV]      │
└─────────────────────────────────────────────────────────────────┘
```

#### Trends View

```
┌─────────────────────────────────────────────────────────────────┐
│ Trends                                                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Time Interval: [Daily ▼]   Metric: [throughput ▼]              │
│                                                                 │
│ ┌─────────────────────────────────────────────────────────────┐│
│ │                   Throughput Trend                          ││
│ │   1000 ┤                                                    ││
│ │    800 ┤    ●────●────●────●                               ││
│ │    600 ┤   ╱                 ╲                              ││
│ │    400 ┤  ●                   ●────●                        ││
│ │    200 ┤                                                    ││
│ │      0 ┼────┬────┬────┬────┬────┬────┬────                 ││
│ │        12/25 12/26 12/27 12/28 12/29 12/30 12/31            ││
│ └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│ Legend: ● GEMM  ■ Attention  ▲ Attention-FP8                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## API Service Layer

### `services/workflow-metrics/configs.ts`

```typescript
import request from '../request'

export interface WorkflowConfig {
  id: number
  name: string
  description?: string
  runnerSetNamespace: string
  runnerSetName: string
  runnerSetUid?: string
  githubOwner: string
  githubRepo: string
  workflowFilter?: string
  branchFilter?: string
  filePatterns: string[]
  metricSchemaId?: number
  enabled: boolean
  clusterName: string
  lastCheckedAt?: string
  createdAt: string
  updatedAt: string
}

export interface CreateConfigRequest {
  name: string
  description?: string
  runnerSetNamespace: string
  runnerSetName: string
  githubOwner: string
  githubRepo: string
  workflowFilter?: string
  branchFilter?: string
  filePatterns: string[]
  enabled?: boolean
  clusterName: string
}

// List configs with pagination
export const getConfigs = (params: {
  page?: number
  pageSize?: number
  name?: string
  enabled?: boolean
}) => request.get('/github-workflow-metrics/configs', { params })

// Get single config
export const getConfig = (id: number) => 
  request.get(`/github-workflow-metrics/configs/${id}`)

// Create config
export const createConfig = (data: CreateConfigRequest) =>
  request.post('/github-workflow-metrics/configs', data)

// Update config
export const updateConfig = (id: number, data: Partial<CreateConfigRequest>) =>
  request.put(`/github-workflow-metrics/configs/${id}`, data)

// Delete config
export const deleteConfig = (id: number) =>
  request.delete(`/github-workflow-metrics/configs/${id}`)

// Toggle enabled status
export const toggleConfigEnabled = (id: number, enabled: boolean) =>
  request.patch(`/github-workflow-metrics/configs/${id}`, { enabled })

// Get config schema
export const getConfigSchema = (configId: number) =>
  request.get(`/github-workflow-metrics/configs/${configId}/schemas/active`)

// Regenerate schema
export const regenerateSchema = (configId: number) =>
  request.post(`/github-workflow-metrics/configs/${configId}/schemas/regenerate`)

// Trigger backfill
export const triggerBackfill = (configId: number, params: {
  startDate?: string
  endDate?: string
}) => request.post(`/github-workflow-metrics/configs/${configId}/backfill`, params)
```

### `services/workflow-metrics/runs.ts`

```typescript
import request from '../request'

export interface WorkflowRun {
  id: number
  configId: number
  configName?: string
  workloadUid: string
  workloadName: string
  workloadNamespace: string
  githubRunId?: number
  githubRunNumber?: number
  headSha?: string
  headBranch?: string
  workflowName?: string
  status: 'pending' | 'collecting' | 'extracting' | 'completed' | 'failed'
  triggerSource: 'realtime' | 'backfill' | 'manual'
  filesFound: number
  filesProcessed: number
  metricsCount: number
  workloadStartedAt?: string
  workloadCompletedAt?: string
  collectionStartedAt?: string
  collectionCompletedAt?: string
  errorMessage?: string
  retryCount: number
  createdAt: string
  updatedAt: string
}

// List runs with pagination and filters
export const getRuns = (params: {
  page?: number
  pageSize?: number
  configId?: number
  status?: string
  triggerSource?: string
  startDate?: string
  endDate?: string
}) => request.get('/github-workflow-metrics/runs', { params })

// Get single run
export const getRun = (id: number) =>
  request.get(`/github-workflow-metrics/runs/${id}`)

// Get run metrics
export const getRunMetrics = (runId: number, params?: {
  page?: number
  pageSize?: number
}) => request.get(`/github-workflow-metrics/runs/${runId}/metrics`, { params })

// Retry failed run
export const retryRun = (id: number) =>
  request.post(`/github-workflow-metrics/runs/${id}/retry`)
```

### `services/workflow-metrics/metrics.ts`

```typescript
import request from '../request'

export interface MetricRecord {
  id: number
  runId: number
  schemaId: number
  dimensions: Record<string, any>
  metrics: Record<string, number>
  rawData?: Record<string, any>
  sourceFile: string
  collectedAt: string
}

export interface MetricsQuery {
  configId: number
  startTime?: string
  endTime?: string
  dimensions?: Record<string, string[]>
  metricFields?: string[]
  groupBy?: string[]
  aggregation?: 'avg' | 'sum' | 'min' | 'max' | 'count'
  page?: number
  pageSize?: number
}

export interface AggregatedResult {
  groupValue: string
  avg?: number
  sum?: number
  min?: number
  max?: number
  count: number
}

export interface TrendsResult {
  metric: string
  series: {
    name: string
    data: { timestamp: string; value: number }[]
  }[]
}

// Query metrics with filters
export const queryMetrics = (params: MetricsQuery) =>
  request.get('/github-workflow-metrics/query', { params })

// Get aggregated metrics
export const getAggregatedMetrics = (params: MetricsQuery) =>
  request.get('/github-workflow-metrics/aggregate', { params })

// Get metrics summary statistics
export const getMetricsSummary = (configId: number, params?: {
  startTime?: string
  endTime?: string
}) => request.get(`/github-workflow-metrics/configs/${configId}/summary`, { params })

// Get metrics trends
export const getMetricsTrends = (configId: number, params: {
  metric: string
  interval: 'hour' | 'day' | 'week'
  startTime?: string
  endTime?: string
  groupBy?: string
}) => request.get(`/github-workflow-metrics/configs/${configId}/trends`, { params })

// Get available dimension values
export const getDimensionValues = (configId: number, dimension: string) =>
  request.get(`/github-workflow-metrics/configs/${configId}/dimensions/${dimension}/values`)

// Get available metric fields
export const getMetricFields = (configId: number) =>
  request.get(`/github-workflow-metrics/configs/${configId}/metric-fields`)

// Export metrics to CSV
export const exportMetrics = (configId: number, params: MetricsQuery) => {
  const queryString = new URLSearchParams(params as any).toString()
  window.open(`/v1/github-workflow-metrics/configs/${configId}/export?${queryString}`)
}
```

---

## Component Specifications

### `DimensionFilter.vue`

Multi-select filter for dimension values.

```vue
<template>
  <div class="dimension-filter">
    <el-select
      v-model="selectedDimension"
      placeholder="Select Dimension"
      @change="onDimensionChange"
    >
      <el-option
        v-for="dim in availableDimensions"
        :key="dim"
        :label="dim"
        :value="dim"
      />
    </el-select>
    
    <el-select
      v-if="selectedDimension"
      v-model="selectedValues"
      multiple
      collapse-tags
      collapse-tags-tooltip
      placeholder="Select Values"
      @change="onValuesChange"
    >
      <el-option
        v-for="val in dimensionValues"
        :key="val"
        :label="val"
        :value="val"
      />
    </el-select>
  </div>
</template>
```

### `MetricsChart.vue`

ECharts wrapper for metrics visualization.

Props:
- `type`: 'bar' | 'line' | 'pie'
- `data`: Chart data
- `title`: Chart title
- `xAxis`: X-axis configuration
- `yAxis`: Y-axis configuration
- `loading`: Loading state

### `TrendChart.vue`

Time-series trend visualization.

Props:
- `series`: Array of series data
- `interval`: 'hour' | 'day' | 'week'
- `metric`: Metric name
- `loading`: Loading state

---

## State Management

### `stores/workflowMetrics.ts`

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { WorkflowConfig, WorkflowRun } from '@/services/workflow-metrics'

export const useWorkflowMetricsStore = defineStore('workflowMetrics', () => {
  // Current selected config
  const currentConfig = ref<WorkflowConfig | null>(null)
  
  // Query filters (persisted)
  const queryFilters = ref({
    configId: null as number | null,
    dimensions: {} as Record<string, string[]>,
    metrics: [] as string[],
    timeRange: [] as string[],
  })
  
  // Recent queries (for quick access)
  const recentQueries = ref<any[]>([])
  
  // Actions
  const setCurrentConfig = (config: WorkflowConfig) => {
    currentConfig.value = config
    queryFilters.value.configId = config.id
  }
  
  const saveQuery = (query: any) => {
    recentQueries.value.unshift({
      ...query,
      timestamp: new Date().toISOString()
    })
    // Keep last 10 queries
    recentQueries.value = recentQueries.value.slice(0, 10)
  }
  
  return {
    currentConfig,
    queryFilters,
    recentQueries,
    setCurrentConfig,
    saveQuery,
  }
}, {
  persist: {
    paths: ['queryFilters', 'recentQueries']
  }
})
```

---

## Responsive Design

Following existing patterns, all pages should support:

- **Desktop (1920px+)**: Full layout with all columns
- **Desktop (1280-1920px)**: Standard layout
- **Tablet (768-1280px)**: Horizontal scroll for tables
- **Mobile (<768px)**: Stacked filters, essential columns only

---

## Implementation Priority

### Phase 1: Core CRUD (Week 1)
1. Config list and create/edit dialog
2. Basic runs list
3. API service layer

### Phase 2: Detail Views (Week 2)
1. Config detail with schema display
2. Run detail with metrics preview
3. Status indicators and actions

### Phase 3: Metrics Explorer (Week 3)
1. Query builder interface
2. Bar/table visualization
3. Dimension filters

### Phase 4: Advanced Features (Week 4)
1. Trends visualization
2. Export functionality
3. Backfill trigger UI
4. Schema editor

---

## Testing Checklist

- [ ] Config CRUD operations
- [ ] Run list filtering and pagination
- [ ] Metrics query with dimension filters
- [ ] Chart rendering with real data
- [ ] Export functionality
- [ ] Responsive layout on all breakpoints
- [ ] Error handling and loading states
- [ ] Cluster selector integration

