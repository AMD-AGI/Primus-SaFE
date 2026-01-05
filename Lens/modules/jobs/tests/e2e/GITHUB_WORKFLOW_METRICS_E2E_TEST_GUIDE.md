# GitHub Workflow Metrics Collector E2E Testing Guide

This document provides a complete end-to-end testing guide for the GitHub Workflow Metrics Collector feature.

## Test Environment

- **Kubernetes Cluster**: test-env
- **Namespace**: primus-lens
- **Required Components**:
  - Lens API server
  - Lens Jobs service
  - PostgreSQL database
  - Node-exporter DaemonSet (for container FS access)
  - (Optional) AI Gateway + Primus-Conductor (for AI extraction)

```bash
export KUBECONFIG=/wekafs/haiskong/.kube/config
kubectl config use-context test-env
```

## Prerequisites

1. Kubernetes cluster access
2. An AutoscalingRunnerSet already deployed (for testing with real data)
3. Or use mock data for basic functionality testing

---

## Phase 1: Database Setup

### 1.1 Verify Database Tables Exist

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- \
  psql -d primus-lens -c "SELECT table_name FROM information_schema.tables WHERE table_name LIKE 'github_workflow%';"
```

Expected output:
```
        table_name
---------------------------
 github_workflow_configs
 github_workflow_runs
 github_workflow_metrics
 github_workflow_metric_schemas
(4 rows)
```

### 1.2 If Tables Don't Exist, Apply Migration

```bash
# Copy migration file to pod
kubectl cp modules/core/pkg/database/migrations/patch043-github_workflow_metrics.sql \
  primus-lens/primus-lens-lens-v6jg-0:/tmp/

# Execute migration
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- \
  psql -d primus-lens -f /tmp/patch043-github_workflow_metrics.sql
```

---

## Phase 2: API Testing

### 2.1 Start Port Forwarding to API Server

```bash
kubectl port-forward -n primus-lens svc/primus-lens-api 58080:8080 &
sleep 3
```

### 2.2 Test Configuration CRUD

#### Create a Configuration

```bash
curl -s -X POST http://localhost:58080/api/v1/github-workflow-metrics/configs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-benchmark-metrics",
    "description": "E2E test configuration",
    "runner_set_namespace": "actions-runner",
    "runner_set_name": "benchmark-runners",
    "github_owner": "AMD-AGI",
    "github_repo": "Primus-SaFE",
    "workflow_filter": "benchmark.yml",
    "file_patterns": ["results/**/*.json", "results/**/*.csv"],
    "enabled": true
  }' | jq .
```

Expected output:
```json
{
  "data": {
    "id": 1,
    "name": "test-benchmark-metrics",
    ...
  }
}
```

#### List Configurations

```bash
curl -s http://localhost:58080/api/v1/github-workflow-metrics/configs | jq .
```

#### Get Configuration Details

```bash
CONFIG_ID=1
curl -s http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID} | jq .
```

#### Update Configuration

```bash
curl -s -X PUT http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-benchmark-metrics-updated",
    "file_patterns": ["results/**/*.json", "results/**/*.csv", "bench/**/*.md"]
  }' | jq .
```

### 2.3 Test Schema Management

#### Create a Schema Manually

```bash
curl -s -X POST http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/schemas \
  -H "Content-Type: application/json" \
  -d '{
    "name": "llm-benchmark-schema",
    "fields": [
      {"name": "model_name", "type": "string", "description": "Model name"},
      {"name": "throughput", "type": "float", "unit": "tokens/s", "description": "Throughput"},
      {"name": "latency_p99", "type": "float", "unit": "ms", "description": "P99 latency"}
    ],
    "dimension_fields": ["model_name"],
    "metric_fields": ["throughput", "latency_p99"]
  }' | jq .
```

#### List Schemas

```bash
curl -s http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/schemas | jq .
```

#### Activate Schema

```bash
SCHEMA_ID=1
curl -s -X POST http://localhost:58080/api/v1/github-workflow-metrics/schemas/${SCHEMA_ID}/activate | jq .
```

---

## Phase 3: Mock EphemeralRunner Testing

Since EphemeralRunner pods are typically deleted after completion, we can simulate by:
1. Creating mock workload records
2. Creating mock run records
3. Testing the collection job manually

### 3.1 Insert Mock Workload Record

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
INSERT INTO gpu_workloads (uid, name, namespace, kind, status, created_at, end_at, labels, annotations, parent_uid)
VALUES (
  'test-runner-uid-001',
  'benchmark-runners-abc123',
  'actions-runner',
  'EphemeralRunner',
  'Completed',
  NOW() - INTERVAL '1 hour',
  NOW() - INTERVAL '30 minutes',
  '{\"actions.github.com/scale-set-name\": \"benchmark-runners\"}'::jsonb,
  '{\"actions.github.com/run-id\": \"12345678\", \"actions.github.com/job-id\": \"98765432\"}'::jsonb,
  ''
)
ON CONFLICT (uid) DO NOTHING;
"
```

### 3.2 Create a Run Record

```bash
curl -s -X POST http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/runs \
  -H "Content-Type: application/json" \
  -d '{
    "workload_uid": "test-runner-uid-001",
    "workload_name": "benchmark-runners-abc123",
    "workload_namespace": "actions-runner",
    "status": "pending",
    "trigger_source": "manual"
  }' | jq .
```

Or directly insert:
```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
INSERT INTO github_workflow_runs (
  config_id, workload_uid, workload_name, workload_namespace, 
  status, trigger_source, created_at, updated_at
)
VALUES (
  ${CONFIG_ID}, 'test-runner-uid-001', 'benchmark-runners-abc123', 'actions-runner',
  'pending', 'manual', NOW(), NOW()
);
"
```

### 3.3 List Runs

```bash
curl -s "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/runs" | jq .
```

---

## Phase 4: Backfill Testing

### 4.1 Trigger Backfill

```bash
START_TIME=$(date -u -d "7 days ago" +"%Y-%m-%dT%H:%M:%SZ")
END_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

curl -s -X POST "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/backfill" \
  -H "Content-Type: application/json" \
  -d "{
    \"start_time\": \"${START_TIME}\",
    \"end_time\": \"${END_TIME}\"
  }" | jq .
```

Expected output:
```json
{
  "data": {
    "message": "Backfill task created",
    "task_id": "backfill-1-1234567890",
    "config_id": 1,
    "status": "pending"
  }
}
```

### 4.2 Check Backfill Status

```bash
curl -s "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/backfill/status" | jq .
```

### 4.3 List All Backfill Tasks

```bash
curl -s "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/backfill/tasks" | jq .
```

### 4.4 Cancel Backfill (if needed)

```bash
curl -s -X POST "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/backfill/cancel" | jq .
```

---

## Phase 5: Metrics Query Testing

### 5.1 Insert Sample Metrics Data

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
INSERT INTO github_workflow_metrics (
  config_id, run_id, schema_id, timestamp, source_file, dimensions, metrics, created_at
)
VALUES 
  (${CONFIG_ID}, 1, 1, NOW() - INTERVAL '1 day', 'results/bench.json', 
   '{\"model_name\": \"llama-70b\", \"batch_size\": 32}'::jsonb,
   '{\"throughput\": 1234.5, \"latency_p99\": 10.2}'::jsonb, NOW()),
  (${CONFIG_ID}, 1, 1, NOW() - INTERVAL '1 day', 'results/bench.json',
   '{\"model_name\": \"mixtral-8x7b\", \"batch_size\": 32}'::jsonb,
   '{\"throughput\": 856.3, \"latency_p99\": 15.8}'::jsonb, NOW()),
  (${CONFIG_ID}, 1, 1, NOW() - INTERVAL '2 days', 'results/bench.json',
   '{\"model_name\": \"llama-70b\", \"batch_size\": 32}'::jsonb,
   '{\"throughput\": 1200.0, \"latency_p99\": 11.5}'::jsonb, NOW());
"
```

### 5.2 Basic Metrics Query

```bash
curl -s "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/metrics" | jq .
```

### 5.3 Advanced Query with Dimension Filtering

```bash
curl -s -X POST "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/metrics/query" \
  -H "Content-Type: application/json" \
  -d '{
    "dimensions": {"model_name": "llama-70b"},
    "limit": 10
  }' | jq .
```

### 5.4 Aggregation Query

```bash
curl -s -X POST "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/metrics/aggregate" \
  -H "Content-Type: application/json" \
  -d '{
    "metric_field": "throughput",
    "agg_func": "avg",
    "interval": "1d",
    "group_by": ["model_name"]
  }' | jq .
```

### 5.5 Get Metrics Summary

```bash
curl -s "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/summary" | jq .
```

### 5.6 Get Trends Data

```bash
curl -s -X POST "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/metrics/trends" \
  -H "Content-Type: application/json" \
  -d '{
    "metric_fields": ["throughput", "latency_p99"],
    "interval": "1d"
  }' | jq .
```

### 5.7 Get Available Dimensions

```bash
curl -s "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/dimensions" | jq .
```

### 5.8 Get Available Fields

```bash
curl -s "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/fields" | jq .
```

---

## Phase 6: Jobs Testing

### 6.1 Check Jobs Service Status

```bash
kubectl get pods -n primus-lens -l app=primus-lens-jobs
kubectl logs -n primus-lens -l app=primus-lens-jobs --tail=50 | grep -i "github"
```

### 6.2 Check Job Execution Metrics

```bash
kubectl port-forward -n primus-lens svc/primus-lens-jobs 58004:8004 &
sleep 2

# Check Prometheus metrics
curl -s http://localhost:58004/metrics | grep -E "github_workflow|GithubWorkflow"
```

Expected metrics:
```
primus_lens_jobs_execution_total{job_name="GithubWorkflowScannerJob"} 10
primus_lens_jobs_execution_total{job_name="GithubWorkflowCollectorJob"} 10
primus_lens_jobs_execution_total{job_name="GithubWorkflowBackfillJob"} 5
primus_lens_github_workflow_backfill_tasks_total 2
primus_lens_github_workflow_backfill_runs_created_total 15
```

### 6.3 Trigger Manual Job Execution (if supported)

Check if jobs are running by observing logs:
```bash
kubectl logs -n primus-lens -l app=primus-lens-jobs -f | grep -i "github"
```

---

## Phase 7: AI Integration Testing (Optional)

### 7.1 Verify AI Gateway is Available

```bash
kubectl port-forward -n primus-lens svc/ai-gateway 58003:8003 &
sleep 2

curl -s http://localhost:58003/v1/ai/stats | jq .
```

### 7.2 Test AI Schema Generation

```bash
curl -s -X POST "http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID}/schemas/regenerate" \
  -H "Content-Type: application/json" \
  -d '{
    "sample_files": [
      {
        "path": "results/benchmark.json",
        "name": "benchmark.json",
        "file_type": "json",
        "content": "[{\"model\": \"llama-70b\", \"throughput\": 1234.5, \"latency_p99\": 10.2}]"
      }
    ]
  }' | jq .
```

---

## Phase 8: Cleanup

### 8.1 Delete Test Data

```bash
kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
DELETE FROM github_workflow_metrics WHERE config_id = ${CONFIG_ID};
DELETE FROM github_workflow_runs WHERE config_id = ${CONFIG_ID};
DELETE FROM github_workflow_metric_schemas WHERE config_id = ${CONFIG_ID};
DELETE FROM github_workflow_configs WHERE id = ${CONFIG_ID};
DELETE FROM gpu_workloads WHERE uid = 'test-runner-uid-001';
"
```

### 8.2 Stop Port Forwarding

```bash
pkill -f "port-forward.*58080"
pkill -f "port-forward.*58003"
pkill -f "port-forward.*58004"
```

---

## Verification Checklist

| Feature | Verification Method | Expected Result |
|---------|---------------------|-----------------|
| Database tables | `\dt github_workflow*` | Shows 4 tables |
| Config CRUD | API calls | All operations return success |
| Schema management | API calls | Schema created and activated |
| Run records | API + DB query | Runs created with correct status |
| Backfill trigger | POST /backfill | Task created with ID |
| Backfill status | GET /backfill/status | Shows progress |
| Metrics query | GET /metrics | Returns metric data |
| Dimension filtering | POST /metrics/query | Filters correctly |
| Aggregation | POST /metrics/aggregate | Returns aggregated data |
| Trends | POST /metrics/trends | Returns time-series |
| Summary | GET /summary | Returns statistics |
| Job metrics | /metrics endpoint | Shows job execution counts |
| AI schema generation | POST /schemas/regenerate | Returns generated schema |

---

## Troubleshooting

### No Runs Being Created

1. Check if Scanner job is running:
   ```bash
   kubectl logs -n primus-lens -l app=primus-lens-jobs --tail=100 | grep Scanner
   ```

2. Verify config is enabled:
   ```bash
   curl -s http://localhost:58080/api/v1/github-workflow-metrics/configs/${CONFIG_ID} | jq .enabled
   ```

3. Check if matching workloads exist:
   ```bash
   kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
     SELECT COUNT(*) FROM gpu_workloads WHERE kind = 'EphemeralRunner' AND status = 'Completed';
   "
   ```

### Collector Job Failing

1. Check collector job logs:
   ```bash
   kubectl logs -n primus-lens -l app=primus-lens-jobs --tail=100 | grep Collector
   ```

2. Check if temp pod can be created:
   ```bash
   kubectl get pods -n actions-runner -l primus-lens.amd.com/temp-pvc-reader=true
   ```

3. Check node-exporter availability:
   ```bash
   kubectl get pods -n primus-lens -l app=node-exporter
   ```

### Metrics Query Returns Empty

1. Verify metrics exist in database:
   ```bash
   kubectl exec -n primus-lens primus-lens-lens-v6jg-0 -- psql -d primus-lens -c "
     SELECT COUNT(*) FROM github_workflow_metrics WHERE config_id = ${CONFIG_ID};
   "
   ```

2. Check time range in query matches data timestamps

---

## Related Files

- Design document: `Lens/docs/github-workflow-metrics-collector-design.md`
- Database migration: `Lens/modules/core/pkg/database/migrations/patch043-github_workflow_metrics.sql`
- API handlers: `Lens/modules/api/pkg/api/github_workflow_metrics.go`
- Scanner job: `Lens/modules/jobs/pkg/jobs/github_workflow_scanner/`
- Collector job: `Lens/modules/jobs/pkg/jobs/github_workflow_collector/`
- Backfill job: `Lens/modules/jobs/pkg/jobs/github_workflow_backfill/`

