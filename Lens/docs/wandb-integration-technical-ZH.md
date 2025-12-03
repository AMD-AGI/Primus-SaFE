# WandB 集成技术文档

## 概述

本文档详细介绍了 Primus Lens 系统中 WandB（Weights & Biases）数据上报和指标展示的完整技术实现，涵盖从 `wandb-exporter` 到 `telemetry-processor` 再到 `api` 的全链路数据流。

## 系统架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                          训练容器 (Pod)                               │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │  用户训练代码                                                   │   │
│  │    import wandb                                              │   │
│  │    wandb.init(project="my-project")                         │   │
│  │    wandb.log({"loss": 0.5, "accuracy": 0.95})              │   │
│  └────────────────────┬─────────────────────────────────────────┘   │
│                       │                                              │
│  ┌────────────────────▼─────────────────────────────────────────┐   │
│  │  primus-lens-wandb-exporter                                  │   │
│  │  - wandb_hook.py: 自动拦截 wandb.init() 和 wandb.log()       │   │
│  │  - data_collector.py: 收集框架检测数据                        │   │
│  │  - api_reporter.py: 异步上报到 telemetry-processor          │   │
│  └────────────────────┬─────────────────────────────────────────┘   │
└─────────────────────┬─│─────────────────────────────────────────────┘
                      │ │
          Framework   │ │ Metrics
          Detection   │ │ Data
                      │ │
          ┌───────────▼─▼─────────────┐
          │  telemetry-processor      │
          │  (Go Service)             │
          │                           │
          │  ┌─────────────────────┐  │
          │  │  WandB API Handler  │  │
          │  │  - Detection        │  │
          │  │  - Metrics          │  │
          │  │  - Logs/Training    │  │
          │  └──────────┬──────────┘  │
          │             │              │
          │  ┌──────────▼──────────┐  │
          │  │ wandb_detector.go   │  │
          │  │ (Framework Detection)│  │
          │  └──────────┬──────────┘  │
          │             │              │
          │  ┌──────────▼──────────┐  │
          │  │wandb_log_processor.go│ │
          │  │  (Metrics Storage)   │  │
          │  └──────────┬──────────┘  │
          └─────────────┼──────────────┘
                        │
                        │ Store to DB
                        │
          ┌─────────────▼──────────────┐
          │    PostgreSQL Database     │
          │                            │
          │  - training_performance    │
          │  - framework_detection     │
          │  - metrics_storage         │
          └─────────────┬──────────────┘
                        │
                        │ Query Data
                        │
          ┌─────────────▼──────────────┐
          │      Lens API Module       │
          │      (Go Service)          │
          │                            │
          │  ┌──────────────────────┐  │
          │  │ training_performance │  │
          │  │      _test.go        │  │
          │  │                      │  │
          │  │  API Endpoints:      │  │
          │  │  - GetDataSources    │  │
          │  │  - GetAvailableMetrics│ │
          │  │  - GetMetricsData    │  │
          │  │  - GetIterationTimes │  │
          │  └──────────┬───────────┘  │
          └─────────────┼──────────────┘
                        │
                        │ HTTP API
                        │
          ┌─────────────▼──────────────┐
          │   前端/Grafana/用户客户端   │
          └────────────────────────────┘
```

## 第一部分：wandb-exporter 数据采集层

### 1.1 自动拦截机制

`wandb-exporter` 通过 Python 的 import hook 机制实现零代码侵入的自动拦截：

#### 1.1.1 安装和激活

```python
# setup.py 安装时自动创建 .pth 文件
# 在 site-packages 目录下创建 primus_lens_wandb_hook.pth
# 内容：import primus_lens_wandb_exporter.wandb_hook

# Python 启动时自动加载 wandb_hook.py
# 注册 WandbImportHook 到 sys.meta_path
```

#### 1.1.2 拦截 wandb.init()

**位置**: `wandb_hook.py` 第 217-270 行

```python
def intercepted_init(*args, **kwargs):
    """拦截 wandb.init"""
    # 1. 获取分布式训练 rank 信息
    rank_info = self._get_rank_info()  # RANK, LOCAL_RANK, NODE_RANK, WORLD_SIZE
    
    # 2. 设置指标输出路径（可选的本地文件保存）
    output_path = self._setup_metrics_output()
    
    # 3. 调用原始 wandb.init()
    result = self.original_init(*args, **kwargs)
    
    # 4. 保存 run 对象和 run_id
    self.wandb_run = result
    self.run_id = result.id
    
    # 5. 异步上报框架检测数据
    if self.api_reporting_enabled:
        self._report_framework_detection(result)
    
    # 6. 重新拦截 wandb.log（因为 wandb.init 会覆盖它）
    wandb.log = intercepted_log
    
    return result
```

**关键环境变量**:
- `WORKLOAD_UID`: 工作负载唯一标识
- `POD_UID`: Pod 唯一标识
- `POD_NAME`: Pod 名称（必需）
- `RANK`, `LOCAL_RANK`, `NODE_RANK`, `WORLD_SIZE`: 分布式训练信息

#### 1.1.3 拦截 wandb.log()

**位置**: `wandb_hook.py` 第 272-338 行

```python
def intercepted_log(data: Dict[str, Any], step: Optional[int] = None, *args, **kwargs):
    """拦截 wandb.log"""
    # 1. 复制数据，添加 Primus Lens 标记
    enhanced_data = data.copy()
    enhanced_data["_primus_lens_enabled"] = True
    
    # 2. 可选：添加系统指标（CPU、内存、GPU）
    if enhance_metrics:
        enhanced_data["_primus_sys_cpu_percent"] = psutil.cpu_percent()
        enhanced_data["_primus_sys_memory_percent"] = psutil.virtual_memory().percent
        # GPU 指标...
    
    # 3. 保存到本地文件（可选）
    if save_local:
        self._save_metrics(enhanced_data, step)
    
    # 4. 异步上报指标到 API
    if self.api_reporting_enabled:
        self._report_metrics(data, step)
    
    # 5. 调用原始 wandb.log()
    return self.original_log(enhanced_data, step=step, *args, **kwargs)
```

### 1.2 框架检测数据收集

**位置**: `data_collector.py`

#### 1.2.1 框架检测层次

系统支持**双层框架检测**：

- **Wrapper Frameworks（包装框架）**: Primus, PyTorch Lightning, Transformers Trainer
- **Base Frameworks（基础框架）**: Megatron, DeepSpeed, JAX, Transformers

#### 1.2.2 数据收集流程

```python
def collect_detection_data(self, wandb_run) -> Dict[str, Any]:
    """收集框架检测数据"""
    # 1. 收集原始证据
    evidence = self._collect_raw_evidence(wandb_run)
    #    - WandB 信息（project, name, id, config, tags）
    #    - 环境变量（框架相关的环境变量）
    #    - PyTorch 信息（版本、CUDA、已导入模块）
    #    - Wrapper 框架检测（通过 import）
    #    - Base 框架检测（通过 import）
    #    - 系统信息（Python 版本、平台）
    
    # 2. 生成检测提示
    hints = self._get_framework_hints(evidence)
    #    - wrapper_frameworks: []
    #    - base_frameworks: []
    #    - confidence: "low" / "medium" / "high"
    #    - primary_indicators: []  # 检测依据
    
    # 3. 构造完整报告数据
    detection_data = {
        "source": "wandb",
        "type": "framework_detection_raw",
        "version": "1.0",
        "workload_uid": os.environ.get("WORKLOAD_UID", ""),
        "pod_uid": os.environ.get("POD_UID", ""),
        "pod_name": os.environ.get("POD_NAME", ""),
        "evidence": evidence,
        "hints": hints,
        "timestamp": time.time(),
    }
    
    return detection_data
```

#### 1.2.3 框架检测方法优先级

1. **Import 检测**（置信度 0.90）- 最强指标
   - 检测实际加载的 Python 模块
   - `_detect_wrapper_by_import()`: 检测 primus, lightning, transformers
   - `_detect_base_by_import()`: 检测 megatron, deepspeed, jax

2. **环境变量检测**（置信度 0.80）
   - `PRIMUS_CONFIG`, `PRIMUS_BACKEND`
   - `DEEPSPEED_CONFIG`, `DS_CONFIG`
   - `MEGATRON_CONFIG`, `MEGATRON_LM_PATH`
   - `JAX_BACKEND`, `JAX_PLATFORMS`

3. **WandB Config 检测**（置信度 0.70）
   - `config.framework`
   - `config.base_framework`
   - `config.trainer`

4. **PyTorch 模块检测**（置信度 0.60）
   - `sys.modules` 中的框架模块

5. **项目名检测**（置信度 0.50）
   - WandB project 名称包含的框架关键词

### 1.3 异步 API 上报

**位置**: `api_reporter.py`

#### 1.3.1 架构设计

```python
class AsyncAPIReporter:
    """异步 API 上报器"""
    def __init__(self):
        # 数据队列
        self.detection_queue = Queue(maxsize=100)
        self.metrics_queue = Queue(maxsize=1000)
        self.logs_queue = Queue(maxsize=1000)
        
        # 后台工作线程
        self.worker_thread = threading.Thread(target=self._worker_loop, daemon=True)
        
        # 配置
        self.api_base_url = os.environ.get(
            "PRIMUS_LENS_API_BASE_URL",
            "http://primus-lens-telemetry-processor:8080/api/v1"
        )
        self.batch_size = 10
        self.flush_interval = 5.0  # 秒
```

#### 1.3.2 上报端点

1. **框架检测上报**
   ```
   POST {api_base_url}/wandb/detection
   Content-Type: application/json
   
   {
     "source": "wandb",
     "type": "framework_detection_raw",
     "workload_uid": "...",
     "pod_name": "...",
     "evidence": { ... },
     "hints": { ... }
   }
   ```

2. **指标上报**
   ```
   POST {api_base_url}/wandb/metrics
   Content-Type: application/json
   
   {
     "source": "wandb",
     "workload_uid": "...",
     "pod_name": "...",
     "run_id": "...",
     "metrics": [
       {
         "name": "loss",
         "value": 0.5,
         "step": 100,
         "timestamp": 1234567890.123,
         "tags": {}
       }
     ]
   }
   ```

#### 1.3.3 批量处理

- **队列机制**: 使用 Python `Queue` 实现无锁线程安全队列
- **批量发送**: 当队列达到 `batch_size` 或超过 `flush_interval` 时触发批量发送
- **非阻塞**: 如果队列满，直接丢弃数据，避免阻塞训练

## 第二部分：telemetry-processor 处理层

### 2.1 API Handler

**位置**: `pkg/module/logs/wandb_api.go`

#### 2.1.1 端点定义

| 端点 | 方法 | 功能 | Handler |
|------|------|------|---------|
| `/api/v1/wandb/detection` | POST | 框架检测上报 | `ReceiveWandBDetection` |
| `/api/v1/wandb/metrics` | POST | 指标上报 | `ReceiveWandBMetrics` |
| `/api/v1/wandb/logs` | POST | 训练数据上报 | `ReceiveWandBLogs` |
| `/api/v1/wandb/batch` | POST | 批量上报 | `ReceiveWandBBatch` |

#### 2.1.2 WorkloadUID 解析

```go
// getWorkloadUIDsFromPodName 从 PodName 解析 WorkloadUID
func getWorkloadUIDsFromPodName(workloadUID string, podName string, apiName string) []string {
    // 1. 如果已提供 WorkloadUID，直接返回
    if workloadUID != "" {
        return []string{workloadUID}
    }
    
    // 2. 通过 PodName 从 pod_cache 查询关联的 workloads
    if podName != "" {
        workloads := pods.GetWorkloadsByPodName(podName)
        // 一个 Pod 可能属于多个 Workload
        // 返回所有关联的 WorkloadUID
    }
}
```

### 2.2 框架检测处理

**位置**: `pkg/module/logs/wandb_detector.go`

#### 2.2.1 检测流程

```go
func (d *WandBFrameworkDetector) ProcessWandBDetection(
    ctx context.Context,
    req *WandBDetectionRequest,
) error {
    // 1. 解析 WorkloadUID
    workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
    
    // 2. 执行框架检测规则
    result := d.detectFramework(req)
    // 返回：
    // - Framework: 主框架
    // - FrameworkLayer: "wrapper" 或 "base"
    // - WrapperFramework: 包装框架（如果有）
    // - BaseFramework: 基础框架（如果有）
    // - Confidence: 置信度
    // - Method: 检测方法
    
    // 3. 构造证据
    evidence := map[string]interface{}{
        "method": result.Method,
        "framework_layer": result.FrameworkLayer,
        "wrapper_framework": result.WrapperFramework,
        "base_framework": result.BaseFramework,
        // ...更多证据
    }
    
    // 4. 报告到 FrameworkDetectionManager
    err = d.detectionManager.ReportDetection(
        ctx,
        workloadUID,
        "wandb",           // source
        result.Framework,  // framework
        "training",        // workload_type
        result.Confidence,
        evidence,
    )
}
```

#### 2.2.2 检测规则优先级

```go
func (d *WandBFrameworkDetector) detectFramework(req *WandBDetectionRequest) *DetectionResult {
    // 1. Import 检测（置信度 0.90）
    if result := d.detectFromImportEvidence(req.Evidence); result != nil {
        return result
    }
    
    // 2. 环境变量检测（置信度 0.80）
    if result := d.detectFromEnvVars(req.Evidence.Environment); result != nil {
        return result
    }
    
    // 3. WandB Config 检测（置信度 0.70）
    if result := d.detectFromWandBConfig(req.Evidence.WandB.Config); result != nil {
        return result
    }
    
    // 4. PyTorch 模块检测（置信度 0.60）
    if result := d.detectFromPyTorchModules(req.Evidence.PyTorch); result != nil {
        return result
    }
    
    // 5. WandB 项目名检测（置信度 0.50）
    if result := d.detectFromWandBProject(req.Evidence.WandB.Project); result != nil {
        return result
    }
    
    return nil
}
```

### 2.3 指标数据处理

**位置**: `pkg/module/logs/wandb_log_processor.go`

#### 2.3.1 ProcessMetrics - 指标处理

```go
func (p *WandBLogProcessor) ProcessMetrics(
    ctx context.Context,
    req *WandBMetricsRequest,
) error {
    // 1. 解析 WorkloadUID
    workloadUID, err := resolveWorkloadUID(req.WorkloadUID, req.PodName)
    
    // 2. 存储到 MetricsStorage（时序数据库）
    for _, metric := range req.Metrics {
        storedMetric := &StoredMetric{
            WorkloadUID: workloadUID,
            Source:      constant.DataSourceWandB,
            RunID:       req.RunID,
            Name:        metric.Name,
            Value:       metric.Value,
            Step:        metric.Step,
            Timestamp:   time.Unix(0, int64(metric.Timestamp*1e9)),
        }
        p.metricsStorage.Store(ctx, storedMetric)
    }
    
    // 3. 聚合指标按 step，存储到 training_performance 表
    stepMetrics := make(map[int64]map[string]interface{})
    for _, metric := range req.Metrics {
        step := metric.Step
        if stepMetrics[step] == nil {
            stepMetrics[step] = make(map[string]interface{})
        }
        stepMetrics[step][metric.Name] = metric.Value
    }
    
    // 4. 按 step 存储
    for step, data := range stepMetrics {
        p.storeTrainingData(ctx, workloadUID, req.PodUID, req.RunID, &WandBLog{
            Step: step,
            Data: data,
        }, timestamp)
    }
}
```

#### 2.3.2 storeTrainingData - 数据持久化

```go
func (p *WandBLogProcessor) storeTrainingData(
    ctx context.Context,
    workloadUID, podUID, runID string,
    data *WandBLog,
    timestamp time.Time,
) error {
    // 1. 准备性能数据
    newPerformanceData := map[string]interface{}{
        "source": constant.DataSourceWandB,
        "run_id": runID,
        "step":   data.Step,
    }
    
    // 合并所有指标
    for key, value := range data.Data {
        newPerformanceData[key] = value
    }
    
    // 2. 检查是否已存在记录（相同 workload_uid + serial + iteration）
    existingPerf, err := database.GetFacade().GetTraining().
        GetTrainingPerformanceByWorkloadIdSerialAndIteration(
            ctx, workloadUID, serial, iteration)
    
    // 3. 如果存在，合并历史数据
    if existingPerf != nil {
        // 将旧数据放入 history 数组
        historyEntry := existingPerf.Performance
        historyEntry["updated_at"] = existingPerf.CreatedAt.Format(time.RFC3339)
        
        // 获取现有历史
        var history []interface{}
        if existingData["history"] != nil {
            history = existingData["history"].([]interface{})
        }
        history = append(history, historyEntry)
        
        // 合并新数据
        finalPerformanceData = merge(existingData, newPerformanceData)
        finalPerformanceData["history"] = history
    }
    
    // 4. 保存/更新记录
    perfRecord := &dbModel.TrainingPerformance{
        ID:          recordID,
        PodUUID:     podUID,
        Performance: encoded,
        Iteration:   int32(iteration),
        Serial:      int32(serial),
        WorkloadUID: workloadUID,
        DataSource:  constant.DataSourceWandB,
    }
    
    if recordID > 0 {
        trainingFacade.UpdateTrainingPerformance(ctx, perfRecord)
    } else {
        trainingFacade.CreateTrainingPerformance(ctx, perfRecord)
    }
}
```

#### 2.3.3 数据库模型

**training_performance 表结构**:

```sql
CREATE TABLE training_performance (
    id SERIAL PRIMARY KEY,
    workload_uid VARCHAR(255) NOT NULL,
    pod_uuid VARCHAR(255),
    serial INTEGER DEFAULT 1,
    iteration INTEGER NOT NULL,
    data_source VARCHAR(50) NOT NULL,  -- 'wandb', 'log', 'tensorflow'
    performance JSONB NOT NULL,        -- 存储所有指标的 JSON
    created_at TIMESTAMP NOT NULL,
    
    INDEX idx_workload_uid (workload_uid),
    INDEX idx_workload_uid_data_source (workload_uid, data_source),
    INDEX idx_workload_uid_serial_iteration (workload_uid, serial, iteration)
);
```

**performance JSONB 字段结构**（wandb 数据源）:

```json
{
  "source": "wandb",
  "run_id": "abc123",
  "step": 100,
  "loss": 0.5,
  "accuracy": 0.95,
  "learning_rate": 0.001,
  "history": [
    {
      "source": "wandb",
      "run_id": "abc123",
      "step": 100,
      "loss": 0.6,
      "accuracy": 0.93,
      "updated_at": "2024-01-01T10:00:00Z"
    }
  ],
  "created_at": "2024-01-01T10:00:00Z",
  "updated_at": "2024-01-01T10:05:00Z"
}
```

### 2.4 监控指标

**位置**: `pkg/module/logs/metrics.go`

系统提供丰富的 Prometheus 监控指标：

```go
// 请求计数
IncWandBRequestCount("detection" | "metrics" | "logs")

// 请求耗时
ObserveWandBRequestDuration(requestType, duration)

// 错误计数
IncWandBRequestErrorCount(requestType, errorType)

// 数据点计数
ObserveWandBMetricsDataPointCount(workloadUID, count)
ObserveWandBLogsDataPointCount(workloadUID, count)

// 存储计数
IncWandBMetricsStoreCount(workloadUID)
IncWandBMetricsStoreErrors(workloadUID)

// 训练性能存储
IncTrainingPerformanceSaveCount(workloadUID, dataSource)
IncTrainingPerformanceSaveErrors(workloadUID, dataSource, errorReason)

// 框架检测
IncFrameworkDetectionCount(framework, method, source)
ObserveFrameworkDetectionConfidence(framework, method, confidence)
IncFrameworkDetectionErrors(source, errorReason)
```

## 第三部分：API 指标查询层

**位置**: `Lens/modules/api/pkg/api/training_performance.go`

### 3.1 API 端点设计

#### 3.1.1 获取数据源列表

```
GET /api/v1/workloads/:uid/metrics/sources?cluster=<cluster_name>
```

**响应**:
```json
{
  "workload_uid": "abc-123",
  "data_sources": [
    {
      "name": "wandb",
      "count": 1500
    },
    {
      "name": "log",
      "count": 800
    }
  ],
  "total_count": 2
}
```

#### 3.1.2 获取可用指标列表

```
GET /api/v1/workloads/:uid/metrics/available?data_source=wandb&cluster=<cluster_name>
```

**响应**:
```json
{
  "workload_uid": "abc-123",
  "metrics": [
    {
      "name": "loss",
      "data_source": ["wandb"],
      "count": 1500
    },
    {
      "name": "accuracy",
      "data_source": ["wandb", "log"],
      "count": 2300
    }
  ],
  "total_count": 2
}
```

#### 3.1.3 获取指标数据

```
GET /api/v1/workloads/:uid/metrics/data?
    data_source=wandb&
    metrics=loss,accuracy&
    start=1704067200000&
    end=1704153600000&
    cluster=<cluster_name>
```

**响应**:
```json
{
  "workload_uid": "abc-123",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "loss",
      "value": 0.5,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "accuracy",
      "value": 0.95,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    }
  ],
  "total_count": 2
}
```

**查询参数**:
- `data_source`: 数据源过滤（可选）
- `metrics`: 指标列表，逗号分隔（可选，支持 "all" 或不指定返回所有）
- `start`: 开始时间戳（毫秒）（可选）
- `end`: 结束时间戳（毫秒）（可选）
- `cluster`: 集群名称（可选）

#### 3.1.4 获取迭代时间信息

```
GET /api/v1/workloads/:uid/metrics/iteration-times?
    data_source=wandb&
    start=1704067200000&
    end=1704153600000&
    cluster=<cluster_name>
```

**响应**:
```json
{
  "workload_uid": "abc-123",
  "data_source": "wandb",
  "data": [
    {
      "metric_name": "iteration",
      "value": 100,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    },
    {
      "metric_name": "target_iteration",
      "value": 10000,
      "timestamp": 1704067200123,
      "iteration": 100,
      "data_source": "wandb"
    }
  ],
  "total_count": 2
}
```

### 3.2 核心实现

#### 3.2.1 指标字段过滤

```go
// wandb 数据源的元数据字段（不是实际指标）
var wandbMetadataFields = map[string]bool{
    "step":       true,
    "run_id":     true,
    "source":     true,
    "history":    true,
    "created_at": true,
    "updated_at": true,
}

// 判断字段是否为实际指标
func isMetricField(fieldName string, dataSource string) bool {
    switch dataSource {
    case "wandb":
        return !wandbMetadataFields[fieldName]
    case "log", "tensorflow":
        return true
    default:
        return true
    }
}
```

#### 3.2.2 数据查询逻辑

```go
func GetMetricsData(ctx *gin.Context) {
    // 1. 解析参数
    workloadUID := ctx.Param("uid")
    dataSource := ctx.Query("data_source")
    metricsStr := ctx.Query("metrics")
    
    // 2. 解析指标列表
    var requestedMetrics []string
    var returnAllMetrics bool = true
    
    if metricsStr != "" && metricsStr != "all" {
        // 支持 Grafana 格式: {metric1,metric2}
        requestedMetrics = strings.Split(metricsStr, ",")
        returnAllMetrics = false
    }
    
    // 3. 查询数据库
    performances, err := database.GetFacade().GetTraining().
        ListTrainingPerformanceByWorkloadUIDAndDataSource(
            ctx, workloadUID, dataSource,
        )
    
    // 4. 构建数据点
    for _, p := range performances {
        for metricName, value := range p.Performance {
            // 过滤元数据字段
            if !isMetricField(metricName, p.DataSource) {
                continue
            }
            
            // 过滤非请求的指标
            if !returnAllMetrics && !metricsSet[metricName] {
                continue
            }
            
            dataPoints = append(dataPoints, MetricDataPoint{
                MetricName: metricName,
                Value:      convertToFloat(value),
                Timestamp:  p.CreatedAt.UnixMilli(),
                Iteration:  p.Iteration,
                DataSource: p.DataSource,
            })
        }
    }
}
```

### 3.3 Grafana 集成

#### 3.3.1 配置 SimpleJson 数据源

```json
{
  "name": "Primus Lens Metrics",
  "type": "simplejson",
  "url": "http://primus-lens-api:8080/api/v1",
  "access": "proxy",
  "jsonData": {
    "queryEndpoint": "/workloads/{workload_uid}/metrics/data"
  }
}
```

#### 3.3.2 查询示例

```json
{
  "targets": [
    {
      "target": "loss",
      "refId": "A",
      "type": "timeseries"
    },
    {
      "target": "accuracy",
      "refId": "B",
      "type": "timeseries"
    }
  ],
  "range": {
    "from": "2024-01-01T00:00:00Z",
    "to": "2024-01-02T00:00:00Z"
  },
  "variables": {
    "workload_uid": "abc-123",
    "data_source": "wandb"
  }
}
```

## 第四部分：配置和部署

### 4.1 wandb-exporter 配置

#### 4.1.1 环境变量

```bash
# Hook 开关
PRIMUS_LENS_WANDB_HOOK=true  # 启用 Hook 拦截（默认 true）

# API 上报配置
PRIMUS_LENS_WANDB_API_REPORTING=true  # 启用 API 上报（默认 true）
PRIMUS_LENS_API_BASE_URL=http://primus-lens-telemetry-processor:8080/api/v1

# 本地保存配置
PRIMUS_LENS_WANDB_SAVE_LOCAL=true  # 启用本地保存（默认 true）
PRIMUS_LENS_WANDB_OUTPUT_PATH=/mnt/output  # 本地保存路径

# 系统指标增强
PRIMUS_LENS_WANDB_ENHANCE_METRICS=false  # 添加系统指标（默认 false）

# 工作负载标识（必需）
WORKLOAD_UID=abc-123  # 工作负载唯一标识
POD_UID=pod-456       # Pod 唯一标识
POD_NAME=training-pod-0  # Pod 名称（必需）

# 分布式训练信息
RANK=0
LOCAL_RANK=0
NODE_RANK=0
WORLD_SIZE=8
```

#### 4.1.2 安装方式

**方法 1: 使用安装脚本（推荐）**
```bash
# 下载并执行安装脚本
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh | bash

# 或者下载后执行
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh -o install.sh
chmod +x install.sh
./install.sh
```

**方法 2: 手动 pip 安装**
```bash
# 如果有本地 wheel 包或已发布到 PyPI
pip install primus-lens-wandb-exporter
# 安装后自动创建 .pth 文件，自动启用 Hook
```

**方法 3: 手动导入**
```python
# 在训练脚本最开始
import primus_lens_wandb_exporter.wandb_hook
primus_lens_wandb_exporter.wandb_hook.install_wandb_hook()

# 然后正常使用 wandb
import wandb
wandb.init(...)
wandb.log(...)
```

### 4.2 telemetry-processor 配置

#### 4.2.1 配置文件

```yaml
# config.yaml
server:
  port: 8080
  
database:
  host: postgres
  port: 5432
  database: primus_lens
  user: lens
  password: ${DB_PASSWORD}
  
wandb:
  enabled: true
  detection_enabled: true
  metrics_enabled: true
  
metrics:
  prometheus:
    enabled: true
    port: 9090
```

#### 4.2.2 路由注册

```go
// router.go
func RegisterWandBRoutes(r *gin.Engine) {
    v1 := r.Group("/api/v1")
    {
        wandb := v1.Group("/wandb")
        {
            wandb.POST("/detection", logs.ReceiveWandBDetection)
            wandb.POST("/metrics", logs.ReceiveWandBMetrics)
            wandb.POST("/logs", logs.ReceiveWandBLogs)
            wandb.POST("/batch", logs.ReceiveWandBBatch)
        }
    }
}
```

### 4.3 API 模块配置

#### 4.3.1 路由注册

```go
// router.go
func RegisterMetricsRoutes(r *gin.Engine) {
    v1 := r.Group("/api/v1")
    {
        workloads := v1.Group("/workloads")
        {
            workloads.GET("/:uid/metrics/sources", api.GetDataSources)
            workloads.GET("/:uid/metrics/available", api.GetAvailableMetrics)
            workloads.GET("/:uid/metrics/data", api.GetMetricsData)
            workloads.GET("/:uid/metrics/iteration-times", api.GetIterationTimes)
        }
    }
}
```

## 第五部分：监控和故障排查

### 5.1 日志级别

#### 5.1.1 wandb-exporter 日志

```python
# logger.py
import os

DEBUG_ENABLED = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false").lower() == "true"

def debug_log(message):
    if DEBUG_ENABLED:
        print(f"[DEBUG] {message}", file=sys.stderr)
```

**启用调试日志**:
```bash
export PRIMUS_LENS_WANDB_DEBUG=true
```

#### 5.1.2 telemetry-processor 日志

```bash
# 设置日志级别
export LOG_LEVEL=debug  # debug, info, warn, error
```

**关键日志**:
- `[WandB Detection API] Received request` - 收到检测请求
- `[WandB Metrics API] Processing metrics for %d workload(s)` - 处理指标
- `✓ WandB metrics stored to MetricsStorage: %d success` - 指标存储成功
- `✓ Updated WandB training data for workload %s` - 训练数据更新

### 5.2 常见问题

#### 5.2.1 数据未上报

**症状**: wandb-exporter 运行但数据未到达 telemetry-processor

**排查步骤**:

1. **检查 Hook 是否启用**
   ```python
   import wandb
   print(hasattr(wandb, '_primus_lens_patched'))  # 应该为 True
   ```

2. **检查环境变量**
   ```bash
   echo $PRIMUS_LENS_API_BASE_URL
   echo $POD_NAME  # 必须设置
   ```

3. **检查网络连通性**
   ```bash
   curl -X POST http://primus-lens-telemetry-processor:8080/api/v1/wandb/metrics \
     -H "Content-Type: application/json" \
     -d '{"source":"wandb","pod_name":"test","metrics":[]}'
   ```

4. **查看 wandb-exporter 日志**
   ```bash
   export PRIMUS_LENS_WANDB_DEBUG=true
   python train.py
   # 查看 stderr 输出
   ```

#### 5.2.2 WorkloadUID 解析失败

**症状**: `no valid workload found` 错误

**原因**: PodName 无法解析为 WorkloadUID

**解决方案**:
1. **直接提供 WorkloadUID**
   ```bash
   export WORKLOAD_UID=abc-123
   ```

2. **确保 Pod 在 pod_cache 中**
   ```bash
   # 检查 pod_cache 表
   SELECT * FROM pod_cache WHERE pod_name = 'training-pod-0';
   ```

#### 5.2.3 指标缺失

**症状**: 部分指标未显示

**排查**:

1. **检查字段过滤**
   ```go
   // 确认指标名不在元数据字段列表中
   var wandbMetadataFields = map[string]bool{
       "step": true,
       "run_id": true,
       "source": true,
       // ...
   }
   ```

2. **检查数据类型**
   ```python
   # wandb.log() 只支持数值类型
   wandb.log({
       "loss": 0.5,          # ✓ 支持
       "name": "training",   # ✗ 不支持，会被过滤
   })
   ```

3. **查看数据库**
   ```sql
   SELECT iteration, performance
   FROM training_performance
   WHERE workload_uid = 'abc-123' AND data_source = 'wandb'
   ORDER BY iteration DESC
   LIMIT 1;
   ```

### 5.3 性能监控

#### 5.3.1 Prometheus 指标查询

```promql
# 上报速率
rate(wandb_requests_total[5m])

# 错误率
rate(wandb_request_errors_total[5m]) / rate(wandb_requests_total[5m])

# 处理延迟（P95）
histogram_quantile(0.95, rate(wandb_request_duration_seconds_bucket[5m]))

# 数据点吞吐量
rate(wandb_metrics_data_points_total[5m])

# 存储成功率
rate(wandb_metrics_store_count[5m]) / rate(wandb_metrics_data_points_total[5m])
```

#### 5.3.2 性能调优

**wandb-exporter**:
- `batch_size`: 批量发送大小（默认 10）
- `flush_interval`: 刷新间隔秒数（默认 5.0）

```python
# 修改默认配置
reporter = AsyncAPIReporter(
    batch_size=50,      # 增加批量大小
    flush_interval=10.0 # 增加刷新间隔
)
```

**telemetry-processor**:
- 增加 worker 数量
- 启用数据库连接池
- 使用异步写入

## 附录

### A. 数据格式完整示例

#### A.1 框架检测请求

```json
{
  "source": "wandb",
  "type": "framework_detection_raw",
  "version": "1.0",
  "workload_uid": "abc-123",
  "pod_uid": "pod-456",
  "pod_name": "training-pod-0",
  "namespace": "default",
  "evidence": {
    "wandb": {
      "project": "my-training",
      "name": "run-001",
      "id": "wandb-run-id",
      "config": {
        "framework": "primus",
        "base_framework": "megatron"
      },
      "tags": ["distributed", "gpu"]
    },
    "environment": {
      "PRIMUS_CONFIG": "/config/primus.yaml",
      "PRIMUS_BACKEND": "megatron",
      "WORLD_SIZE": "8",
      "RANK": "0"
    },
    "pytorch": {
      "available": true,
      "version": "2.1.0",
      "cuda_available": true,
      "cuda_version": "12.1",
      "detected_modules": {
        "deepspeed": false,
        "megatron": true,
        "transformers": true,
        "lightning": false
      }
    },
    "wrapper_frameworks": {
      "primus": {
        "detected": true,
        "version": "1.0.0",
        "initialized": true,
        "base_framework": "megatron"
      }
    },
    "base_frameworks": {
      "megatron": {
        "detected": true,
        "version": "unknown",
        "initialized": true
      }
    }
  },
  "hints": {
    "wrapper_frameworks": ["primus"],
    "base_frameworks": ["megatron"],
    "possible_frameworks": ["primus", "megatron"],
    "confidence": "high",
    "primary_indicators": [
      "import.primus",
      "PRIMUS env vars",
      "PRIMUS_BACKEND=megatron"
    ]
  },
  "timestamp": 1704067200.123
}
```

#### A.2 指标上报请求

```json
{
  "source": "wandb",
  "workload_uid": "abc-123",
  "pod_uid": "pod-456",
  "pod_name": "training-pod-0",
  "run_id": "wandb-run-id",
  "metrics": [
    {
      "name": "loss",
      "value": 0.5234,
      "step": 100,
      "timestamp": 1704067200.123,
      "tags": {}
    },
    {
      "name": "accuracy",
      "value": 0.9512,
      "step": 100,
      "timestamp": 1704067200.123,
      "tags": {}
    },
    {
      "name": "learning_rate",
      "value": 0.0001,
      "step": 100,
      "timestamp": 1704067200.123,
      "tags": {}
    }
  ],
  "timestamp": 1704067200.123
}
```

### B. API 完整路由表

| 模块 | 方法 | 路径 | 功能 | Handler |
|------|------|------|------|---------|
| telemetry-processor | POST | `/api/v1/wandb/detection` | 框架检测上报 | `ReceiveWandBDetection` |
| telemetry-processor | POST | `/api/v1/wandb/metrics` | 指标上报 | `ReceiveWandBMetrics` |
| telemetry-processor | POST | `/api/v1/wandb/logs` | 训练数据上报 | `ReceiveWandBLogs` |
| telemetry-processor | POST | `/api/v1/wandb/batch` | 批量上报 | `ReceiveWandBBatch` |
| api | GET | `/api/v1/workloads/:uid/metrics/sources` | 获取数据源列表 | `GetDataSources` |
| api | GET | `/api/v1/workloads/:uid/metrics/available` | 获取可用指标 | `GetAvailableMetrics` |
| api | GET | `/api/v1/workloads/:uid/metrics/data` | 获取指标数据 | `GetMetricsData` |
| api | GET | `/api/v1/workloads/:uid/metrics/iteration-times` | 获取迭代时间 | `GetIterationTimes` |

### C. 支持的框架列表

#### C.1 Wrapper Frameworks（包装框架）

| 框架 | 检测方法 | 优先级 | 说明 |
|------|---------|--------|------|
| Primus | Import, ENV, Config | 最高 | 企业级训练框架 |
| PyTorch Lightning | Import, Modules | 高 | PyTorch 高级封装 |
| Transformers Trainer | Import | 中 | Hugging Face 训练器 |

#### C.2 Base Frameworks（基础框架）

| 框架 | 检测方法 | 优先级 | 说明 |
|------|---------|--------|------|
| Megatron-LM | Import, ENV, Config | 最高 | NVIDIA 大模型训练 |
| DeepSpeed | Import, ENV, Modules | 高 | Microsoft 分布式优化 |
| JAX | Import, ENV | 高 | Google ML 框架 |
| Transformers | Import, Modules | 低 | Hugging Face 模型库 |

---

**文档版本**: 1.0  
**最后更新**: 2024-12-03  
**维护者**: Primus Lens Team

