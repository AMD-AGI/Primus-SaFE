# Workload Escort: Plugin Architecture Design

## 1. 动机

训练 workload 的故障模式非常多样（hang、慢节点、OOM、NCCL 超时、loss 发散、配置错误...），且随着框架/硬件/网络栈演进不断出现新的故障模式。护航系统需要一个**可扩展的插件架构**，让核心框架稳定不变，检测能力通过插件持续丰富。

## 2. 架构总览

```
┌─────────────────────────────────────────────────────────┐
│  Escort Runtime (框架层)                                 │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ PhaseTracker  │  │ PluginRunner │  │ EventEmitter  │  │
│  │ 阶段状态机     │  │ 调度 + 执行   │  │ 告警 + 事件    │  │
│  └──────┬───────┘  └──────┬───────┘  └───────┬───────┘  │
│         │                 │                   │          │
│         └────────┬────────┘                   │          │
│                  │                            │          │
│  ┌───────────────▼────────────────────────────▼───────┐  │
│  │  CheckContext (每次 tick 传给插件的上下文)             │  │
│  │  - WorkloadUID, Phase, PhaseAge                    │  │
│  │  - PodNames, NodeNames                             │  │
│  │  - DataAccessors (DB, VM, OpenSearch, trace-agent)  │  │
│  │  - PreviousResults (其他插件的输出)                   │  │
│  │  - SharedState (插件间共享状态)                       │  │
│  └────────────────────────────────────────────────────┘  │
│                           │                              │
│         ┌─────────────────┼─────────────────┐            │
│         │                 │                 │            │
│  ┌──────▼──────┐  ┌──────▼──────┐  ┌──────▼──────┐     │
│  │ Plugin A    │  │ Plugin B    │  │ Plugin C    │     │
│  │ (iteration  │  │ (NCCL log   │  │ (cross-node │ ... │
│  │  heartbeat) │  │  watch)     │  │  GPU compare│     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
```

## 3. 核心接口

### 3.1 Plugin 接口

```go
// CheckPlugin is the interface all escort plugins must implement.
type CheckPlugin interface {
    // Name returns the unique plugin name (e.g., "iteration_heartbeat")
    Name() string

    // Phases returns which workload phases this plugin should run in.
    // Empty = run in all phases.
    Phases() []Phase

    // Interval returns how often this plugin should be invoked.
    Interval() time.Duration

    // Init is called once when the escort session starts.
    // Plugins can read workload config, set up state, etc.
    Init(ctx CheckContext) error

    // Check is called periodically. Returns a CheckResult.
    Check(ctx CheckContext) CheckResult
}
```

### 3.2 CheckContext — 插件的数据访问

```go
// CheckContext provides plugins with everything they need.
type CheckContext struct {
    // Workload identity
    WorkloadUID    string
    WorkloadName   string
    Namespace      string
    PodNodes       map[string]string   // pod_name -> node_name

    // Current state
    Phase          Phase
    PhaseEnteredAt time.Time
    PhaseAge       time.Duration

    // Data accessors (plugins don't directly call HTTP/DB)
    DB             DBAccessor          // training_performance, gpu_workload, workload_event, etc.
    Metrics        MetricsAccessor     // VictoriaMetrics queries
    Logs           LogAccessor         // OpenSearch queries
    TraceAgent     TraceAgentAccessor  // trace-agent API (per node)
    ProcessTree    ProcessTreeAccessor // node-exporter process tree API

    // Cross-plugin communication
    SharedState    *SharedState        // key-value store shared across plugins
    PreviousAlerts []Alert             // alerts fired in this tick by earlier plugins

    // Output
    EmitAlert(alert Alert)             // fire an alert
    EmitEvent(event WorkloadEvent)     // write a workload_event record
    RequestPhaseTransition(Phase)      // suggest phase change to PhaseTracker
}
```

### 3.3 CheckResult

```go
type CheckResult struct {
    Status   CheckStatus  // OK, Warning, Critical, Unknown
    Message  string       // human-readable description
    Details  map[string]any // structured data for downstream consumption
}

type CheckStatus int
const (
    StatusOK       CheckStatus = iota
    StatusWarning              // something looks off, not yet critical
    StatusCritical             // definite problem detected
    StatusUnknown              // couldn't determine (e.g., data unavailable)
)
```

### 3.4 Phase 枚举

```go
type Phase string
const (
    PhaseScheduling    Phase = "SCHEDULING"    // pods pending
    PhaseInitializing  Phase = "INITIALIZING"  // pods running, framework loading
    PhaseCompiling     Phase = "COMPILING"     // triton/HIP JIT compilation
    PhaseTraining      Phase = "TRAINING"      // iteration loop active
    PhaseEvaluating    Phase = "EVALUATING"    // eval/validation step
    PhaseCheckpointing Phase = "CHECKPOINTING" // saving checkpoint
    PhaseCompleting    Phase = "COMPLETING"    // training done, cleanup
    PhaseExited        Phase = "EXITED"        // workload ended
)
```

## 4. 框架核心组件

### 4.1 PhaseTracker — 阶段状态机

PhaseTracker 本身也是通过注册的 **PhaseDetector 插件** 来确定阶段转换的：

```go
type PhaseDetector interface {
    DetectPhase(ctx CheckContext) (Phase, float64) // phase + confidence [0,1]
}
```

内置的 PhaseDetector：
- **LogBasedPhaseDetector**: 从最新日志匹配 phase 关键词
- **MetricBasedPhaseDetector**: 从 GPU util/power 推断 (idle→scheduling, util=100%+power低→compiling)
- **TrainingPerfPhaseDetector**: training_performance 有新数据 → TRAINING phase
- **CodeAwarePhaseDetector**: 用 Cortex 预测的 PhaseSequence 做匹配（可选，Phase A 完成后注入）

多个 detector 的结果通过置信度加权融合。

### 4.2 PluginRunner — 调度引擎

```go
type PluginRunner struct {
    plugins    []CheckPlugin
    intervals  map[string]time.Duration  // per-plugin interval
    lastRun    map[string]time.Time      // per-plugin last execution time
}

// RunTick executes all due plugins for the current tick
func (r *PluginRunner) RunTick(ctx CheckContext) []CheckResult {
    var results []CheckResult
    for _, p := range r.plugins {
        // Skip if not in the right phase
        if !phaseMatch(p.Phases(), ctx.Phase) { continue }
        // Skip if not due yet
        if time.Since(r.lastRun[p.Name()]) < p.Interval() { continue }
        
        result := p.Check(ctx)
        results = append(results, result)
        r.lastRun[p.Name()] = time.Now()
        
        // Make result available to subsequent plugins
        ctx.PreviousAlerts = append(ctx.PreviousAlerts, result.Alerts...)
    }
    return results
}
```

### 4.3 EscortSession — 单个 workload 的护航会话

```go
type EscortSession struct {
    WorkloadUID  string
    Plugins      []CheckPlugin
    PhaseTracker *PhaseTracker
    Runner       *PluginRunner
    SharedState  *SharedState
    CancelFunc   context.CancelFunc
}

// Run is the main loop (called by WorkloadEscortExecutor)
func (s *EscortSession) Run(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)  // base tick
    defer ticker.Stop()
    
    // Init all plugins
    checkCtx := s.buildContext()
    for _, p := range s.Plugins {
        p.Init(checkCtx)
    }
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            checkCtx := s.buildContext()
            
            // 1. Detect current phase
            s.PhaseTracker.Update(checkCtx)
            checkCtx.Phase = s.PhaseTracker.CurrentPhase()
            checkCtx.PhaseAge = s.PhaseTracker.PhaseAge()
            
            // 2. Run due plugins
            results := s.Runner.RunTick(checkCtx)
            
            // 3. Process results (alerts, events, phase suggestions)
            s.processResults(results)
            
            // 4. Exit if workload ended
            if checkCtx.Phase == PhaseExited {
                return
            }
        }
    }
}
```

## 5. 插件注册

```go
// Global plugin registry
var pluginRegistry = map[string]PluginFactory{}

type PluginFactory func(config map[string]any) CheckPlugin

func RegisterPlugin(name string, factory PluginFactory) {
    pluginRegistry[name] = factory
}

// Default plugin set (built-in)
func init() {
    RegisterPlugin("iteration_heartbeat", NewIterationHeartbeatPlugin)
    RegisterPlugin("first_step_timeout", NewFirstStepTimeoutPlugin)
    RegisterPlugin("nccl_error_watch", NewNcclErrorWatchPlugin)
    RegisterPlugin("cross_node_gpu_compare", NewCrossNodeGpuComparePlugin)
    RegisterPlugin("rdma_error_scan", NewRdmaErrorScanPlugin)
    RegisterPlugin("known_bad_config", NewKnownBadConfigPlugin)
}
```

也可以通过配置启用/禁用插件：

```yaml
# escort config (可存在 system_config 表或 ConfigMap)
escort:
  base_tick: 30s
  plugins:
    iteration_heartbeat:
      enabled: true
      soft_multiplier: 3     # 3x baseline = soft alert
      hard_multiplier: 10    # 10x baseline = hard alert
    first_step_timeout:
      enabled: true
      default_timeout: 10m   # fallback if no code analysis
    nccl_error_watch:
      enabled: true
    cross_node_gpu_compare:
      enabled: true
      deviation_threshold: 0.2  # 20% deviation = alert
    rdma_error_scan:
      enabled: true
    known_bad_config:
      enabled: true
    code_phase_predictor:
      enabled: false  # requires Cortex A2A
    trace_agent_diagnosis:
      enabled: false  # requires trace-agent deployment
```

## 6. 第一批插件设计

### 6.1 IterationHeartbeatPlugin

```
阶段: TRAINING
频率: 30s
逻辑:
  1. 查 training_performance: SELECT MAX(iteration), MAX(created_at) WHERE workload_uid=?
  2. 如果有新 iteration → 更新 baseline interval (EMA)
  3. 如果 now - last_iter_at > baseline * soft_multiplier → WARNING
  4. 如果 > baseline * hard_multiplier → CRITICAL + 建议触发 trace-agent
输出:
  - SharedState["last_iteration"] = N
  - SharedState["baseline_interval"] = Xs
  - Alert: "IterationStalled" / "IterationHeartbeatOK"
```

### 6.2 FirstStepTimeoutPlugin

```
阶段: INITIALIZING, COMPILING
频率: 30s
逻辑:
  1. 查 workload_event: SELECT created_at WHERE type='StartTrain' AND workload_uid=?
  2. 查 training_performance: SELECT MIN(created_at) WHERE workload_uid=?
  3. 如果有 StartTrain 但没有 training_performance:
     timeout = SharedState["predicted_first_step_timeout"] 或 default_timeout
     if now - start_train_at > timeout → CRITICAL
  4. 如果 CodePhasePredictor 产出了 PhaseSequence:
     用当前阶段的预期上限作为 timeout
输出:
  - Alert: "FirstStepTimeout"
依赖:
  - SharedState["predicted_first_step_timeout"] (来自 CodePhasePredictorPlugin)
```

### 6.3 NcclErrorWatchPlugin

```
阶段: ALL
频率: 30s
逻辑:
  1. 查 OpenSearch: query_string "Watchdog caught" OR "Signal 11" OR "ALLTOALL" in pod logs
  2. 如果命中:
     解析具体错误 (NCCL timeout? SIGSEGV? which rank?)
     → CRITICAL alert with details
输出:
  - Alert: "NcclCollectiveTimeout" / "Sigsegv" / "AlltoallHang"
```

### 6.4 CrossNodeGpuComparePlugin

```
阶段: TRAINING
频率: 60s
逻辑:
  1. 查 VictoriaMetrics: avg(workload_gpu_power_usage) by pod_name
  2. 计算各节点的 power 均值和标准差
  3. 如果某节点偏离 > deviation_threshold → WARNING "SlowNodeDetected"
  4. 同样检查 XGMI throughput 和 GPU utilization
输出:
  - SharedState["slow_node"] = node_name (如果有)
  - Alert: "SlowNodeDetected"
```

### 6.5 RdmaErrorScanPlugin

```
阶段: ALL
频率: 60s
逻辑:
  1. 查 VictoriaMetrics: increase(workload_rdma_stat_*{workload_name=X}[2m])
  2. 对比 init-phase baseline (SharedState["rdma_baseline"])
  3. 如果某个 error counter 的 rate 比 baseline 高 3x → WARNING
输出:
  - SharedState["rdma_baseline"] = {metric: rate} (first tick sets baseline)
  - Alert: "RdmaErrorSpike"
```

### 6.6 KnownBadConfigPlugin

```
阶段: INITIALIZING (只运行一次)
频率: N/A (Init 时运行)
逻辑:
  1. 从 workload spec 提取 training 参数 (通过 DB 或 K8s API)
  2. 匹配已知坏组合:
     - FP8 + moe_use_legacy_grouped_gemm=False + AINIC → "TE GG FP8 AINIC hang"
     - BF16 + use_turbo_grouped_mlp=True → "BF16 turbo SIGSEGV after 1 iter"
  3. 如果命中 → CRITICAL alert immediately
输出:
  - Alert: "KnownBadConfiguration"
知识库:
  - 存在 DB 表或 ConfigMap 中，可动态更新
  - 每次 RCA 诊断出新的坏组合 → 自动写入
```

## 7. 高级插件（后续扩展）

| 插件 | 阶段 | 依赖 | 用途 |
|------|------|------|------|
| CodePhasePredictorPlugin | INIT | Cortex A2A | 分析代码预测阶段序列，写入 SharedState |
| TraceAgentDiagnosisPlugin | (被触发) | trace-agent | CRITICAL alert 时自动 dump trace |
| MemoryLeakDetectorPlugin | TRAINING | VM | 监控 VRAM 趋势 |
| LossDivergencePlugin | TRAINING | DB | 监控 loss NaN/Inf/spike |
| CheckpointHealthPlugin | CHECKPOINTING | OpenSearch | 监控 checkpoint save 是否卡住 |
| ProcessLivenessPlugin | ALL | node-exporter | 检查 Python 进程是否还活着 |
| EccErrorPlugin | ALL | VM | 监控 GPU ECC errors |
| ThermalThrottlePlugin | TRAINING | VM | 监控 GPU 温度 |
| DataLoadingPlugin | TRAINING | trace-agent | 检测 data loading 瓶颈 |
| GradientHealthPlugin | TRAINING | DB | 检测梯度爆炸/消失 |

## 8. 与现有系统的集成

### 8.1 作为 ai-advisor TaskExecutor

```go
const TaskTypeWorkloadEscort = "workload_escort"

type WorkloadEscortExecutor struct {
    pluginRegistry map[string]PluginFactory
}

func (e *WorkloadEscortExecutor) Execute(ctx context.Context, task *model.WorkloadTaskState) error {
    workloadUID := task.WorkloadUID
    
    // Create session with configured plugins
    session := NewEscortSession(workloadUID, e.pluginRegistry)
    
    // Run until workload exits or context cancelled
    session.Run(ctx)
    
    return nil
}
```

由现有 `TaskCreator.ScanForRunningWorkloads` 触发，`TaskScheduler` 分发。

### 8.2 告警输出

- 写入 `workload_event` 表 (复用现有表，新增 event type)
- 通过 telemetry-processor Alert Router 发送通知 (webhook/email)
- 可选：写入 `alert_events` 表参与告警关联

### 8.3 为 agentic-rc 预收集证据

当 workload 失败时，EscortSession 的 SharedState 包含：
- 各阶段耗时
- RDMA baseline + 异常点
- GPU 指标快照
- 最后几条 NCCL 错误日志
- slow node 信息

打包为 JSON 作为 agentic-rc 的 pre-collected evidence。

## 9. 实现路线

| 阶段 | 内容 | 工作量 |
|------|------|--------|
| **P0: 框架骨架** | EscortSession + PluginRunner + PhaseTracker + CheckContext + 插件注册表 | 3天 |
| **P1: 核心插件** | IterationHeartbeat + FirstStepTimeout + NcclErrorWatch | 3天 |
| **P2: 设备插件** | CrossNodeGpuCompare + RdmaErrorScan | 2天 |
| **P3: 配置检测** | KnownBadConfig + 知识库表 | 2天 |
| **P4: TaskExecutor 集成** | 接入 ai-advisor TaskScheduler + 告警输出 | 2天 |
| **P5: 代码感知** | CodePhasePredictor (Cortex A2A 集成) | 3天 |
| **P6: Trace 集成** | TraceAgentDiagnosis (触发 + 分析) | 3天 |
| **P7: 高级插件** | MemoryLeak, LossDivergence, Checkpoint, ECC, Thermal | 按需 |
