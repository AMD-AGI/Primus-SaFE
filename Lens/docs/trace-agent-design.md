# Trace Agent 设计文档

## 概述

Trace Agent 是 Primus-Lens 可观测平台的训练/推理进程全链路追踪组件。基于 eBPF（内核态）+ bpftime（用户态）双层探针架构，对目标进程进行从 Python 调用栈到 RDMA 网络通信的完整调用链路采集，本地维持 10 分钟滑动窗口的 trace 数据，并支持持续上报或按需导出火焰图。

与传统 profiling 工具（py-spy、perf）不同，trace-agent 采用 **事件驱动** 而非轮询采样，仅在关键函数调用时触发采集，配合 bpftime 用户态 uprobe 的低开销特性（~270ns/probe vs 内核 uprobe ~2700ns），实现对训练进程近零干扰的全栈追踪。

### 在 Lens 架构中的位置

```
┌─ Lens 数据面 ──────────────────────────────────────────────────────────────┐
│                                                                            │
│  已有组件:                                                                  │
│  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────────────┐ │
│  │ node-exporter    │  │ network-exporter │  │ gpu-resource-exporter    │ │
│  │ (容器事件+进程树) │  │ (eBPF TCP flow)  │  │ (K8s workload lifecycle) │ │
│  └──────────────────┘  └──────────────────┘  └───────────────────────────┘ │
│                                                                            │
│  新增:                                                                      │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │ trace-agent (DaemonSet)                                                │ │
│  │ eBPF + bpftime 全链路调用追踪, 本地 ring buffer, 按需/持续上报          │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│           │                                                                │
│           ▼                                                                │
│  ┌──────────────────────┐                                                  │
│  │ telemetry-processor  │ ← 接收 trace events, 写入存储                    │
│  └──────────────────────┘                                                  │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 能力矩阵

### 核心能力

| # | 能力 | 描述 | 现有工具对比 |
|---|------|------|-------------|
| 1 | **全链路火焰图** | 从 Python 调用栈 → PyTorch C++ → RCCL 集合通信 → libibverbs RDMA → HIP GPU kernel → 内核驱动，一张火焰图展现完整调用链 | py-spy 只有 Python 栈; perf 只有 native 栈; 两者无法关联 |
| 2 | **RDMA 通信时序图** | 精确记录每次 ibv_post_send/ibv_poll_cq 的时间戳、QP、数据量、RTT，还原 RDMA 操作的完整时序 | 目前无工具能在不修改代码的情况下捕获用户态 RDMA 调用时序 |
| 3 | **事件驱动的 Python 栈采集** | 仅在 RCCL/RDMA/HIP 关键调用触发时才 walk Python frame chain，非轮询，近零空闲开销 | py-spy 100Hz 持续轮询，即使无事件也消耗 CPU |
| 4 | **10 分钟本地回溯窗口** | 节点本地保留最近 10 分钟 trace 数据，故障发生后可立即回溯，无需提前开启 profiling | 传统 profiling 需要提前开启，错过窗口则无数据 |
| 5 | **运行时 attach/detach** | 不需要重启训练进程，不需要修改训练代码，运行时动态挂载和卸载探针 | torch.profiler 需要修改代码; NCCL profiler 需要环境变量 |
| 6 | **管理面任务下发** | 中心化管理面可远程触发任意节点上的 trace 采集，支持自动侦测和手动指定 | 目前 Lens pyspy job 需要手动触发且只采集 Python 栈 |

### 可诊断的场景

| 场景 | trace-agent 提供的数据 | 诊断方式 |
|------|----------------------|---------|
| **训练 hang** | 所有 rank 的调用栈 + 最后一次 RCCL collective 的位置和时间 | 定位哪个 rank 卡在哪个 collective，是 RDMA 超时还是 GPU kernel 未完成 |
| **NCCL/RCCL 超时** | RDMA post_send/poll_cq 时序 + 重传事件 + QP 状态 | 区分是网络层丢包、对端未 post_recv、还是 NIC 故障 |
| **训练变慢（iteration 时间增长）** | 各层耗时占比火焰图: compute vs communication vs synchronization | 区分是 GPU 算力下降、RDMA 带宽下降、还是 all-reduce 等待不均 |
| **GPU 利用率低** | HIP kernel launch 时序 + hipStreamSynchronize 等待时间 | 区分是 data loading 瓶颈、通信等待、还是 kernel launch 开销 |
| **单卡掉速** | 特定 rank 的 RDMA RTT 异常 + GPU kernel 耗时异常 | 定位到具体节点/GPU/网卡的硬件问题 |
| **梯度同步不均** | 各 rank all-reduce 的起止时间差 | 发现 straggler rank，定位是 compute 不均还是网络不均 |
| **推理延迟抖动** | 单次推理的完整调用链耗时分解 | 定位延迟来源: tokenizer → model forward → sampling → KV cache |

### 数据输出能力

| 输出类型 | 格式 | 用途 |
|---------|------|------|
| 火焰图 | collapsed stack → SVG / speedscope JSON | 调用热点分析 |
| 时序图 | Chrome Trace JSON (Perfetto 兼容) | RDMA 通信时序、GPU kernel 并行度可视化 |
| 原始 trace | TraceEvent JSON stream | 供 AI Advisor 做自动诊断分析 |
| Prometheus 指标 | /metrics endpoint | trace session 数量、探针开销、buffer 使用率监控 |

---

## 架构设计

### 顶层结构

```
trace-agent (DaemonSet, per node, hostPID + privileged)
│
├── 1. DiscoveryEngine ── 发现目标进程
│   ├── ContainerdWatcher   (containerd 事件驱动, GPU Pod 生命周期)
│   ├── ProcScanner         (/proc 扫描, 定位根 Python 进程)
│   └── TaskReceiver        (HTTP API / ActionTask, 管理面下发任务)
│
├── 2. SessionManager ── 管理活跃的 trace session
│   ├── TraceSession[0..N]  (每个被追踪的 workload 一个 session)
│   ├── GC goroutine        (10s tick, 清理已结束 session)
│   └── Recovery            (重启后恢复未结束 session)
│
├── 3. Reporter ── 数据上报
│   ├── BatchReporter       (HTTP 批量上报 → telemetry-processor)
│   └── OnDemandDump        (API 触发导出)
│
└── 4. API Server (Gin :8990)
    ├── POST   /v1/trace/start          (创建 trace session)
    ├── POST   /v1/trace/stop           (停止 trace session)
    ├── GET    /v1/trace/sessions       (列出活跃 session)
    ├── GET    /v1/trace/dump/:id       (导出 trace 数据)
    ├── GET    /v1/trace/flamegraph/:id (生成火焰图)
    └── GET    /metrics                 (Prometheus 指标)
```

### 参照的 Lens 现有模式

| trace-agent 组件 | 参照来源 | 复用方式 |
|-----------------|---------|---------|
| DaemonSet 部署, hostPID | node-exporter | 相同部署模式 |
| Daemon 生命周期 (Start/Stop/Signal) | SaFE node-agent `Daemon` | 相同模式: context + signal + graceful shutdown |
| SessionManager | gpu-resource-exporter `listener/Manager` | 相同模式: map[uid]*Session + GC + Recovery |
| 内核 eBPF (cilium/ebpf + ringbuf) | network-exporter `BpfTcpFlow` | 直接复用库和模式 |
| HTTP BatchReporter | node-exporter `HTTPReporter` | 相同模式: buffer + batchSize + batchTimeout + flush |
| 任务接收 | jobs `ActionTaskExecutor` | 可复用, 也可纯 HTTP API |
| 进程发现 (containerd + /proc) | node-exporter `process-tree` collector | 复用 ProcReader + ContainerdReader |
| API Server (Gin) | node-exporter API | 相同框架 |

---

## 详细设计

### 1. Discovery Engine — 目标进程发现

支持两种并行的触发模式:

#### 1.1 自动侦测模式

复用 node-exporter 的 containerd 事件监听 + process-tree 扫描:

```
containerd EventService.Subscribe()
    │
    ├─ TaskStart 事件
    │   ├─ container.HasGpu() == false → 忽略
    │   └─ container.HasGpu() == true
    │       → ProcScanner.FindRootProcess(containerPID)
    │       → 判断是否为训练/推理进程 (匹配 python/torchrun/deepspeed 等)
    │       → SessionManager.CreateSession(pid, podMeta, defaultConfig)
    │
    └─ TaskExit 事件
        → SessionManager.StopSession(containerID)
```

进程定位流程:

```
PodUID
  → containerd: ListContainers(podUID)     // 获取容器 ID 和 PID
    → /proc: FindContainerProcesses(pid)   // 扫描容器内进程树
      → 匹配: cmdline 包含 python/torchrun/deepspeed
        → 取最顶层 Python 进程作为 rootPID
          → 解析 /proc/{pid}/maps 获取 .so 路径:
              librccl.so, libibverbs.so, libamdhip64.so, libpython3.x.so
```

#### 1.2 管理面任务下发模式

HTTP API 直接调用:

```
POST /v1/trace/start
{
  "pod_uid": "abc-123-def",
  "config": {
    "probes": ["rccl", "rdma", "hip", "python"],
    "buffer_duration": "10m",
    "buffer_size_mb": 256,
    "report_mode": "on_demand",
    "python_modules_filter": ["torch.nn", "torch.distributed"]
  }
}

Response:
{
  "session_id": "trace-abc123",
  "root_pid": 1234,
  "attached_probes": ["rccl", "rdma", "hip", "python"],
  "state": "active"
}
```

也兼容 ActionTask 轮询模式 (与现有 jobs ActionTaskExecutor 一致):

```
action_tasks 表:
  type: "start_trace"
  payload: { pod_uid, config }
  status: "pending" → "running" → "completed"
```

### 2. TraceSession — 单 Workload 追踪会话

每个被追踪的 workload 对应一个 TraceSession 实例:

```
TraceSession {
    ID            string          // "trace-{pod_uid_short}-{timestamp}"
    PodUID        string
    PodName       string
    Namespace     string
    NodeName      string
    RootPID       int             // 目标根进程 PID
    State         SessionState    // Attaching → Active → Detaching → Ended
    Config        TraceConfig
    StartTime     time.Time
    Libraries     LibraryPaths    // 目标进程加载的 .so 路径

    bpftimeProc   *BpftimeProcess        // bpftime 用户态探针子进程
    kernelProbes  *KernelProbeSet        // 内核 eBPF 探针集合
    ringBuffer    *TimeWindowRingBuffer  // 本地 10 分钟环形缓冲
    eventChan     chan TraceEvent         // 事件汇聚通道
}
```

#### Session 生命周期

```
CreateSession(pid, config)
    │
    ├── Phase 1: 解析目标进程
    │   readlink /proc/{pid}/root → 获取容器 rootfs
    │   解析 /proc/{pid}/maps → 找到 librccl.so / libibverbs.so / libamdhip64.so / libpython3.x.so 路径
    │   获取 Python 版本 (影响 PyFrameObject 结构布局)
    │
    ├── Phase 2: 启动 bpftime 用户态探针
    │   fork/exec bpftime-agent 子进程
    │   bpftime-agent 通过 ptrace attach 到目标进程
    │   加载 uprobe: RCCL / libibverbs / HIP / Python frame walker
    │   建立共享内存通道 (/dev/shm/trace-{session-id})
    │
    ├── Phase 3: 加载内核 eBPF 探针
    │   cilium/ebpf 加载 BPF 程序
    │   attach kprobe / tracepoint, filter by cgroup 或 PID
    │   创建 ringbuf reader → eventChan
    │
    ├── Phase 4: 启动 event consumer
    │   goroutine: 从 bpftime shm + kernel ringbuf → merge → TimeWindowRingBuffer
    │   goroutine: 可选持续上报 → BatchReporter
    │
    └── 状态: Active

StopSession(id)
    │
    ├── Detach bpftime-agent (发 SIGTERM, 等待退出)
    ├── Close 内核 eBPF probes (link.Close())
    ├── Flush ring buffer → 最终上报 / 保存
    └── 状态: Ended → 等待 GC 清理
```

### 3. 双层探针架构

#### 3.1 bpftime 用户态探针 (主力, 高频热路径)

bpftime 通过 binary rewriting 实现用户态 uprobe, 不需要内核态切换, 延迟 ~270ns/probe (比内核 uprobe 低 10x)。

| 探针目标 | Hook 函数 | 采集数据 |
|---------|----------|---------|
| **RCCL 集合通信** | `ncclAllReduce`, `ncclAllGather`, `ncclReduceScatter`, `ncclBroadcast`, `ncclSend`, `ncclRecv` | 操作类型、数据量 (count * datatype)、communicator ID、stream、entry/exit 时间戳 |
| **libibverbs RDMA** | `ibv_post_send`, `ibv_post_recv`, `ibv_poll_cq` | 操作类型 (SEND/RDMA_WRITE/RDMA_READ)、QP 号、数据长度、WR ID、completion status、RTT |
| **librccl-anp 插件** | ANP send/recv 函数 | RCCL channel → RDMA QP 的映射关系 |
| **HIP Runtime** | `hipLaunchKernel`, `hipMemcpyAsync`, `hipStreamSynchronize`, `hipEventRecord`, `hipEventSynchronize` | kernel 名称、grid/block 维度、memcpy 方向和大小、同步等待时间 |
| **Python 栈** | 在上述 uprobe 触发时, 就地 walk `PyThreadState` → `_PyInterpreterFrame` 链 | Python 函数名、文件名、行号 (完整调用栈) |

**Python 栈采集机制 (替代 py-spy)**:

bpftime 运行在目标进程地址空间内, 可以直接读取 CPython 解释器的内部结构:

```
uprobe 触发 (如 ncclAllReduce 被调用)
  │
  ├── 读取 TLS: PyThreadState *tstate = _PyThreadState_GET()
  ├── 获取当前帧: _PyInterpreterFrame *frame = tstate->cframe->current_frame  (3.11+)
  └── Walk frame chain:
      for each frame:
        PyCodeObject *code = frame->f_code
        func_name = code->co_qualname   // "MyModel.forward"
        filename  = code->co_filename   // "model.py"
        lineno    = frame->f_lineno     // 120
        frame     = frame->previous
```

与 py-spy 对比:

| | py-spy | bpftime 事件驱动栈采集 |
|---|---|---|
| 触发方式 | 固定频率轮询 (100Hz) | 仅在关键函数调用时触发 |
| 内存读取方式 | `process_vm_readv` 系统调用 (跨进程) | 进程内直接指针解引用 (零 syscall) |
| 空闲开销 | 持续消耗 CPU | 零 |
| 信息完整性 | 采样, 可能错过短调用 | 确定性, 每次关键调用都捕获 |
| 获取内容 | 仅 Python 栈 | Python 栈 + Native 栈, 天然关联 |

#### 3.2 内核 eBPF 探针 (辅助, 系统级上下文)

使用 cilium/ebpf 库 (与 network-exporter 一致), 用于捕获 bpftime 无法触及的内核态事件:

| 探针目标 | Hook 类型 | 采集数据 |
|---------|----------|---------|
| **KFD GPU 驱动** | kprobe: `kfd_ioctl` | GPU 内存分配/映射、page fault、GPU 调度 |
| **RDMA 控制面** | kprobe: `ib_modify_qp`, `ib_create_cq` | QP 状态转换、连接建立/断开 |
| **CPU 调度器** | tracepoint: `sched_switch`, `sched_wakeup` | 上下文切换延迟、CPU 迁移 (影响 RDMA 性能) |
| **内存子系统** | tracepoint: `mm_page_fault` | page fault 频率、NUMA 内存访问模式 |
| **进程退出** | tracepoint: `sched_process_exit` | 目标进程退出检测, 自动触发 session 清理 |
| **中断/完成** | tracepoint: `irq_handler_entry/exit` | RDMA completion 中断延迟 |

内核 eBPF 事件通过 cilium/ebpf ringbuf reader 传递到 Go 侧 (与 network-exporter 的 `BpfTcpFlow.doSyncEvent()` 相同模式)。

#### 3.3 RDMA 通信时序采集

这是 trace-agent 最有价值的独特能力。RDMA 数据路径绕过内核 (kernel bypass), 因此:
- 内核 eBPF 只能看到控制面 (QP 创建、状态转换)
- 数据面的 post_send / poll_cq 全部在用户态, 必须用 bpftime uprobe

```
bpftime uprobe                              内核 eBPF
     │                                           │
     ├─ ibv_post_send(qp, wr)                    │
     │   记录: timestamp_send, qp_num,            │
     │         wr_id, opcode, length              │
     │       ↓                                    │
     │   [RDMA NIC 硬件传输 - 两者都看不到]        │
     │       ↓                                    │
     │                                      ├─ IRQ: RDMA completion 中断
     │                                      │   记录: timestamp_irq
     ├─ ibv_poll_cq(cq, wc)                      │
     │   记录: timestamp_poll, wc.status,         │
     │         wc.wr_id, wc.byte_len             │
     │                                            │
     └─ 计算:
         RDMA RTT = timestamp_poll - timestamp_send
         硬件延迟 = timestamp_irq - timestamp_send
         软件延迟 = timestamp_poll - timestamp_irq
```

### 4. Go ↔ bpftime 通信

trace-agent 主进程 (Go) 与 bpftime-agent (C++) 之间通过共享内存通信, 避免 CGO:

```
trace-agent 进程 (Go)
    │
    │  fork/exec
    ├──────────────→  bpftime-agent 进程 (C++)
    │                    │
    │                    │ ptrace attach
    │                    ├──────────────→ 目标训练进程
    │                    │                  librccl.so    ← uprobe
    │                    │                  libibverbs.so ← uprobe
    │                    │                  libamdhip64.so← uprobe
    │                    │                  libpython3.x.so ← frame walk
    │                    │
    │  /dev/shm/trace-{session-id}
    │  ←─────────────────┤  bpftime ring buffer (shared memory)
    │                    │
    └── Go ShmReader: mmap 读取 → 反序列化 → eventChan
```

bpftime-agent 管理:

```
BpftimeProcess {
    cmd        *exec.Cmd       // 子进程句柄
    pid        int
    shmPath    string          // /dev/shm/trace-{session-id}
    shmReader  *ShmRingReader  // mmap 共享内存读取器
    configPath string          // probe 配置文件路径

    Start(targetPID int, config ProbeConfig) error  // fork + exec
    Stop() error                                     // SIGTERM + waitpid
    ReadEvents() <-chan RawTraceEvent                // 从 shm 读取事件流
    IsAlive() bool                                   // 检查子进程状态
}
```

### 5. TimeWindowRingBuffer — 10 分钟本地环形缓冲

Lens 现有组件 (node-exporter HTTPReporter) 使用简单 slice + flush, 不支持时间窗口回溯。trace-agent 需要新的 ring buffer:

```
TimeWindowRingBuffer {
    data         []byte           // mmap-backed, 固定大小 (默认 256MB)
    head         atomic.Uint64    // 写入位置
    tail         atomic.Uint64    // 最旧有效数据位置
    window       time.Duration    // 10 分钟
    entryCount   atomic.Uint64    // 有效 entry 数量
    index        []IndexEntry     // 时间索引 (每秒一个, 指向 data 中的 offset)
    mu           sync.RWMutex     // 保护 index
}

IndexEntry {
    Timestamp    uint64    // 秒级时间戳
    Offset       uint64    // 在 data 中的起始 offset
    EntryCount   uint32    // 该秒内的 event 数量
}
```

操作:

```
Write(event TraceEvent):
    1. 序列化 event → bytes
    2. 写入 data[head], 推进 head
    3. 更新 index (当前秒)
    4. 淘汰: while index[0].Timestamp < now - 10min:
         推进 tail 到 index[1].Offset
         删除 index[0]

ReadWindow(from, to time.Time) []TraceEvent:
    1. 通过 index 二分查找 from/to 对应的 offset 范围
    2. 顺序读取 data[offset_from..offset_to]
    3. 反序列化为 TraceEvent 列表

Snapshot() []byte:
    1. 复制当前 data[tail..head] (用于导出)
```

为什么用 mmap 而不是 Go slice:
- 固定大小, 不触发 GC
- 进程重启可选恢复 (映射到 tmpfs)
- bpftime 的 shm 数据可直接 mmap 到同一进程空间, 减少拷贝

### 6. TraceEvent 统一数据格式

所有探针层的事件统一为一种格式:

```
TraceEvent {
    Timestamp     uint64           // CLOCK_MONOTONIC 纳秒
    SessionID     string
    TID           uint32           // 线程 ID
    EventType     EventType        // 见下方枚举

    // entry/exit 配对
    Duration      uint64           // exit 时计算的耗时 (ns)

    // 混合调用栈 (Python + Native)
    StackFrames   []StackFrame

    // 事件特定属性 (不同 EventType 有不同字段)
    Attrs         EventAttrs
}

EventType 枚举:
    RCCLCollectiveEntry     // ncclAllReduce 等进入
    RCCLCollectiveExit      // ncclAllReduce 等返回
    RDMAPostSend            // ibv_post_send
    RDMAPollCQ              // ibv_poll_cq (含 completion 信息)
    HIPKernelLaunch         // hipLaunchKernel
    HIPMemcpy               // hipMemcpyAsync
    HIPSync                 // hipStreamSynchronize / hipEventSynchronize
    KFDIoctl                // GPU 驱动 ioctl
    SchedSwitch             // CPU 上下文切换
    ProcessExit             // 目标进程退出

StackFrame {
    Type     FrameType     // Python / Native / Kernel
    Symbol   string        // 函数名: "MyModel.forward" / "ncclAllReduce"
    File     string        // 文件路径
    Line     int
    Module   string        // .so 名 或 Python 模块名
}

EventAttrs (按 EventType 不同):
    // RCCL:
    OpType       string    // "AllReduce" / "AllGather" / ...
    Count        uint64    // 元素数量
    DataType     string    // "float16" / "float32"
    CommID       uint64    // communicator ID
    StreamID     uint64

    // RDMA:
    QPN          uint32    // Queue Pair Number
    Opcode       string    // "RDMA_WRITE" / "SEND" / ...
    Length       uint32    // 数据长度
    WRID         uint64
    Status       string    // completion status

    // HIP:
    KernelName   string    // GPU kernel 名称
    GridDim      [3]uint32
    BlockDim     [3]uint32
    SharedMem    uint32
    Direction    string    // memcpy: "HtoD" / "DtoH" / "DtoD"
```

### 7. SessionManager — 生命周期管理

参照 gpu-resource-exporter 的 `listener/Manager` 模式:

```
SessionManager {
    sessions     map[string]*TraceSession
    mu           sync.RWMutex
    ctx          context.Context
    cancelFunc   context.CancelFunc
    stateFile    string    // 本地状态持久化路径

    CreateSession(target TargetSpec, config TraceConfig) (string, error)
    StopSession(id string) error
    GetSession(id string) *TraceSession
    ListSessions() []SessionInfo

    // 后台 goroutine
    garbageCollect()      // 10s tick, 清理 Ended session (同 listener Manager)
    RecoverSessions()     // 重启后从 stateFile 恢复未结束 session
    watchProcessExit()    // 通过 kernel eBPF sched_process_exit 检测进程退出
}
```

GC 与恢复逻辑 (与 listener/Manager 一致):

```
InitManager(ctx):
    1. manager = newManager(ctx)
    2. manager.RecoverSessions()     // 从本地状态文件恢复
    3. go ticker(10s) → garbageCollect()

garbageCollect():
    for uid, session := range sessions:
        if session.State == Ended:
            delete(sessions, uid)
            cleanup shm, kernel probes

RecoverSessions():
    读取 stateFile → 获取上次未结束的 session 列表
    for each session:
        检查 /proc/{pid} 是否存在
        存在 → 重新 attach (CreateSession)
        不存在 → 标记 Ended
```

### 8. Reporter — 数据上报

参照 node-exporter HTTPReporter, 支持两种模式:

#### 8.1 持续上报模式 (continuous)

```
eventChan → BatchReporter
    batchSize:    1000 events
    batchTimeout: 5s
    → POST /v1/trace-events/batch → telemetry-processor
    → telemetry-processor 写入 ClickHouse / OpenSearch
```

#### 8.2 按需导出模式 (on_demand)

```
events → 仅写入本地 TimeWindowRingBuffer

触发导出:
    GET /v1/trace/dump/{session_id}?from=2026-03-18T10:00:00Z&to=2026-03-18T10:05:00Z
    → 读 ring buffer → 返回 TraceEvent JSON stream

触发火焰图:
    GET /v1/trace/flamegraph/{session_id}?from=...&to=...
    → 读 ring buffer
    → 折叠栈: "train_step;MyModel.forward;allreduce;ncclAllReduce;ibv_post_send 42"
    → 返回 collapsed stack 格式 (可直接被 speedscope / flamegraph.pl 消费)

触发时序图:
    GET /v1/trace/timeline/{session_id}?from=...&to=...
    → 读 ring buffer
    → 生成 Chrome Trace Event Format JSON
    → 可在 Perfetto UI (ui.perfetto.dev) 中打开
```

### 9. API 设计

```
基础路径: http://{pod-ip}:8990

Session 管理:
    POST   /v1/trace/start              创建 trace session
    POST   /v1/trace/stop               停止 trace session
    GET    /v1/trace/sessions           列出所有活跃 session
    GET    /v1/trace/session/:id        获取单个 session 详情

数据导出:
    GET    /v1/trace/dump/:id           导出原始 trace 数据 (JSON stream)
    GET    /v1/trace/flamegraph/:id     生成折叠栈火焰图数据
    GET    /v1/trace/timeline/:id       生成 Chrome Trace 时序图

查询参数:
    from       起始时间 (RFC3339)
    to         结束时间 (RFC3339)
    event_type 过滤事件类型 (可多选: rccl,rdma,hip)
    tid        过滤线程 ID

运维:
    GET    /metrics                     Prometheus 指标
    GET    /healthz                     健康检查
```

---

## 部署方案

### Kubernetes 部署

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: primus-lens-trace-agent
  namespace: primus-lens
spec:
  selector:
    matchLabels:
      app: primus-lens-trace-agent
  template:
    metadata:
      labels:
        app: primus-lens-trace-agent
    spec:
      hostPID: true
      hostNetwork: true
      containers:
      - name: trace-agent
        image: primus-lens/trace-agent:latest
        securityContext:
          privileged: true
        ports:
        - containerPort: 8990
          name: http
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: NODE_IP
          valueFrom:
            fieldRef:
              fieldPath: status.hostIP
        - name: TELEMETRY_PROCESSOR_URL
          value: "http://primus-lens-telemetry-processor:8989"
        volumeMounts:
        - name: proc
          mountPath: /host/proc
          readOnly: true
        - name: sys
          mountPath: /host/sys
          readOnly: true
        - name: dev-shm
          mountPath: /dev/shm
        - name: run-containerd
          mountPath: /run/containerd
          readOnly: true
        - name: bpf
          mountPath: /sys/fs/bpf
        - name: state
          mountPath: /var/lib/trace-agent
        resources:
          requests:
            cpu: 200m
            memory: 512Mi
          limits:
            cpu: "2"
            memory: 2Gi
      volumes:
      - name: proc
        hostPath: { path: /proc }
      - name: sys
        hostPath: { path: /sys }
      - name: dev-shm
        hostPath: { path: /dev/shm }
      - name: run-containerd
        hostPath: { path: /run/containerd }
      - name: bpf
        hostPath: { path: /sys/fs/bpf }
      - name: state
        hostPath:
          path: /var/lib/trace-agent
          type: DirectoryOrCreate
```

关键权限要求:
- `hostPID: true` — 需要访问宿主机进程 (ptrace attach, /proc 读取)
- `privileged: true` — 需要加载 eBPF 程序, ptrace 其他进程
- `/dev/shm` — bpftime 共享内存通信
- `/sys/fs/bpf` — eBPF maps pinning (可选)
- `/run/containerd` — containerd 事件监听

### 容器镜像内容

```
/usr/local/bin/trace-agent          # Go 主进程
/usr/local/bin/bpftime-agent        # bpftime C++ 二进制
/usr/local/lib/trace-probes/
    rccl.bpf.o                      # RCCL uprobe BPF 字节码
    rdma.bpf.o                      # libibverbs uprobe BPF 字节码
    hip.bpf.o                       # HIP runtime uprobe BPF 字节码
    python-walker.bpf.o             # Python frame walker BPF 字节码
    kernel-probes.bpf.o             # 内核 kprobe/tracepoint 字节码
/usr/local/lib/
    libbpftime-*.so                 # bpftime 运行时库
```

---

## 性能开销评估

| 探针类型 | 单次开销 | 预估触发频率 | 总开销/秒 |
|---------|---------|------------|----------|
| bpftime uprobe (RCCL) | ~270ns | ~100/s (每 iteration 几次 collective) | ~27μs |
| bpftime uprobe (RDMA) | ~270ns | ~10,000/s (大规模 all-reduce 拆分为多次 RDMA) | ~2.7ms |
| bpftime uprobe (HIP) | ~270ns | ~1,000/s (kernel launch) | ~270μs |
| bpftime Python frame walk | ~1-5μs | ~100/s (仅在 RCCL/RDMA 触发时) | ~100-500μs |
| 内核 eBPF tracepoint | ~2.7μs | ~1,000/s (sched_switch 等) | ~2.7ms |
| **总计** | | | **~6-8ms/s (< 1% CPU)** |

Ring Buffer 内存:
- 256MB 固定分配, 10 分钟窗口
- 单个 TraceEvent 平均 ~200 bytes (含 5 层栈帧)
- 10 分钟 @ 10,000 events/s = 6M events ≈ 1.2GB → 需做采样或聚合
- 实际方案: RDMA 层做 1/10 采样 (仅全采集 RCCL 层), 可控制在 256MB 内

---

## 实现路线

| 阶段 | 内容 | 依赖 | 预估工作量 |
|-----|------|------|----------|
| **P1: 骨架** | Daemon + SessionManager + API + containerd discovery | Lens core 库 | 1 周 |
| **P2: bpftime 集成** | BpftimeProcess 子进程管理 + shm 通信协议 + RCCL uprobe | bpftime v0.2+ | 2 周 |
| **P3: RDMA uprobe** | libibverbs post_send/poll_cq uprobe + RTT 计算 | P2 | 1 周 |
| **P4: Python 栈** | CPython frame walker (支持 3.10/3.11+) + 栈合并 | P2 | 2 周 |
| **P5: 内核 eBPF** | KFD/sched/IRQ tracepoint (cilium/ebpf) | 无 (可并行) | 1 周 |
| **P6: Ring Buffer** | mmap 环形缓冲 + 时间窗口 + 索引 | 无 (可并行) | 1 周 |
| **P7: 数据输出** | 火焰图生成 + Chrome Trace 输出 + BatchReporter | P6 | 1 周 |
| **P8: 管理面集成** | ActionTask 接入 + telemetry-processor trace 接收 API | Lens 管理面 | 1 周 |
| **P9: HIP tracing** | hipLaunchKernel/hipMemcpy/hipSync uprobe | P2 | 1 周 |

P1-P3 完成后即可交付核心能力: RCCL + RDMA 全链路追踪 + 火焰图。
