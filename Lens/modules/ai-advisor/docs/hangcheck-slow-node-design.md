# Hangcheck & Slow-Node Diagnosis Design

## 核心思路

将护航系统从"全面诊断"收窄到两个最高价值场景：
1. **Hangcheck** — 训练卡住不动（最常见的 GPU 浪费场景）
2. **Slow-node** — 训练在跑但某些节点拖慢整体（最隐蔽的性能损失）

设计分两层：
- **Detection 层**：基于现有 Lens 数据（training_performance + workload_gpu_* + workload_rdma_stat_*），轻量级、持续运行、所有 workload 默认覆盖
- **Diagnosis 层**：基于 trace-agent（RCCL/RDMA/HIP uprobe），重量级、按需触发、只在检测到异常时启动

```
Detection (always-on, all workloads)           Diagnosis (on-demand, triggered)
┌─────────────────────────────┐               ┌─────────────────────────────┐
│ training_performance 轮询    │──── HUNG ────→│ trace-agent: dump all ranks │
│ GPU util 跨节点对比          │               │ → 定位卡在哪个 collective    │
│ RDMA error rate 监控         │               │ → RDMA RTT/QP 状态          │
│ iteration interval 基线对比  │               │ → Python 调用栈              │
│                              │               │                             │
│ iteration interval 跨节点对比│── SLOW_NODE ─→│ trace-agent: 对比慢/快节点   │
│ GPU power 跨节点对比         │               │ → per-rank collective 耗时   │
│ XGMI throughput 跨节点对比   │               │ → compute vs comm 分解       │
└─────────────────────────────┘               └─────────────────────────────┘
```

---

## 1. Hangcheck

### 1.1 Detection：迭代心跳监控

**数据源**: `training_performance` 表

训练开始后，每个 iteration 都会写一条 `training_performance` 记录。监控这个"心跳"的停止即可检测 hang。

```
状态机:

WAITING_FOR_FIRST_ITER
  │
  ├─ 收到第一条 training_performance → 记录 T1
  │
  ▼
LEARNING_BASELINE (收集前 3 个 iteration 间隔)
  │
  ├─ 收集到 3 个间隔 → 计算 baseline_interval = median(intervals)
  │
  ▼
MONITORING
  │
  ├─ 每 30s 检查: now - last_iteration_at > baseline_interval * N ?
  │    N=3: SOFT_ALERT（可能在做 checkpoint 或 eval）
  │    N=10: HARD_ALERT（确定 hang）
  │    N=20: CRITICAL（长时间 hang，立即触发 trace-agent）
  │
  ├─ 特殊情况: 如果 checkpoint_event 刚发生，延长阈值到 5 分钟
  │
  └─ 始终更新 baseline（滑动窗口 EMA，排除 checkpoint 时段）
```

**为什么用 `training_performance` 而不是日志**:
- 已结构化：有 `iteration` 字段和 `created_at` 时间戳，不需要日志解析
- 去重：telemetry-processor 已做 workload+serial+iteration 去重
- 可查询：直接 SQL `SELECT MAX(iteration), MAX(created_at) FROM training_performance WHERE workload_uid = ?`

### 1.2 Detection：首步超时（编译阶段 hang）

今天这个 case 的核心问题：workload 在第一个 iteration 之前就卡住了（triton 编译 → ALLTOALL hang）。这时 `training_performance` 没有数据。

**补充信号**:
- `workload_event` 表中 `StartTrain` 事件的时间戳 → 标记训练循环开始
- 如果 `StartTrain` 后 T 分钟内没有第一条 `training_performance` → 首步超时

**T 的确定**: 
- 同集群/同镜像/同框架的历史 workload 的首步耗时统计
- 如果没有历史数据，默认阈值：4 层模型 5min，全量模型 20min

### 1.3 Diagnosis：trace-agent 触发

当 Detection 层发出 HARD_ALERT 或 CRITICAL：

```
1. 护航系统调用 trace-agent API (每个节点的 :8990):
   POST /v1/trace/start
   {
     "pod_uid": "<master-0 pod uid>",
     "config": {
       "probes": ["rccl", "rdma"],     // 只开 RCCL + RDMA，不要 HIP（减少噪音）
       "buffer_duration": "2m",         // 短窗口，快速采集
       "report_mode": "on_demand"
     }
   }

2. 等待 30s（让 ring buffer 积累数据）

3. 导出各节点的 trace:
   GET /v1/trace/dump/{session_id}?event_type=rccl,rdma

4. 分析 trace 数据:
```

**Hang 根因判定逻辑**:

```
从各 rank 的 RCCL trace 中找到最后一个 collective:

Case A: 所有 rank 都卡在同一个 collective (e.g., AllToAll_Base)
  → 且 RDMA post_send 有发出但 poll_cq 无返回
  → 诊断: RDMA 层通信死锁 / AINIC 网络问题
  → 进一步: 对比各 rank 的 RDMA RTT，找出最慢的 QP/节点

Case B: 所有 rank 都卡在同一个 collective
  → 但 RDMA 层没有任何活动（无 post_send）
  → 诊断: RCCL 层 hang（在进入 RDMA 之前就卡了）
  → 进一步: 看 Python 栈，可能是 GIL 死锁或 GPU kernel 未完成

Case C: 大部分 rank 在 collective，但 1-2 个 rank 不在
  → Straggler rank 还在做 compute，其他 rank 在等它
  → 诊断: 单卡/单节点计算慢（GPU 故障？内存问题？）
  → 进一步: 看 straggler rank 的 HIP kernel 耗时

Case D: 没有任何 RCCL collective 活跃
  → 训练循环本身没进入 collective
  → 诊断: 应用层 hang（Python 死锁、无限循环、IO 阻塞）
  → 进一步: Python 栈分析
```

### 1.4 Diagnosis 增强：10 分钟回溯窗口

trace-agent 的关键能力：本地保留 10 分钟 ring buffer。

这意味着 **hang 发生时数据已经在了**，不需要等采集。当 Detection 层发出 HARD_ALERT 时：

```
不是 "开始采集 → 等 30s → 导出"
而是 "直接导出过去 10 分钟的 trace → 立即分析"
```

这对 hangcheck 极其重要：hang 的根因往往在最后一个正常操作和第一个异常操作之间，回溯窗口能捕获这个转折点。

---

## 2. Slow-Node Diagnosis

### 2.1 Detection：跨节点性能对比

**数据源**: `training_performance` 表 + VictoriaMetrics `workload_gpu_*`

分布式训练中，所有 rank 应该以相近的速度推进。如果某个节点的 rank 经常是最慢的（straggler），就会拖慢整个训练。

**方法 1: Iteration interval per rank** (如果各 rank 分别记录 training_performance)

```
对于每个 iteration:
  rank_times[rank] = training_performance.created_at WHERE iteration=N AND pod_uid=<rank's pod>
  
slowest_rank = max(rank_times)
fastest_rank = min(rank_times)
gap = slowest - fastest

如果 gap > 0.1 * avg(rank_times):  // 最慢比最快慢 10%+
  → SLOW_NODE_DETECTED
  → 标记 slowest_rank 所在节点
```

**方法 2: GPU 指标对比** (如果 training_performance 只记录 master)

```
对同一个 workload 的所有 pod:
  gpu_util[node] = avg(workload_gpu_utilization{pod_name=<pod>})
  gpu_power[node] = avg(workload_gpu_power_usage{pod_name=<pod>})
  xgmi_tx[node] = sum(rate(workload_gpu_xgmi_link_tx{pod_name=<pod>}))

如果某节点的 gpu_power 持续低于其他节点 20%+:
  → 可能是 GPU 降频/故障
  
如果某节点的 xgmi_tx 持续低于其他节点 30%+:
  → 可能是 XGMI 链路降级
  
如果某节点的 gpu_util 持续高于其他节点:
  → 其他节点在等这个节点的通信完成（这个节点的网络慢）
```

**方法 3: RDMA 延迟对比**

```
对每个节点的每个 AINIC 设备:
  rdma_ack_timeout[node] = workload_rdma_stat_tx_rdma_ack_timeout
  rdma_retx[node] = workload_rdma_stat_tx_rdma_retx_pkts

如果某节点的 RDMA 错误率持续高于其他节点:
  → 该节点的 AINIC 可能有问题
```

### 2.2 Diagnosis：trace-agent 定向对比

当 Detection 层标记了一个 slow node：

```
1. 选择两个节点做对比:
   - Slow node (检测到的慢节点)
   - Reference node (性能最好的节点)

2. 在两个节点上同时启动 trace-agent:
   POST /v1/trace/start
   {
     "config": {
       "probes": ["rccl", "rdma", "hip"],  // 全量探针
       "buffer_duration": "5m"
     }
   }

3. 等待 2-3 个 iteration 的数据积累

4. 导出两个节点的 trace，做对比分析:
```

**对比分析维度**:

```
维度 1: RCCL collective 耗时
  对于每个 collective (AllReduce, AllToAll, etc.):
    slow_node_duration = avg(collective.exit - collective.entry)
    ref_node_duration  = avg(collective.exit - collective.entry)
    
  → 如果 slow_node 的 AllReduce 明显慢:
    → 进一步看 RDMA 层

维度 2: RDMA RTT
  对于每个 QP:
    slow_node_rtt = avg(poll_cq.timestamp - post_send.timestamp)
    ref_node_rtt  = avg(poll_cq.timestamp - post_send.timestamp)
    
  → 如果 slow_node 的某些 QP RTT 特别高:
    → 定位到具体的 AINIC 端口 (ionic_X)
    → 可能是特定链路的 cable/switch 问题

维度 3: HIP kernel 耗时
  对于频繁调用的 kernel:
    slow_node_kernel_time = avg(kernel duration)
    ref_node_kernel_time  = avg(kernel duration)
    
  → 如果 slow_node 的 kernel 耗时一致更高:
    → GPU 硬件问题（降频、ECC 修正开销、thermal throttle）

维度 4: 等待时间分解
  slow_node 的一个 iteration:
    compute_time = sum(HIP kernel time)
    comm_time    = sum(RCCL collective time)
    idle_time    = iteration_time - compute_time - comm_time
    
  ref_node 同理
  
  → compute_time 差大: GPU 问题
  → comm_time 差大: 网络问题
  → idle_time 差大: 同步等待/data loading 问题
```

### 2.3 慢节点根因分类

```
根据对比结果分类:

1. GPU_COMPUTE_SLOW
   - 表现: HIP kernel 耗时 slow > ref
   - 可能原因: GPU 降频、ECC 修正、thermal throttle、HBM 带宽降级
   - 验证: 检查 workload_gpu_ecc_correct_*, junction_temperature, gpu_clock

2. RDMA_LINK_SLOW
   - 表现: RDMA RTT 高，特定 QP 明显慢
   - 可能原因: AINIC 端口故障、cable 质量差、switch 拥塞
   - 验证: 对应 ionic_X 的 rdma_stat 错误计数

3. XGMI_LINK_DEGRADED
   - 表现: 节点内 collective 慢，XGMI throughput 低
   - 可能原因: XGMI link 降级、ECC 错误
   - 验证: workload_gpu_ecc_uncorrect_xgmi_wafl, xgmi_link_tx/rx

4. COMM_WAIT_STRAGGLER
   - 表现: slow_node 的 GPU util 高（在算），其他节点的 RCCL idle 时间长（在等）
   - 可能原因: 数据不均、模型参数不均
   - 验证: 检查 batch size / data loading 时间

5. IO_BOTTLENECK
   - 表现: idle_time 高，compute 和 comm 都正常
   - 可能原因: data loading 慢、NFS/storage 瓶颈
   - 验证: workload_container_fs_reads_bytes_total, disk IO metrics
```

---

## 3. 数据流总结

```
                                ALWAYS-ON (Detection)
                    ┌─────────────────────────────────────┐
                    │                                      │
training_performance│  Hangcheck:                          │
    (DB, per-iter)  │  - iteration heartbeat monitor       │
         ──────────→│  - first-step timeout                │
                    │  - checkpoint-aware threshold         │
workload_gpu_*      │                                      │
    (VM, 30s)       │  Slow-Node:                          │
         ──────────→│  - cross-node GPU util/power compare │
                    │  - cross-node XGMI throughput compare│
workload_rdma_stat_*│  - cross-node RDMA error compare     │
    (VM, 30s)       │  - iteration timing per rank compare │
         ──────────→│                                      │
                    └──────────┬──────────────┬────────────┘
                               │              │
                          HUNG alert     SLOW_NODE alert
                               │              │
                               ▼              ▼
                    ┌──────────────────────────────────────┐
                    │          ON-DEMAND (Diagnosis)        │
                    │                                      │
trace-agent         │  Hangcheck:                          │
  (eBPF+bpftime,    │  - dump 10min ring buffer            │
   per-node)        │  - find last RCCL collective per rank│
         ──────────→│  - check RDMA RTT / QP state         │
                    │  - get Python call stacks             │
                    │  → 输出: 卡在哪个 collective, 哪个 rank│
                    │                                      │
                    │  Slow-Node:                           │
                    │  - compare slow vs ref node traces    │
                    │  - decompose: compute/comm/idle       │
                    │  - per-QP RDMA RTT comparison         │
                    │  - per-kernel HIP timing comparison   │
                    │  → 输出: 慢在 GPU/RDMA/XGMI/IO        │
                    └──────────────────────────────────────┘
                               │
                               ▼
                    ┌──────────────────────────────────────┐
                    │  Output                               │
                    │  - workload_event: HangDetected,      │
                    │    SlowNodeDetected, DiagnosisResult  │
                    │  - Alert notification (webhook)       │
                    │  - Structured evidence for agentic-rc │
                    │  - Flame graph / timeline (Perfetto)  │
                    └──────────────────────────────────────┘
```

---

## 4. 与 trace-agent 的接口约定

Detection 层触发 Diagnosis 时，通过 trace-agent HTTP API 交互:

### Hangcheck 触发

```
1. 护航系统发现 hang (iteration stall)
2. 查询 gpu_pods 获取 workload 的所有 pod + 节点
3. 对每个节点调用 trace-agent:
   
   // 如果 session 已存在（自动侦测模式）:
   GET http://{node-ip}:8990/v1/trace/dump/{session_id}?from={hang_start-10m}&to={now}&event_type=rccl,rdma
   
   // 如果没有 session（按需模式）:
   POST http://{node-ip}:8990/v1/trace/start
   → wait 30s →
   GET /v1/trace/dump/{session_id}

4. 收集所有节点的 trace → 发送给 ai-advisor 分析
```

### Slow-node 触发

```
1. 护航系统发现 slow node (cross-node metric deviation)
2. 在 slow node 和 reference node 上启动/获取 trace:
   
   POST http://{slow-node-ip}:8990/v1/trace/start   config: rccl+rdma+hip
   POST http://{ref-node-ip}:8990/v1/trace/start     config: rccl+rdma+hip
   
   → wait 2-3 iterations →
   
   GET /v1/trace/dump/{slow_session_id}
   GET /v1/trace/dump/{ref_session_id}

3. 对比两个 trace → 分类根因
```

---

## 5. 实现优先级

### Phase 0 (可立即做，不依赖 trace-agent)

- [ ] Hangcheck Detection: 基于 training_performance 的 iteration 心跳监控
- [ ] 首步超时检测: StartTrain event → first training_performance 的间隔监控
- [ ] 跨节点 GPU 指标对比: 从 VictoriaMetrics 做 cross-node deviation

这一层只需要在 ai-advisor 里加一个 WorkloadEscortExecutor，查询现有 DB + VictoriaMetrics 数据。

### Phase 1 (需要 trace-agent P1-P3)

- [ ] Hang Diagnosis: 调用 trace-agent 获取 RCCL/RDMA trace，定位卡住的 collective
- [ ] 10 分钟回溯: 利用 ring buffer 获取 hang 前的完整 trace

### Phase 2 (需要 trace-agent P4-P9)

- [ ] Slow-node Diagnosis: 调用 trace-agent 获取 RCCL+RDMA+HIP trace，做 compute/comm/idle 分解
- [ ] Per-rank flame graph 对比
- [ ] Python 栈分析（GIL 死锁、应用层 hang）

### Phase 3 (完整闭环)

- [ ] 自动修复: hang → 诊断 → 如果是节点问题 → 自动 taint 节点 + relaunch
- [ ] 知识积累: 每次诊断结果写入知识库，更新 Detection 规则
- [ ] 预测: 基于历史 slow-node 数据，预测哪些节点可能出问题
