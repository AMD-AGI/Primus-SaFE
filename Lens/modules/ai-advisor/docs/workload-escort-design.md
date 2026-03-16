# Workload Escort Design

## Problem

Training workloads can fail silently — hanging at the first step for 10+ minutes before NCCL timeout, SIGSEGV after N iterations, ALLTOALL deadlocks. The current system detects the workload's *existence* (GpuWorkloadJob) and *framework* (DetectionCoordinator), but not its *health during execution*.

## Current Gaps

| What exists | What's missing |
|-------------|---------------|
| GpuWorkloadJob (20s): sync K8s status to DB | No training progress monitoring |
| Log Alert Engine: regex-based rules | No stateful multi-signal correlation |
| WorkloadEvent: "StartTrain" event | No "StepCompleted", "Hanging", "DegradedPerformance" events |
| anomaly/ diagnostics/ insights/ | All TODO stubs |

## Design: Escort Process

### Core Idea

For each GPU workload, spawn an **escort task** that runs a state machine tracking the workload through its lifecycle phases. Each phase has specific health checks and timeout expectations.

### Phase Model

```
SCHEDULING -> INITIALIZING -> COMPILING -> TRAINING -> COMPLETING -> EXITED
                                  |              |
                                  v              v
                               HUNG           DEGRADED --> FAILING
```

### Phase Detection (from existing data sources)

| Phase | Detection Signal | Data Source |
|-------|-----------------|-------------|
| SCHEDULING | Pod exists, phase=Pending | gpu_pods table (5s poll) |
| INITIALIZING | Pod Running, no "training" log yet | OpenSearch node-* logs + gpu_pods |
| COMPILING | Triton warnings appearing, GPU util >0 but power low | OpenSearch logs + workload_gpu_utilization + workload_gpu_power_usage (VM) |
| TRAINING | "after N iterations" log appears, or loss values logged | OpenSearch logs |
| COMPLETING | "Training completed" or "Skip saving" log | OpenSearch logs |
| EXITED | Pod phase=Succeeded/Failed, or gpu_workload status=Done/Deleted | gpu_workload + gpu_pods |

### Health Checks Per Phase

```
SCHEDULING:
  - timeout: 10min (gang-scheduling can take time)
  - check: FailedScheduling events in k8s-event-* index
  - alert: "Workload stuck in scheduling for >10min"

INITIALIZING:
  - timeout: 5min
  - check: RCCL init logs, IPv6 DNS warnings, NCCL env vars
  - metrics: workload_rdma_stat_tx_rdma_ack_timeout (baseline comparison)
  - alert: "Init taking unusually long"

COMPILING:
  - timeout: 15min (triton compilation can be slow)
  - check: triton warnings count, GPU utilization should be >50%
  - metrics: workload_gpu_utilization, workload_gpu_power_usage
  - alert: "Compilation phase exceeded 15min"

TRAINING:
  - liveness: expect iteration log every T seconds (learned from iter 1-2 timing)
  - check: loss values (NaN/Inf detection), iteration time degradation
  - metrics: workload_gpu_utilization (should be >80%), XGMI throughput, RDMA errors
  - alert: "No iteration progress for 2x expected interval"
  - alert: "RDMA errors spiking during training"
  - alert: "GPU utilization dropped below 50%"

COMPLETING:
  - timeout: 5min after last iteration
  - check: checkpoint save logs
  - alert: "Completion phase taking too long"
```

### Data Collection Strategy

The escort process should NOT add new data collection — it queries existing data:

1. **OpenSearch** (node-YYYY.MM.DD): container logs, k8s events
2. **VictoriaMetrics** (workload_* metrics): GPU, RDMA, PCIe, XGMI, container stats
3. **PostgreSQL** (gpu_workload, gpu_pods): workload/pod state

### Implementation Options

#### Option A: New Executor in ai-advisor (recommended)

Add a `WorkloadEscortExecutor` to the existing TaskScheduler:

```
TaskCreator.ScanForRunningWorkloads (every 30s)
  -> Create escort task for new Running workloads
  -> WorkloadEscortExecutor picks up task
  -> Runs state machine until workload ends
  -> Produces WorkloadEvent records for each phase transition
  -> Triggers alerts on anomalies
```

Fits naturally into the existing `workload_task_state` + `TaskScheduler` pattern.

#### Option B: Standalone sidecar per workload

A lightweight container injected into each PyTorchJob that monitors from inside.
Pro: low latency, direct container access.
Con: invasive, requires webhook mutation.

#### Option C: Periodic job in lens-agent

A job in lens-agent that polls all running workloads every 30s.
Pro: simple, no new infra.
Con: doesn't scale well with many workloads.

### Recommended: Option A Implementation

#### New Task Type: `workload_escort`

```go
const TaskTypeWorkloadEscort = "workload_escort"
```

#### State Machine in Ext field

```json
{
  "phase": "TRAINING",
  "phase_entered_at": "2026-03-17T13:11:08Z",
  "last_iteration": 5,
  "last_iteration_at": "2026-03-17T13:12:30Z",
  "expected_iter_interval_s": 15.2,
  "rdma_ack_timeout_baseline": 68,
  "alerts_fired": ["compilation_slow"],
  "compilation_start": "2026-03-17T13:11:08Z",
  "training_start": "2026-03-17T13:11:45Z"
}
```

#### Queries Per Tick (30s cycle)

1. **Phase detection**: query latest 5 log lines from OpenSearch
2. **Metric snapshot**: query workload_gpu_utilization, workload_gpu_power_usage (instant)
3. **RDMA check**: query workload_rdma_stat_tx_rdma_ack_timeout increase (if in TRAINING phase)
4. **Liveness**: compare current iteration count vs last known

#### Alert Output

Write to `workload_event` table with new event types:

```
EscortPhaseTransition  - "Workload entered TRAINING phase"
EscortHangDetected     - "No iteration progress for 120s (expected 15s)"
EscortRdmaAnomaly      - "RDMA ack_timeout rate 3x above baseline"
EscortGpuDrop          - "GPU utilization dropped from 95% to 30%"
EscortNcclTimeout      - "NCCL Watchdog timeout detected in logs"
EscortSigsegv          - "SIGSEGV detected, workload crashed"
EscortSuccess          - "Training completed 50/50 iterations"
```

These events flow into the existing notification system (Log Alert Engine webhooks).

### MVP Scope

For the first version, implement just the **log-based phase detection + timeout alerting**:

1. Detect phase transitions from OpenSearch logs
2. Fire timeout alerts if phase exceeds expected duration
3. Detect NCCL errors and SIGSEGV from logs
4. Record all events in workload_event table

Metrics-based health checks (RDMA, GPU, XGMI) can be added in v2.
