# Code-Aware Hangcheck Design

## 核心问题

固定超时阈值不可行：
- 4 层 DeepSeek V3 的 triton 编译可能 30 秒就完成
- 全量 671B 模型的 triton 编译可能需要 15 分钟
- 用 vLLM serving 的启动过程和 Megatron 训练完全不同
- 同一个框架不同超参（FP8 vs BF16、turbo_grouped_mlp vs legacy_gg）编译路径不同

**核心思路**：启动时分析代码 → 预测执行阶段序列 → 实时检查进度 → 阶段内异常时深入分析。

---

## 设计

### 三阶段流水线

```
Phase A: Code Analysis (workload 启动时)
  code_snapshot ──→ Cortex A2A ──→ 预测的阶段序列 + 每阶段的调用栈特征

Phase B: Progress Monitoring (workload 运行中)
  process tree / trace-agent ──→ 当前实际调用栈 ──→ 与预测阶段匹配
                                                     ↓
                                               匹配到的当前阶段
                                               该阶段已持续时间
                                                     ↓
                                            超过该阶段预期时间？
                                                 ↓           ↓
                                                否            是
                                                ↓             ↓
                                             继续监控      Phase C

Phase C: Deep Analysis (某阶段异常时触发)
  trace-agent dump ──→ Cortex 代码分析 ──→ "卡在这个函数是因为..."
```

### Phase A: 启动时代码分析

#### 已有数据（Lens ai-advisor code_snapshot_collector）

workload Running 后约 1 分钟，Lens 已经采集了：
- `entry_script`：主入口脚本内容
- `config_files`：训练配置（YAML/JSON）
- `local_modules`：本地 Python 模块
- `pip_freeze`：依赖版本
- `cmdline`：完整启动命令
- `env vars`：NCCL/RCCL/框架相关环境变量

#### 需要新增：调用 Cortex A2A 做阶段预测

```
输入:
  - entry_script 内容
  - config_files 内容
  - framework detection 结果 (megatron/deepspeed/vllm/...)
  - cmdline 参数
  - 关键 env vars (PRETRAIN_TYPE=FP8, TURBO_GROUPED_MLP=True, etc.)

Cortex 分析任务: "分析这个训练脚本的执行流程，预测从启动到开始训练的各个阶段"

输出: PhaseSequence
  phases:
    - id: "framework_init"
      description: "Primus/Megatron framework initialization"
      stack_signatures:        # 调用栈特征（用于匹配当前阶段）
        - "megatron.training.initialize"
        - "primus/cli/main.py"
        - "import torch"
      expected_log_patterns:   # 日志特征
        - "Loading config"
        - "Loaded config"
      expected_duration: "10-30s"
      
    - id: "model_construction"
      description: "Model initialization (DeepSeek V3 4-layer)"
      stack_signatures:
        - "megatron.core.models"
        - "DeepseekModel.__init__"
        - "MoELayer.__init__"
      expected_log_patterns:
        - "Setting up model"
        - "Number of parameters"
      expected_duration: "30s-2min"
      notes: "4 层模型较快，全量模型可能数分钟"
      
    - id: "distributed_init"
      description: "NCCL/RCCL process group initialization"
      stack_signatures:
        - "torch.distributed.init_process_group"
        - "ncclCommInitRank"
        - "ProcessGroupNCCL"
      expected_log_patterns:
        - "NCCL"
        - "process group"
        - "IPv6 network addresses"
      expected_duration: "30s-2min"
      notes: "AINIC 环境下可能有 ack timeout，属正常"
      
    - id: "data_setup"
      description: "Dataset and dataloader construction"
      stack_signatures:
        - "build_train_valid_test_datasets"
        - "GPTDataset"
        - "MockGPTDataset"
      expected_log_patterns:
        - "building train, validation"
        - "finished creating GPT datasets"
      expected_duration: "5s-2min"
      notes: "mock_data=True 时极快"
      
    - id: "compilation"
      description: "Triton/HIP kernel JIT compilation (first forward pass)"
      stack_signatures:
        - "triton"
        - "tl.where"
        - "hipLaunchKernel"
      expected_log_patterns:
        - "tl.where with a non-boolean"
        - "training ..."
        - "before the start of training step"
      expected_duration: "30s-10min"
      notes: "FP8+turbo_grouped_mlp 编译时间更长；BF16 更短"
      risk_factors:
        - condition: "PRETRAIN_TYPE=FP8 && moe_use_legacy_grouped_gemm=False"
          note: "已知 TE grouped gemm FP8 路径在 AINIC 上 ALLTOALL 会 hang"
          
    - id: "training_loop"
      description: "Training iterations"
      stack_signatures:
        - "train_step"
        - "forward_step"
        - "backward_step"
      expected_log_patterns:
        - "after N iterations"
        - "elapsed time per iteration"
      expected_duration: "per iteration ~1-5s (4 层), ~30-120s (全量)"
      monitoring: "iteration heartbeat"
```

#### Cortex 怎么做阶段预测

**方案 A: LLM 分析（推荐先用）**

通过 Cortex A2A `invoke` 调用，用 LLM 分析代码：

```python
# Cortex A2A invoke
POST /invoke
{
  "skill_id": "code_search",  # 或自定义 skill
  "input": {
    "task": "analyze_training_phases",
    "entry_script": "<code content>",
    "config": "<yaml content>",
    "framework": "megatron",
    "env": {"PRETRAIN_TYPE": "FP8", ...},
    "cmdline": "bash start_training_dsv3_4layers_proxy.sh"
  }
}

# LLM prompt (内置在 skill handler):
"""
分析以下训练脚本的执行流程。从启动到开始训练循环，列出每个阶段：
1. 阶段名称
2. 对应的函数/模块
3. 预期的日志输出特征
4. 预期耗时范围
5. 可能的风险点

代码：{entry_script}
配置：{config}
框架：{framework}
启动参数：{cmdline}
环境变量：{env}
"""
```

**方案 B: 规则 + SymbolExtractor（更可靠）**

用 Cortex 的 `SymbolExtractor` 做 AST 分析，结合框架知识库：

```python
# 1. tree-sitter 提取调用关系
symbols = symbol_extractor.extract(entry_script)

# 2. 匹配已知框架入口
framework_phases = {
    "megatron": [
        ("initialize_megatron", "framework_init"),
        ("get_model", "model_construction"),
        ("torch.distributed.init_process_group", "distributed_init"),
        ("build_train_valid_test_datasets", "data_setup"),
        ("train_step", "training_loop"),
    ],
    "deepspeed": [...],
    "vllm": [...],
}

# 3. 从调用图中确定实际执行顺序
# 4. 从 config 推断耗时范围 (num_layers → model size → init time)
```

**方案 C: 历史学习（最终形态）**

同框架 + 相似配置的历史 workload 已有 training_performance + workload_event 数据：

```sql
-- 找同框架、相似配置的历史 workload
SELECT wp.workload_uid, 
       we.type AS event_type, 
       we.created_at,
       tp.iteration, tp.created_at AS iter_time
FROM workload_detection wd
JOIN workload_event we ON we.workload_uid = wd.workload_uid
LEFT JOIN training_performance tp ON tp.workload_uid = wd.workload_uid
WHERE wd.framework = 'megatron' 
  AND wd.category = 'pre_training'
  -- 相似配置：同镜像、相似 GPU 数量
ORDER BY we.created_at
```

从历史数据统计：同配置 workload 的 StartTrain → first iteration 的 p50/p95 时间。

### Phase B: 实时进度匹配

#### 数据源

**低频 (已有)**: 30s 周期，覆盖所有 workload
- `training_performance` 表 → iteration 心跳
- `workload_event` 表 → StartTrain 事件
- OpenSearch 日志 → 日志模式匹配

**中频 (node-exporter)**: 5s 周期
- process tree API → Python 进程调用栈快照
- GPU utilization → 推断当前阶段 (idle/compiling/training)

**高频 (trace-agent, 按需)**: 实时
- RCCL/RDMA/HIP 调用链 → 精确定位当前操作

#### 阶段匹配逻辑

```
每 30 秒:
  1. 获取 master-0 最新日志 (OpenSearch, 最近 30s)
  2. 获取 process tree (node-exporter API, 当前调用栈)
  3. 获取 GPU metrics (VictoriaMetrics, 当前 util/power)
  
  4. 用日志匹配当前阶段:
     for phase in predicted_phases:
       if any(pattern in recent_logs for pattern in phase.expected_log_patterns):
         current_phase = phase
         break
  
  5. 用调用栈辅助确认:
     for phase in predicted_phases:
       if any(sig in process_stack for sig in phase.stack_signatures):
         current_phase = phase  # 覆盖日志匹配（更准确）
         break
  
  6. 检查当前阶段持续时间:
     phase_duration = now - phase_entered_at
     
     if phase_duration > phase.expected_duration.max:
       # 超过预期上限
       if phase_duration > phase.expected_duration.max * 2:
         → HARD_ALERT: "阶段 {phase.id} 持续 {phase_duration}s，预期最多 {max}s"
         → 触发 Phase C (深入分析)
       else:
         → SOFT_ALERT: "阶段 {phase.id} 可能偏慢"
```

#### 举例：今天的 FP8+TE GG 场景

```
T+0s:   workload 启动
T+30s:  code_snapshot_collector 采集代码
T+60s:  Cortex 分析完成，预测 6 个阶段:
        - framework_init: 10-30s
        - model_construction: 30s-2min
        - distributed_init: 30s-2min
        - data_setup: 5s (mock_data=True)
        - compilation: 30s-5min (FP8, 4层)
        - training_loop: ~1.8s/iter
        
        ⚠️ risk_factor 命中: "PRETRAIN_TYPE=FP8 && legacy_gg=False → 已知 TE GG hang"
        → 立即告警: KNOWN_BAD_CONFIG

T+90s:  进度匹配: 日志显示 "done with setup" → 进入 compilation 阶段
T+120s: GPU util=100%, power=280W → 确认在编译
T+5min: compilation 阶段已持续 3.5min (预期上限 5min) → 还在预期内
T+6min: 仍在 compilation (4.5min) → 接近上限，SOFT_ALERT
T+7min: 超过 5min → HARD_ALERT + 触发 trace-agent dump
T+7min: trace-agent 显示所有 rank 卡在 ALLTOALL_BASE → 确认 HUNG
        → 结合 risk_factor: "这是 TE GG FP8 路径的已知 bug"
```

对比没有代码分析的固定阈值：
- 固定 15 分钟阈值 → 白等 8 分钟才告警
- 代码分析后的 5 分钟阈值 → 提前 8 分钟发现问题

### Phase C: 异常时深入分析

当 Phase B 检测到某阶段超时：

```
1. 获取当前调用栈 (process tree 或 trace-agent)

2. 发送给 Cortex 做代码级分析:
   POST /invoke
   {
     "skill_id": "debugging",
     "input": {
       "question": "训练进程在以下位置卡住了超过 5 分钟，分析可能原因",
       "call_stack": "<当前 Python 调用栈>",
       "entry_script": "<代码内容>",
       "config": "<训练配置>",
       "phase": "compilation",
       "gpu_metrics": {"util": 100, "power": 280},
       "rdma_errors": {"ack_timeout": 68},
       "last_logs": "<最近 20 行日志>"
     }
   }

3. Cortex 分析输出:
   "进程卡在 megatron/core/transformer/moe/grouped_mlp.py 的 grouped_gemm 调用中。
    当前配置 moe_use_legacy_grouped_gemm=False 走的是 TE grouped gemm 路径。
    结合 NCCL 环境 (NCCL_NET_PLUGIN=librccl-anp.so) 和 FP8 精度，
    TE 的 all-to-all dispatch 在 AINIC 上存在兼容性问题。
    建议设置 moe_use_legacy_grouped_gemm=True。"
```

---

## 数据流

```
                  ┌──────────────────────────────────────────────────┐
                  │  Workload 启动                                    │
                  └──────────┬───────────────────────────────────────┘
                             │
                  ┌──────────▼───────────────────────────────────────┐
                  │  code_snapshot_collector (Lens, 已有)              │
                  │  → entry_script, config, modules, pip_freeze     │
                  └──────────┬───────────────────────────────────────┘
                             │
                  ┌──────────▼───────────────────────────────────────┐
                  │  Cortex A2A: 代码阶段预测                         │
                  │  输入: code + config + framework + env            │
                  │  输出: PhaseSequence                              │
                  │    (各阶段的栈特征 + 日志特征 + 预期耗时 + 风险点)  │
                  └──────────┬───────────────────────────────────────┘
                             │
              ┌──────────────▼──────────────────────────────┐
              │  Progress Monitor (新, ai-advisor executor)   │
              │                                              │
              │  每 30s:                                     │
              │  ├─ 查 OpenSearch 最新日志                     │
              │  ├─ 查 node-exporter 进程调用栈                │
              │  ├─ 查 VictoriaMetrics GPU 指标                │
              │  ├─ 匹配当前阶段                               │
              │  └─ 检查阶段是否超时                            │
              │                                              │
              │  检测到:                                      │
              │  ├─ 阶段超时 → 触发 trace-agent + Cortex 分析  │
              │  ├─ risk_factor 命中 → 立即告警                │
              │  └─ 正常 → 继续监控                            │
              └──────────────────────────────────────────────┘
                             │ (超时触发)
              ┌──────────────▼──────────────────────────────┐
              │  Deep Analysis                               │
              │  ├─ trace-agent: RCCL/RDMA/Python 栈        │
              │  └─ Cortex: 代码级原因分析                    │
              │     → "卡在 grouped_mlp.py:L120 的            │
              │        grouped_gemm，TE FP8 路径在             │
              │        AINIC 上不兼容"                        │
              └──────────────────────────────────────────────┘
```

---

## 实现路径

### Step 1: Cortex 新增训练阶段预测 Skill

在 `primus-cortex-internal` 中新增 A2A skill:

```
skill_id: "predict_training_phases"
输入: entry_script, config_files, framework, env, cmdline
输出: PhaseSequence (JSON)
实现: LLM prompt + 框架知识库
```

可复用 Cortex 已有能力:
- `SymbolExtractor`: 提取函数调用关系
- `CodeChunker`: 按函数切分分析
- LLM: 结合代码和框架知识做阶段标注

### Step 2: Lens ai-advisor 新增 PhaseAwareEscortExecutor

在 `workload_task_state` 中新增任务类型 `phase_aware_escort`:

```
触发: TaskCreator.ScanForRunningWorkloads 发现新的 Running workload
流程:
  1. 等待 code_snapshot_collector 完成
  2. 调用 Cortex A2A 获取 PhaseSequence
  3. 进入监控循环 (30s tick)
  4. 阶段匹配 + 超时检测
  5. 必要时触发 trace-agent + Cortex deep analysis
```

### Step 3: 历史学习

用已有 `training_performance` + `workload_detection` + `workload_code_snapshot` 数据，
统计同框架同配置 workload 的各阶段实际耗时，自动校准预测。

### Step 4: 知识库闭环

每次诊断结果写入知识库：
- "FP8 + TE GG + AINIC → ALLTOALL hang" 
- "DeepSeek V3 4层 triton 编译正常耗时 30-60s"
- "节点 106-2076 作为 master 时 RDMA ack_timeout 稳定 68"

下次遇到相同配置直接命中 risk_factor，不需要等超时。
