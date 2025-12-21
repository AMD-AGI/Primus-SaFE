# Detection Coordinator 设计文档

## 概述

本文档描述对 `ai-advisor` 模块中 Active Detection 系统的重构设计，将单体 `ActiveDetectionExecutor` 拆分为协调器 + 专项采集任务的架构。

---

## 1. 现状分析

### 1.1 当前架构

```
┌─────────────────────────────────────────────────────────────────┐
│                   ActiveDetectionExecutor                        │
│  - 选择目标 Pod                                                   │
│  - 探测进程信息                                                   │
│  - 探测环境变量                                                   │
│  - 探测镜像信息                                                   │
│  - 探测标签信息                                                   │
│  - 聚合证据                                                       │
│  - 判断框架                                                       │
│  - 重试调度                                                       │
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 当前问题

| 问题 | 描述 |
|------|------|
| **时机问题** | Pod 刚启动时进程可能还没就绪，上来就采集效率低且容易失败 |
| **单体问题** | 一个 executor 做所有事情，不够灵活，难以扩展 |
| **重复采集** | 无法追踪哪些源已经采集过，可能重复采集相同信息 |
| **缺乏协调** | 无法根据已有证据智能决定下一步采集什么 |
| **日志遗漏** | 日志流检测可能遗漏历史窗口，缺乏回扫机制 |
| **代码重复** | 与 `MetadataCollectionExecutor` 存在大量重复逻辑 |

### 1.3 与 MetadataCollectionExecutor 的重复

| 功能 | ActiveDetectionExecutor | MetadataCollectionExecutor |
|------|------------------------|---------------------------|
| 选择目标 Pod | `selectTargetPod` | `selectTargetPod` |
| 获取进程树 | `probeProcessInfo` | `getProcessTree` |
| 查找 Python 进程 | `findFirstPythonProcess` | `findTopLevelPythonProcess` |
| 提取环境变量 | `probeEnvInfo` | `extractEnvMap` |

---

## 2. 目标架构

### 2.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Workload 发现                                    │
└─────────────────────────────────┬───────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    DetectionCoordinator（协调器）                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │ 状态机: INIT → WAITING → PROBING → ANALYZING → CONFIRMED/WAITING │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  职责:                                                                   │
│  1. 检查 workload 状态（是否就绪、是否终止）                               │
│  2. 查询证据覆盖情况（哪些源已采集、哪些未采集）                            │
│  3. 决策并下发采集任务                                                    │
│  4. 聚合证据并判断框架                                                    │
│  5. 管理重试调度                                                          │
└────────────────────────────────┬────────────────────────────────────────┘
                                 │ 下发子任务
        ┌────────────────────────┼────────────────────────┐
        ▼                        ▼                        ▼
┌───────────────┐       ┌───────────────┐       ┌───────────────┐
│ ProcessProbe  │       │  LogDetection │       │  ImageProbe   │
│    Task       │       │     Task      │       │    Task       │
│               │       │               │       │               │
│ - cmdline     │       │ - 日志窗口扫描 │       │ - 镜像名      │
│ - env vars    │       │ - 模式匹配    │       │ - 镜像标签    │
│ - cwd         │       │ - 历史回扫    │       │               │
└───────┬───────┘       └───────┬───────┘       └───────┬───────┘
        │                       │                       │
        └───────────────────────┼───────────────────────┘
                                ▼
                 ┌──────────────────────────┐
                 │ workload_detection_      │
                 │     evidence 表          │
                 └──────────────────────────┘
                                │
                                ▼
                 ┌──────────────────────────┐
                 │   EvidenceAggregator     │
                 │   (计算置信度、判断框架)  │
                 └──────────────────────────┘
                                │
                                ▼
                 ┌──────────────────────────┐
                 │  workload_detection 表   │
                 │  (聚合结果)              │
                 └──────────────────────────┘
```

### 2.2 组件职责

| 组件 | 职责 | 执行模式 |
|------|------|---------|
| **DetectionCoordinator** | 总调度、状态管理、决策、聚合 | 周期性调度 |
| **ProcessProbeTask** | 采集进程信息（cmdline, env, cwd） | 一次性 |
| **LogDetectionTask** | 扫描指定时间窗口的日志 | 一次性 |
| **ImageProbeTask** | 采集镜像信息 | 一次性 |
| **LabelProbeTask** | 采集 Pod 标签/注解 | 一次性 |
| **PodProber** | 共享的 Pod 探测能力 | 工具类 |
| **EvidenceAggregator** | 聚合证据、计算置信度 | 被调用 |

---

## 3. 协调器状态机

### 3.1 状态定义

```go
const (
    CoordinatorStateInit      = "init"       // 初始状态，等待首次调度
    CoordinatorStateWaiting   = "waiting"    // 等待下次调度
    CoordinatorStateProbing   = "probing"    // 正在执行采集任务
    CoordinatorStateAnalyzing = "analyzing"  // 正在聚合分析
    CoordinatorStateConfirmed = "confirmed"  // 框架已确认
    CoordinatorStateCompleted = "completed"  // 任务完成（已确认或 workload 终止）
)
```

### 3.2 状态转换

```
                         ┌─────────┐
                         │  INIT   │ workload 刚发现
                         └────┬────┘
                              │ 初始延迟（如 30s）后首次调度
                              ▼
                         ┌─────────┐
                    ┌───►│ WAITING │◄──────────────────────┐
                    │    └────┬────┘                       │
                    │         │ 到达调度时间               │
                    │         ▼                            │
                    │    ┌─────────┐                       │
                    │    │ PROBING │ 下发并等待采集任务     │
                    │    └────┬────┘                       │
                    │         │ 所有任务完成               │
                    │         ▼                            │
                    │    ┌──────────┐                      │
                    │    │ANALYZING │ 聚合证据              │
                    │    └────┬─────┘                      │
                    │         │                            │
                    │   ┌─────┴─────┐                      │
                    │   ▼           ▼                      │
                    │ 未确认     已确认                    │
                    │   │           │                      │
                    └───┤           ▼                      │
                        │      ┌─────────┐                 │
                        │      │CONFIRMED│                 │
                        │      └────┬────┘                 │
                        │           │ 创建后续任务         │
                        │           ▼                      │
                        │      ┌─────────┐                 │
                        └─────►│COMPLETED│◄────────────────┘
                               └─────────┘   workload 终止
```

### 3.3 调度时机

| 场景 | 初始延迟 | 调度间隔 | 说明 |
|------|---------|---------|------|
| Pod 刚创建 | 30s | - | 等待进程启动 |
| 首次采集后未确认 | - | 30s | 快速重试 |
| 多次采集后未确认 | - | 60s | 指数退避，最大 60s |
| 已确认 | - | - | 停止调度 |
| workload 终止 | - | - | 停止调度 |

---

## 4. 证据覆盖追踪

### 4.1 新增表: detection_coverage

追踪每个 workload 各证据源的采集状态：

```sql
CREATE TABLE IF NOT EXISTS detection_coverage (
    id BIGSERIAL PRIMARY KEY,
    workload_uid VARCHAR(128) NOT NULL,
    source VARCHAR(50) NOT NULL,  -- process, log, image, label, wandb, import
    
    -- 覆盖状态
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- pending: 待采集
    -- collecting: 采集中
    -- collected: 已采集
    -- failed: 采集失败
    -- not_applicable: 不适用
    
    -- 采集记录
    attempt_count INT NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_error TEXT,
    
    -- 时间窗口覆盖（针对日志等有时间范围的源）
    covered_from TIMESTAMPTZ,     -- 已检测覆盖的起始时间
    covered_to TIMESTAMPTZ,       -- 已检测覆盖的结束时间
    pending_from TIMESTAMPTZ,     -- 待采集的起始时间（用于离线回扫）
    pending_to TIMESTAMPTZ,       -- 待采集的结束时间
    
    -- 日志源专用字段（由 telemetry-processor 上报更新）
    log_available_from TIMESTAMPTZ,  -- telemetry-processor 上报的最早日志时间戳
    log_available_to TIMESTAMPTZ,    -- telemetry-processor 上报的最新日志时间戳
    
    -- 采集结果统计
    evidence_count INT NOT NULL DEFAULT 0,
    
    -- 元数据
    ext JSONB DEFAULT '{}'::jsonb,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(workload_uid, source)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_dc_workload_uid ON detection_coverage(workload_uid);
CREATE INDEX IF NOT EXISTS idx_dc_status ON detection_coverage(status);
CREATE INDEX IF NOT EXISTS idx_dc_source ON detection_coverage(source);
CREATE INDEX IF NOT EXISTS idx_dc_pending ON detection_coverage(status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_dc_log_available ON detection_coverage(log_available_to) 
    WHERE source = 'log';
```

### 4.2 覆盖状态流转

```
                    ┌─────────┐
                    │ pending │ 初始状态
                    └────┬────┘
                         │ 开始采集
                         ▼
                    ┌─────────────┐
                    │ collecting  │ 采集中
                    └──────┬──────┘
                           │
               ┌───────────┴───────────┐
               ▼                       ▼
          ┌─────────┐            ┌─────────┐
          │collected│            │ failed  │
          └─────────┘            └────┬────┘
                                      │ 重试
                                      ▼
                                 ┌─────────┐
                                 │ pending │
                                 └─────────┘
```

---

## 5. 协调器决策逻辑

### 5.1 采集计划生成

```go
func (c *DetectionCoordinator) planCollectionTasks(ctx context.Context) []*CollectionPlan {
    plans := []*CollectionPlan{}
    workloadUID := c.workloadUID
    
    // 1. 检查进程证据
    processCoverage := c.getCoverage(ctx, "process")
    if c.shouldCollectProcess(processCoverage) {
        plans = append(plans, &CollectionPlan{
            TaskType: TaskTypeProcessProbe,
            Source:   "process",
            Priority: 100,
        })
    }
    
    // 2. 检查日志证据
    logCoverage := c.getCoverage(ctx, "log")
    if window := c.findUnscannedLogWindow(logCoverage); window != nil {
        plans = append(plans, &CollectionPlan{
            TaskType: TaskTypeLogDetection,
            Source:   "log",
            Priority: 80,
            Params: map[string]interface{}{
                "from": window.From,
                "to":   window.To,
            },
        })
    }
    
    // 3. 检查镜像证据
    imageCoverage := c.getCoverage(ctx, "image")
    if imageCoverage.Status == "pending" {
        plans = append(plans, &CollectionPlan{
            TaskType: TaskTypeImageProbe,
            Source:   "image",
            Priority: 60,
        })
    }
    
    // 4. 检查标签证据
    labelCoverage := c.getCoverage(ctx, "label")
    if labelCoverage.Status == "pending" {
        plans = append(plans, &CollectionPlan{
            TaskType: TaskTypeLabelProbe,
            Source:   "label",
            Priority: 40,
        })
    }
    
    return plans
}
```

### 5.2 采集条件判断

```go
// shouldCollectProcess 判断是否应该采集进程信息
func (c *DetectionCoordinator) shouldCollectProcess(coverage *DetectionCoverage) bool {
    // 已采集过且有证据 → 不需要再采集
    if coverage.Status == "collected" && coverage.EvidenceCount > 0 {
        return false
    }
    
    // 采集中 → 等待
    if coverage.Status == "collecting" {
        return false
    }
    
    // 失败次数过多 → 暂停
    if coverage.AttemptCount >= 5 {
        return false
    }
    
    // 检查 Pod 是否就绪
    if !c.isPodReady() {
        return false
    }
    
    // 检查 Pod 运行时间（至少 30 秒）
    if c.getPodAge() < 30*time.Second {
        return false
    }
    
    return true
}

// findUnscannedLogWindow 找到未扫描的日志窗口
func (c *DetectionCoordinator) findUnscannedLogWindow(coverage *DetectionCoverage) *TimeWindow {
    // 获取 workload 的日志时间范围（从 detection_coverage 表的 log_available_* 字段）
    logRange := c.getLogTimeRange()
    if logRange == nil {
        return nil
    }
    
    // 如果从未扫描过
    if coverage.CoveredTo.IsZero() {
        return &TimeWindow{
            From: logRange.From,
            To:   logRange.To,
        }
    }
    
    // 如果有新日志
    if logRange.To.After(coverage.CoveredTo) {
        return &TimeWindow{
            From: coverage.CoveredTo,
            To:   logRange.To,
        }
    }
    
    return nil
}
```

---

## 5.3 日志窗口追踪机制

### 5.3.1 与 telemetry-processor 的协作

日志检测窗口的追踪需要与 `telemetry-processor` 组件协作：

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         日志检测窗口追踪                                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────┐     上报日志检测结果      ┌──────────────────────┐ │
│  │ telemetry-       │ ───────────────────────► │    ai-advisor        │ │
│  │ processor        │    (包含检测时间戳)       │                      │ │
│  │                  │                          │  - 更新 log_available │ │
│  │ - 实时日志流处理  │                          │  - 记录检测窗口       │ │
│  │ - 框架模式匹配    │                          │                      │ │
│  │ - 上报检测时间戳  │                          │                      │ │
│  └──────────────────┘                          └──────────┬───────────┘ │
│                                                           │             │
│                                                           ▼             │
│                                              ┌──────────────────────┐   │
│                                              │ detection_coverage   │   │
│                                              │ (source='log')       │   │
│                                              │                      │   │
│                                              │ log_available_from   │   │
│                                              │ log_available_to     │   │
│                                              │ covered_from         │   │
│                                              │ covered_to           │   │
│                                              └──────────────────────┘   │
│                                                           │             │
│                                                           ▼             │
│                                              ┌──────────────────────┐   │
│                                              │ DetectionCoordinator │   │
│                                              │                      │   │
│                                              │ 比较:                │   │
│                                              │ log_available_to vs  │   │
│                                              │ covered_to           │   │
│                                              │                      │   │
│                                              │ 有差异 → 下发回扫任务 │   │
│                                              └──────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.3.2 detection_coverage 表扩展

为日志源添加额外字段：

```sql
-- detection_coverage 表（日志源专用字段）
ALTER TABLE detection_coverage ADD COLUMN IF NOT EXISTS log_available_from TIMESTAMPTZ;
ALTER TABLE detection_coverage ADD COLUMN IF NOT EXISTS log_available_to TIMESTAMPTZ;

-- log_available_from: telemetry-processor 上报的最早日志时间戳
-- log_available_to:   telemetry-processor 上报的最新日志时间戳
-- covered_from:       ai-advisor 已检测的起始时间
-- covered_to:         ai-advisor 已检测的结束时间
```

### 5.3.3 telemetry-processor 上报接口

`telemetry-processor` 在检测日志后，调用 ai-advisor 接口更新日志可用时间范围：

```go
// telemetry-processor 上报的结构
type LogDetectionReport struct {
    WorkloadUID     string    `json:"workload_uid"`
    DetectedAt      time.Time `json:"detected_at"`       // 检测时间
    LogTimestamp    time.Time `json:"log_timestamp"`     // 日志时间戳
    Framework       string    `json:"framework"`         // 检测到的框架（可选）
    Confidence      float64   `json:"confidence"`        // 置信度（可选）
    PatternMatched  string    `json:"pattern_matched"`   // 匹配的模式（可选）
}

// ai-advisor 接收接口
// POST /api/v1/detection/log-report
func (h *DetectionHandler) HandleLogReport(ctx *gin.Context) {
    var report LogDetectionReport
    if err := ctx.BindJSON(&report); err != nil {
        ctx.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // 更新 detection_coverage 的 log_available_to
    h.coverageFacade.UpdateLogAvailableTime(ctx, report.WorkloadUID, report.LogTimestamp)
    
    // 如果检测到框架，直接存储 evidence
    if report.Framework != "" {
        h.evidenceStore.CreateEvidence(ctx, &model.WorkloadDetectionEvidence{
            WorkloadUID: report.WorkloadUID,
            Source:      "log",
            SourceType:  "passive",
            Framework:   report.Framework,
            Confidence:  report.Confidence,
            DetectedAt:  report.DetectedAt,
            Evidence: model.ExtType{
                "pattern_matched": report.PatternMatched,
                "log_timestamp":   report.LogTimestamp,
            },
        })
    }
    
    ctx.JSON(200, gin.H{"status": "ok"})
}
```

### 5.3.4 日志窗口缺口检测

协调器判断是否有日志窗口缺口：

```go
// findUnscannedLogWindow 找到未扫描的日志窗口
func (c *DetectionCoordinator) findUnscannedLogWindow(coverage *DetectionCoverage) *TimeWindow {
    // log_available_from/to 由 telemetry-processor 上报
    // covered_from/to 记录 ai-advisor 已检测的范围
    
    // 场景 1: 从未检测过，但有日志可用
    if coverage.CoveredTo.IsZero() && !coverage.LogAvailableTo.IsZero() {
        return &TimeWindow{
            From: coverage.LogAvailableFrom,
            To:   coverage.LogAvailableTo,
        }
    }
    
    // 场景 2: 有新日志产生（telemetry-processor 上报了新的时间戳）
    if !coverage.LogAvailableTo.IsZero() && coverage.LogAvailableTo.After(coverage.CoveredTo) {
        return &TimeWindow{
            From: coverage.CoveredTo,
            To:   coverage.LogAvailableTo,
        }
    }
    
    // 场景 3: 早期日志遗漏（startup 阶段的日志可能没有被实时检测到）
    if !coverage.LogAvailableFrom.IsZero() && coverage.CoveredFrom.After(coverage.LogAvailableFrom) {
        return &TimeWindow{
            From: coverage.LogAvailableFrom,
            To:   coverage.CoveredFrom,
        }
    }
    
    return nil
}
```

### 5.3.5 时序示例

```
时间 ────────────────────────────────────────────────────────────────────►

[T+0s]    Workload 启动
          │
[T+5s]    Pod 开始产生日志
          │
[T+10s]   telemetry-processor 开始处理日志
          │ 上报: log_timestamp=T+5s
          │
          ▼
          detection_coverage 更新:
          - log_available_from: T+5s
          - log_available_to: T+10s
          
[T+30s]   首次调度
          │
          ├─► 检查日志窗口:
          │   log_available: [T+5s, T+30s] (telemetry-processor 持续上报)
          │   covered: 空
          │   缺口: [T+5s, T+30s]
          │
          └─► 不下发 LogDetectionTask（由 telemetry-processor 实时处理）
              但标记 covered_to = T+30s（表示实时检测已覆盖到此）
          
[T+60s]   第二次调度
          │
          ├─► 检查日志窗口:
          │   log_available: [T+5s, T+60s]
          │   covered: [T+5s, T+30s]
          │   缺口: [T+30s, T+60s]
          │
          └─► 缺口由实时检测覆盖，更新 covered_to = T+60s
          
[T+120s]  telemetry-processor 故障恢复
          │
          ├─► 上报: 发现 [T+60s, T+90s] 窗口的日志未检测
          │   log_available: [T+5s, T+120s]
          │   但 covered: [T+5s, T+60s]（故障期间未更新）
          │
          └─► 缺口: [T+60s, T+90s]
          
[T+150s]  协调器检测到缺口
          │
          └─► 下发 LogDetectionTask:
              - from: T+60s
              - to: T+90s
              - mode: "backfill" (回扫模式)
```

### 5.3.6 LogDetectionTask 增强

```go
type LogDetectionParams struct {
    WorkloadUID string
    From        time.Time
    To          time.Time
    BatchSize   int
    Mode        string  // "realtime" | "backfill"
}

// backfill 模式：从日志存储中查询历史日志
// realtime 模式：仅处理实时流（通常由 telemetry-processor 处理）
```

---

## 6. 子任务设计

### 6.1 ProcessProbeTask

**职责**: 采集进程信息（cmdline, env, cwd）

**执行流程**:
```
1. 获取 Pod 信息
2. 获取 node-exporter client
3. 调用 GetPodProcessTree
4. 遍历进程树，提取 Python 进程信息
5. 检测框架特征（cmdline 模式匹配、env 匹配）
6. 存储 evidence
7. 更新 detection_coverage
```

**输入**:
```go
type ProcessProbeParams struct {
    WorkloadUID string
    PodUID      string
    NodeName    string
}
```

**输出**:
```go
type ProcessProbeResult struct {
    Cmdlines     []string
    EnvVars      map[string]string
    ProcessNames []string
    Cwd          string
    Frameworks   []FrameworkEvidence
}
```

### 6.2 LogDetectionTask

**职责**: 扫描指定时间窗口的日志，进行框架检测

**执行流程**:
```
1. 查询指定时间窗口的日志
2. 对每条日志应用 PatternMatcher
3. 收集匹配结果
4. 存储 evidence
5. 更新 detection_coverage（更新 covered_to）
```

**输入**:
```go
type LogDetectionParams struct {
    WorkloadUID string
    From        time.Time
    To          time.Time
    BatchSize   int
}
```

**输出**:
```go
type LogDetectionResult struct {
    LogsScanned   int
    MatchesFound  int
    Frameworks    []FrameworkEvidence
    CoveredFrom   time.Time
    CoveredTo     time.Time
}
```

### 6.3 ImageProbeTask

**职责**: 采集镜像信息

**执行流程**:
```
1. 获取 Pod 或 Workload 的镜像信息
2. 解析镜像名和标签
3. 进行框架特征匹配
4. 存储 evidence
5. 更新 detection_coverage
```

### 6.4 LabelProbeTask

**职责**: 采集 Pod 标签和注解

**执行流程**:
```
1. 查询 Pod 的 labels 和 annotations
2. 检查是否有框架相关标签（如 app.kubernetes.io/name）
3. 存储 evidence
4. 更新 detection_coverage
```

---

## 7. 共享模块: PodProber

提取公共的 Pod 探测能力，供多个组件使用：

```go
// pkg/common/pod_prober.go

// PodProber 提供 Pod 探测的公共能力
type PodProber struct {
    collector      *metadata.Collector
    workloadFacade database.WorkloadFacadeInterface
    podFacade      database.PodFacadeInterface
}

// NewPodProber 创建 PodProber
func NewPodProber(collector *metadata.Collector) *PodProber

// SelectTargetPod 选择目标 pod（优先 master-0）
func (p *PodProber) SelectTargetPod(ctx context.Context, workloadUID string) (*model.GpuPods, error)

// GetNodeExporterClient 获取指定节点的 node-exporter client
func (p *PodProber) GetNodeExporterClient(ctx context.Context, nodeName string) (NodeExporterClient, error)

// GetProcessTree 获取 pod 的进程树
func (p *PodProber) GetProcessTree(ctx context.Context, pod *model.GpuPods, opts ProcessTreeOptions) (*types.PodProcessTree, error)

// FindPythonProcess 在进程树中找到 Python 进程
func (p *PodProber) FindPythonProcess(tree *types.PodProcessTree) *types.ProcessInfo

// ExtractEnvMap 从进程中提取环境变量
func (p *PodProber) ExtractEnvMap(proc *types.ProcessInfo) map[string]string

// ReadContainerFile 读取容器内文件
func (p *PodProber) ReadContainerFile(ctx context.Context, pid int, path string) (string, error)

// IsPodReady 检查 Pod 是否就绪
func (p *PodProber) IsPodReady(ctx context.Context, pod *model.GpuPods) bool

// GetPodAge 获取 Pod 运行时长
func (p *PodProber) GetPodAge(ctx context.Context, pod *model.GpuPods) time.Duration
```

---

## 8. 任务类型定义

```go
// core/pkg/constant/task.go

const (
    // 检测协调任务
    TaskTypeDetectionCoordinator = "detection_coordinator"
    
    // 检测采集子任务
    TaskTypeProcessProbe   = "detection_process_probe"
    TaskTypeLogDetection   = "detection_log_scan"
    TaskTypeImageProbe     = "detection_image_probe"
    TaskTypeLabelProbe     = "detection_label_probe"
    
    // 已有任务类型（保持不变）
    TaskTypeMetadataCollection  = "metadata_collection"
    TaskTypeTensorBoardStream   = "tensorboard_stream"
    TaskTypeProfilerCollection  = "profiler_collection"
    
    // 废弃（将被 DetectionCoordinator 取代）
    // TaskTypeActiveDetection = "active_detection"
)
```

---

## 9. 执行时序

### 9.1 正常流程

```
时间 ────────────────────────────────────────────────────────────────────►

[T+0s]    Workload 发现
          │
          ▼
          TaskCreator.CreateDetectionCoordinatorTask()
          │
          ▼
          DetectionCoordinator 创建 (状态: INIT)
          初始化 detection_coverage 记录:
          - process: pending
          - log: pending
          - image: pending
          - label: pending
          
[T+30s]   首次调度 (初始延迟后)
          │
          ├─► 检查 Pod 状态: 已就绪，运行 30s+
          ├─► 检查 coverage: process=pending, log=pending, image=pending
          │
          └─► 生成采集计划:
              - ProcessProbeTask (priority=100)
              - ImageProbeTask (priority=60)
          │
          ▼
          下发任务，状态 → PROBING
          
[T+32s]   ProcessProbeTask 完成
          │
          ├─► 检测到: cmdline 包含 "primus"
          ├─► 存储 evidence (source=process, framework=primus, confidence=0.85)
          └─► 更新 coverage (process: collected, evidence_count=1)
          
[T+33s]   ImageProbeTask 完成
          │
          ├─► 检测到: 镜像名包含 "primus-training"
          ├─► 存储 evidence (source=image, framework=primus, confidence=0.6)
          └─► 更新 coverage (image: collected, evidence_count=1)
          
[T+33s]   所有任务完成，状态 → ANALYZING
          │
          ├─► 调用 EvidenceAggregator.AggregateEvidence()
          │   - process: primus (0.85)
          │   - image: primus (0.6)
          │   - 多源加成: +0.1
          │   - 最终置信度: 0.93
          │
          └─► 置信度 >= 0.85 → 状态 → CONFIRMED
          
[T+33s]   框架确认后
          │
          ├─► 更新 workload_detection (status=confirmed, framework=primus)
          ├─► 创建 MetadataCollectionTask
          └─► 状态 → COMPLETED
```

### 9.2 需要多次采集的流程

```
时间 ────────────────────────────────────────────────────────────────────►

[T+30s]   首次调度
          │
          └─► 下发 ProcessProbeTask
          
[T+32s]   ProcessProbeTask 完成
          │
          ├─► 只检测到 pytorch (置信度 0.5)
          └─► 聚合后置信度 0.5 < 0.85
          │
          ▼
          状态 → WAITING，下次调度: T+62s
          
[T+62s]   第二次调度
          │
          ├─► 检查 coverage: log 有新窗口 (T+32s ~ T+62s)
          └─► 下发 LogDetectionTask
          
[T+65s]   LogDetectionTask 完成
          │
          ├─► 检测到: 日志中出现 "megatron" 特征
          ├─► 存储 evidence (source=log, framework=megatron, confidence=0.9)
          └─► 聚合: pytorch(0.5) + megatron(0.9) + 多源加成
          │
          ▼
          置信度 0.88 >= 0.85 → CONFIRMED
```

---

## 10. 文件变更清单

### 10.1 新增文件

| 文件路径 | 说明 |
|---------|------|
| `ai-advisor/pkg/task/detection_coordinator.go` | 检测协调器 |
| `ai-advisor/pkg/task/process_probe_executor.go` | 进程采集任务 |
| `ai-advisor/pkg/task/log_detection_executor.go` | 日志检测任务 |
| `ai-advisor/pkg/task/image_probe_executor.go` | 镜像采集任务 |
| `ai-advisor/pkg/task/label_probe_executor.go` | 标签采集任务 |
| `ai-advisor/pkg/common/pod_prober.go` | 共享 Pod 探测能力 |
| `core/pkg/database/detection_coverage_facade.go` | coverage 表操作 |
| `core/pkg/database/model/detection_coverage.gen.go` | coverage 模型 |
| `core/pkg/database/dal/detection_coverage.gen.go` | coverage DAL |
| `core/pkg/database/migrations/patch039-detection_coverage.sql` | 新表迁移 |

### 10.2 修改文件

| 文件路径 | 修改内容 |
|---------|---------|
| `core/pkg/constant/task.go` | 添加新任务类型常量 |
| `ai-advisor/pkg/detection/task_creator.go` | 修改为创建 Coordinator 任务 |
| `ai-advisor/pkg/bootstrap/bootstrap.go` | 注册新 executors |
| `ai-advisor/pkg/task/metadata_collection_executor.go` | 提取公共逻辑到 PodProber |
| `core/pkg/database/facade.go` | 添加 DetectionCoverageFacade |
| `ai-advisor/pkg/handlers/detection_handler.go` | 添加日志检测上报接口 |
| `ai-advisor/pkg/router/router.go` | 注册 `/detection/log-report` 路由 |

### 10.3 telemetry-processor 需配合修改

| 文件路径 | 修改内容 |
|---------|---------|
| `telemetry-processor/pkg/detector/log_detector.go` | 添加日志检测时间戳上报 |
| `telemetry-processor/pkg/client/ai_advisor_client.go` | 调用 ai-advisor 日志上报接口 |

### 10.3 废弃文件

| 文件路径 | 说明 |
|---------|------|
| `ai-advisor/pkg/task/active_detection_executor.go` | 被拆分为多个组件 |

---

## 11. 数据库变更

### 11.1 新增表

```sql
-- patch039-detection_coverage.sql
CREATE TABLE IF NOT EXISTS detection_coverage (
    -- 见第 4.1 节
);
```

### 11.2 修改表

无需修改现有表结构。

---

## 12. 迁移计划

### Phase 1: 基础设施（1-2 天）

- [ ] 创建 `detection_coverage` 表
- [ ] 实现 `DetectionCoverageFacade`
- [ ] 实现 `PodProber` 共享模块
- [ ] 添加新任务类型常量

### Phase 2: 子任务实现（2-3 天）

- [ ] 实现 `ProcessProbeExecutor`
- [ ] 实现 `LogDetectionExecutor`
- [ ] 实现 `ImageProbeExecutor`
- [ ] 实现 `LabelProbeExecutor`
- [ ] 为每个子任务编写单元测试

### Phase 3: 协调器实现（2-3 天）

- [ ] 实现 `DetectionCoordinator` 状态机
- [ ] 实现采集计划生成逻辑
- [ ] 实现子任务调度逻辑
- [ ] 集成 `EvidenceAggregator`
- [ ] 编写协调器测试

### Phase 4: 集成与迁移（1-2 天）

- [ ] 修改 `TaskCreator` 创建 Coordinator 任务
- [ ] 修改 `MetadataCollectionExecutor` 使用 `PodProber`
- [ ] 注册所有新 executors
- [ ] 废弃 `ActiveDetectionExecutor`
- [ ] 端到端测试

### Phase 5: 文档与清理（1 天）

- [ ] 更新设计文档
- [ ] 清理废弃代码
- [ ] 更新 README

---

## 13. 风险评估

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| 子任务调度延迟影响检测时效 | 中 | 中 | 优化任务调度优先级 |
| 多任务并发导致资源竞争 | 低 | 中 | 限制并发数，添加超时 |
| 日志回扫数据量过大 | 中 | 高 | 添加批处理和限流 |
| 与现有被动检测冲突 | 低 | 低 | 被动检测直接写 evidence，协调器负责聚合 |

---

## 14. 监控指标

| 指标 | 说明 |
|------|------|
| `detection_coordinator_state` | 协调器状态分布 |
| `detection_task_duration_seconds` | 子任务执行耗时 |
| `detection_coverage_status` | 各源覆盖状态分布 |
| `detection_evidence_count` | 证据数量统计 |
| `detection_confirmation_latency` | 从发现到确认的时间 |

---

## 15. 后续优化

1. **智能调度**: 根据历史数据预测最佳采集时机
2. **自适应阈值**: 根据 workload 类型动态调整确认阈值
3. **增量日志扫描**: 实时日志流 + 离线回扫结合
4. **分布式协调**: 支持多实例协调避免重复采集

