# Workload Diagnosis Engine Design

## Overview

A real-time workload health monitoring and diagnosis system built on top of existing Lens data infrastructure. Combines performance liveness, device health, network metrics, system logs, and K8s state into a unified detection and diagnosis pipeline.

## 1. Signal Inventory — What Lens Already Has

### 1.1 Training Progress (DB: `training_performance`)

| Signal | Source | Frequency | Detection Use |
|--------|--------|-----------|---------------|
| Iteration count + timestamp | telemetry-processor log parsing | per-iteration | Hang detection (no new iteration for >3x expected interval) |
| `elapsed_time_per_iteration_ms` | log regex extraction | per-iteration | Performance degradation (>2x baseline) |
| `lm_loss`, `total_loss` | log regex extraction | per-iteration | Loss divergence (NaN, Inf, sudden spike) |
| `grad_norm` | log regex extraction | per-iteration | Gradient explosion/vanishing |
| `mem_usages`, `mem_allocated` | log regex extraction | per-iteration | Memory leak trending |

**Gap**: No iteration-level liveness check exists today. telemetry-processor writes data but nobody monitors the cadence.

### 1.2 GPU Device Metrics (VictoriaMetrics: `workload_gpu_*`)

| Signal | Metric | Detection Use |
|--------|--------|---------------|
| Compute activity | `workload_gpu_utilization`, `workload_gpu_gfx_activity` | Low util = hang or idle; 100% but no iter = compilation |
| Power draw | `workload_gpu_power_usage`, `workload_gpu_socket_power_watts` | Low power + high util = compilation; high power + progress = normal |
| Temperature | `workload_gpu_junction_temperature`, `workload_gpu_memory_temperature` | Thermal throttling detection |
| VRAM | `workload_gpu_used_vram`, `workload_gpu_free_vram` | OOM prediction (trending towards limit) |
| ECC errors | `workload_gpu_ecc_uncorrect_total`, `workload_gpu_ecc_uncorrect_umc`, `_xgmi_wafl` | Hardware failure detection |
| XGMI links | `workload_gpu_xgmi_link_tx`, `workload_gpu_xgmi_link_rx` | Intra-node P2P health, EP all-to-all monitoring |
| PCIe | `workload_pcie_replay_count`, `workload_pcie_nack_received_count`, `workload_pcie_recovery_count` | PCIe link degradation |

**Gap**: No anomaly detection on these metrics. Data exists but is only used for dashboards.

### 1.3 RDMA Network Metrics (VictoriaMetrics: `workload_rdma_stat_*`)

| Signal | Metric | Detection Use |
|--------|--------|---------------|
| ACK timeout | `workload_rdma_stat_tx_rdma_ack_timeout` | AINIC init issues, remote peer unreachable |
| Retransmission | `workload_rdma_stat_tx_rdma_retx_pkts/bytes` | Network-level packet loss |
| Sequence errors | `workload_rdma_stat_out_of_sequence`, `packet_seq_err` | Fabric routing issues |
| CQE errors | `workload_rdma_stat_req_cqe_error`, `resp_cqe_error` | HCA firmware/driver bug |
| Retry exhausted | `workload_rdma_stat_req_tx_retry_excd_err` | Persistent network failure |
| RNR | `workload_rdma_stat_req_rx_rnr_retry_err` | Receiver buffer exhaustion |
| Congestion | `workload_rdma_stat_np_cnp_sent`, `rp_cnp_handled` | Network congestion |
| ICRC | `workload_rdma_stat_rx_icrc_encapsulated` | Cable/transceiver issue |
| Traffic volume | `workload_rdma_stat_tx_rdma_ucast_bytes/pkts` | Is RDMA traffic flowing vs. dead |

**Gap**: Same — data exists, no automated anomaly detection or baseline comparison.

### 1.4 System Logs (OpenSearch: `node-*`)

| Signal | Detection Method | Detection Use |
|--------|-----------------|---------------|
| NCCL timeout | `Watchdog caught collective operation timeout` | Collective communication hang |
| ALLTOALL hang | `ALLTOALL_BASE` + `EXPERT_MODEL_PARALLEL_GROUP` | MoE expert parallel deadlock |
| SIGSEGV | `Signal 11` | Memory corruption, RCCL/driver bug |
| OOM | `CUDA out of memory`, `OutOfMemoryError` | GPU memory exhaustion |
| Python traceback | `Traceback (most recent call last)` | Application-level errors |
| Training completed | `Training completed` | Normal completion |
| Process exit | `exitcode : -N` | Abnormal exit with signal |

**Already monitored**: Log Alert Engine has 12 built-in rule templates covering most of these. 
**Gap**: Rules fire independently — no correlation with metrics or timeline context.

### 1.5 K8s Events (OpenSearch: `k8s-event-*`)

| Signal | Event Reason | Detection Use |
|--------|-------------|---------------|
| Scheduling failure | `FailedScheduling` | Gang-scheduling can't find nodes |
| Preemption | `Preempted` | Higher-priority workload took resources |
| Node crash | `NodeNotReady` | Hardware failure, kubelet crash |
| Eviction | `Evicted` | Resource pressure |
| Image pull failure | `Failed` + `ImagePullBackOff` | Registry or image issue |
| OOM Kill | `OOMKilling` | Kernel OOM killer |

**Already monitored**: investigate-workload-k8s-failure skill covers these queries.

### 1.6 Process & Environment (node-exporter API)

| Signal | Endpoint | Detection Use |
|--------|----------|---------------|
| Process tree | `/v1/processTree` | Verify training processes are alive, detect zombies |
| GPU binding | process tree GPU allocation | Verify correct GPU-process mapping |
| Python packages | `pip freeze` via code snapshot | Dependency version verification |
| Environment vars | process env extraction | NCCL/RCCL config verification |

### 1.7 Image Analysis (DB: `image_registry_cache`, `image_layer_cache`)

| Signal | Field | Detection Use |
|--------|-------|---------------|
| Package versions | `installed_packages` | Driver/library compatibility check |
| Framework hints | `framework_hints` | Verify expected framework is present |
| Base image | `base_image` | Detect image provenance |
| Image env | `image_env` | Built-in configuration |

---

## 2. Diagnosis Dimensions

Borrowing from agentic-rc's failure taxonomy, organize detection into dimensions:

### Dimension 1: Training Progress (Liveness)

```
Data: training_performance table
Check: Is the iteration count advancing?

Phase detection:
  - No data yet → INITIALIZING (check logs for startup progress)
  - First iteration appeared → WARMING_UP (triton compilation, first forward pass)
  - Regular iterations → TRAINING (monitor interval consistency)
  - No new iteration for >3x avg interval → POTENTIALLY_HUNG
  - Training completed log → COMPLETING

Anomaly detection:
  - Iteration interval spike (>2x baseline) → PERFORMANCE_DEGRADATION
  - Loss = NaN/Inf → CONVERGENCE_FAILURE
  - grad_norm spike (>10x baseline) → GRADIENT_EXPLOSION
  - mem_allocated trending up without plateau → MEMORY_LEAK
```

### Dimension 2: GPU Device Health

```
Data: workload_gpu_* metrics from VictoriaMetrics
Check: Are GPUs healthy and performing?

Anomaly detection:
  - GPU util drops from >80% to <30% mid-training → GPU_STALL or COMMUNICATION_WAIT
  - Power draw drops to idle while util stays high → COMPILATION_PHASE (benign)
  - ECC uncorrectable errors > 0 → GPU_HARDWARE_FAULT
  - XGMI link throughput drops to 0 during training → XGMI_LINK_FAILURE
  - PCIe replay/NACK count increasing → PCIE_LINK_DEGRADATION
  - Temperature exceeding threshold → THERMAL_THROTTLING
  - VRAM approaching total → OOM_RISK

Cross-GPU comparison:
  - One GPU's util/power significantly different from peers → SINGLE_GPU_ISSUE
  - All GPUs on one node different from other nodes → NODE_LEVEL_ISSUE
```

### Dimension 3: Network/RDMA Health

```
Data: workload_rdma_stat_* metrics from VictoriaMetrics
Check: Is inter-node communication healthy?

Anomaly detection:
  - tx_rdma_ack_timeout rate vs. init-phase baseline
    → Spike during training = RDMA_TIMEOUT_ANOMALY
  - retx_pkts increasing → PACKET_RETRANSMISSION
  - req_tx_retry_excd_err > 0 → RDMA_RETRY_EXHAUSTED (critical)
  - rx_icrc_encapsulated > 0 → CABLE_OR_SWITCH_ISSUE
  - RDMA ucast bytes = 0 during expected inter-node comm → RDMA_TRAFFIC_DEAD
  
Baseline comparison:
  - Compare workload's RDMA errors against cluster median
  - Compare across nodes in the same workload (one node worse = node issue)
```

### Dimension 4: System Logs (Error Detection)

```
Data: OpenSearch node-* + Log Alert Engine
Check: Are there error signals in the logs?

Detection (via existing Log Alert Engine + new rules):
  - NCCL timeout pattern → NCCL_COLLECTIVE_TIMEOUT
  - SIGSEGV / Signal 11 → SEGFAULT
  - OOM pattern → GPU_OOM or HOST_OOM
  - Python traceback → APPLICATION_ERROR
  - Connection reset / broken pipe → NETWORK_PARTITION
  
New rules to add:
  - "Primus-Turbo FP8 delayed not work" → FP8_TURBO_INCOMPATIBILITY
  - "tl.where with a non-boolean" count > 1000 → EXCESSIVE_TRITON_WARNINGS
  - "nicctl show qos failed" → AINIC_QOS_FAILURE
  - "IPv6 network addresses cannot be retrieved" → DNS_RESOLUTION_WARNING
```

### Dimension 5: K8s Infrastructure

```
Data: k8s-event-* OpenSearch + gpu_workload/gpu_pods DB
Check: Is the K8s layer healthy?

Detection:
  - Pod stuck in Pending > 5min → SCHEDULING_FAILURE
  - Pod restarted → POD_RESTART (check reason)
  - Node condition not Ready → NODE_FAILURE
  - Preemption event → PREEMPTED
  - Pod exitCode != 0 and not "Training completed" → ABNORMAL_EXIT
```

### Dimension 6: Configuration & Environment

```
Data: workload spec (K8s), image analysis (DB), process env (node-exporter)
Check: Is the configuration consistent and correct?

Detection:
  - NCCL env vars inconsistent across pods → ENV_MISMATCH
  - Image version mismatch between pods → IMAGE_MISMATCH
  - RCCL/libionic/AINIC FW version unexpected → DRIVER_VERSION_MISMATCH
  - Training parameters that are known-bad combos (e.g., legacy_gg=False + FP8 on AINIC) → KNOWN_BAD_CONFIG
```

---

## 3. Correlation Engine

The key value of a diagnosis engine is **correlating signals across dimensions**:

### Time-based correlation

```
When: NCCL_COLLECTIVE_TIMEOUT detected in logs at T
Then:
  1. Check training_performance: was iteration progress stalled before T?
  2. Check GPU metrics at T-10min..T: util pattern (compilation vs hang)
  3. Check RDMA metrics at T-10min..T: any error spike?
  4. Check K8s events around T: any node/pod issues?
  
→ Build a timeline: what happened first?
```

### Space-based correlation (node/GPU)

```
When: One node's GPU utilization differs from others
Then:
  1. Check ECC errors on that node's GPUs
  2. Check RDMA errors on that node's AINIC
  3. Check K8s events on that node
  4. Check if the failing rank maps to this node
  
→ Localize: is the problem on a specific node/GPU?
```

### Pattern-based correlation (from agentic-rc taxonomy)

```
Evidence pattern → Root cause classification:

1. NCCL_TIMEOUT + all nodes same SeqNum + low RDMA traffic
   → SOFTWARE_BUG (not network — RCCL/plugin issue)

2. NCCL_TIMEOUT + one node high RDMA errors + others clean
   → NETWORK_SINGLE_NODE (AINIC/cable issue on that node)

3. SIGSEGV + after N iterations + memory trending up
   → MEMORY_CORRUPTION (likely buffer overflow in kernel)

4. HUNG_AT_STEP1 + GPU util=100% power=low + triton warnings
   → COMPILATION_HANG (triton JIT taking too long, possibly infinite)

5. HUNG_AT_STEP1 + GPU util=100% power=high + no RDMA traffic
   → INTRA_NODE_COLLECTIVE_HANG (XGMI P2P issue)

6. All workloads with param X fail, others succeed
   → CONFIGURATION_ROOT_CAUSE (e.g., legacy_gg=False triggers TE bug)
```

---

## 4. Implementation Approach

### Phase 1: Liveness Monitor (training_performance based)

- New column or table: `workload_health_state` tracking current phase and expected interval
- In telemetry-processor or ai-advisor: when `training_performance` is written, update health state
- Periodic check (30s): query workloads whose last iteration is older than threshold
- Output: `workload_event` with type `HealthCheckHung`, `HealthCheckDegraded`

This requires minimal new code — just a periodic job querying existing DB data.

### Phase 2: Metric Anomaly Detection

- Add to ai-advisor: periodic job that for each running workload:
  - Queries VictoriaMetrics for GPU/RDMA/XGMI metrics
  - Compares against baseline (first 5 minutes of stable training)
  - Detects anomalies (spike/drop/divergence)
- Output: `workload_event` with type `MetricAnomaly`

Can reuse agentic-rc's `detect_anomalies.py` logic (spike, drop, cross-node comparison, memory leak).

### Phase 3: Multi-Signal Correlation

- When a `HealthCheckHung` or `MetricAnomaly` event fires:
  - Gather all signals from the 6 dimensions for a time window around the event
  - Apply pattern matching against known root cause patterns
  - Produce a structured diagnosis
- Output: `workload_diagnosis` table or extension of `workload_event` with diagnosis detail

### Phase 4: Proactive Configuration Check

- At workload start (when `gpu_workload` status becomes Running):
  - Check training parameters against known-bad combinations
  - Check image/driver versions against compatibility matrix
  - Fire early warning if configuration is risky
- Output: `workload_event` with type `ConfigurationWarning`

---

## 5. Data Flow

```
                    ┌─────────────────┐
                    │  Training Logs   │──→ telemetry-processor ──→ training_performance (DB)
                    │  (OpenSearch)    │                          ──→ Log Alert Engine → alert_events
                    └─────────────────┘
                    ┌─────────────────┐
                    │  GPU/RDMA/XGMI   │──→ VictoriaMetrics
                    │  (Prometheus)    │
                    └─────────────────┘
                    ┌─────────────────┐
                    │  K8s Events      │──→ OpenSearch k8s-event-*
                    │  (API Server)    │
                    └─────────────────┘
                    ┌─────────────────┐
                    │  Pod/Workload    │──→ gpu_workload, gpu_pods (DB)
                    │  State (K8s)     │
                    └─────────────────┘
                            │
                            ▼
                ┌───────────────────────┐
                │  Workload Diagnosis    │  ← NEW
                │  Engine (ai-advisor)   │
                │                        │
                │  1. Liveness Monitor   │──→ Check training_performance cadence
                │  2. Metric Anomaly     │──→ Query VM for GPU/RDMA anomalies
                │  3. Log Error Detect   │──→ Consume alert_events from Log Alert Engine
                │  4. Config Check       │──→ Verify params + image at start
                │  5. Correlator         │──→ Multi-signal timeline correlation
                │                        │
                └───────────────────────┘
                            │
                            ▼
                ┌───────────────────────┐
                │  Output                │
                │  - workload_event      │  (phase transitions, anomalies)
                │  - Structured diagnosis│  (root cause classification)
                │  - Alert notifications │  (webhook, email)
                │  - Feed to agentic-rc  │  (pre-collected evidence)
                └───────────────────────┘
```

---

## 6. Relation to Existing Systems

| System | Role | Integration |
|--------|------|-------------|
| **telemetry-processor** | Data producer: training_performance, log alerts | Diagnosis engine consumes its output |
| **ai-advisor DetectionCoordinator** | Framework/intent detection | Can trigger diagnosis after detection completes |
| **ai-advisor LogAnalysis** | Pattern discovery for new log formats | Complementary — discovers patterns, diagnosis uses them |
| **Log Alert Engine** | Rule-based log error detection | Diagnosis engine subscribes to alert_events |
| **agentic-rc** | Deep LLM-driven root cause analysis | Diagnosis engine provides pre-collected evidence as input |
| **root_cause (CrewAI)** | Multi-agent RCA | Can consume diagnosis output as structured context |
| **GpuWorkloadJob / GpuPodJob** | Workload lifecycle tracking | Triggers diagnosis on state changes |

The diagnosis engine sits **between** raw data collection (telemetry-processor, node-exporter) and deep analysis (agentic-rc, root_cause), providing the real-time detection and signal correlation layer that is currently missing.
