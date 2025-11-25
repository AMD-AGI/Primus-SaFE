# 快速开始：WandB Exporter API 异步上报

## 5 分钟上手

### 1. 设置环境变量

```bash
# 必需配置
export WORKLOAD_UID="my-workload-123"
export POD_UID="my-pod-456"

# 可选：框架特征（用于框架检测）
export PRIMUS_CONFIG="/config/primus.yaml"
export PRIMUS_VERSION="1.2.3"

# 可选：API 地址（默认：http://primus-lens-telemetry-processor:8080/api/v1）
export PRIMUS_LENS_API_BASE_URL="http://localhost:8080/api/v1"
```

### 2. 运行训练代码（无需修改）

```python
import wandb

# 正常使用 wandb - Primus Lens 会自动劫持
run = wandb.init(
    project="my-project",
    config={"framework": "primus"}
)

# 训练循环
for step in range(100):
    wandb.log({"loss": 0.5, "accuracy": 0.9}, step=step)

wandb.finish()
```

### 3. 自动发生的事情

✅ **框架检测数据采集**（`wandb.init()` 时）
- 采集环境变量（PRIMUS_CONFIG 等）
- 采集 WandB 配置
- 采集 PyTorch 信息
- 生成预判断 hints
- **异步上报到** `POST /api/v1/wandb/detection`

✅ **训练指标上报**（`wandb.log()` 时）
- 提取指标数据
- 入队（非阻塞）
- 批量处理
- **异步上报到** `POST /api/v1/wandb/metrics`

✅ **程序退出时**
- 自动刷新所有待处理数据
- 确保数据不丢失

## 输出示例

```
[Primus Lens WandB] Installing WandB hook...
[Primus Lens WandB] API reporting enabled
[Primus Lens API Reporter] Started (API: http://localhost:8080/api/v1)
[Primus Lens WandB] Intercepted wandb.init()
[Primus Lens WandB] WandB run initialized: my-run
[Primus Lens WandB] Framework detection data queued for reporting
  Detected frameworks: ['primus']
  Confidence: high
...
[Primus Lens WandB] Cleaning up...
[Primus Lens API Reporter] Stopped. Stats: {'detection_sent': 1, 'metrics_sent': 100, 'errors': 0}
```

## 在 Kubernetes 中使用

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: training-pod
spec:
  containers:
  - name: training
    image: your-training-image:latest
    env:
    # 由 Adapter 自动注入
    - name: WORKLOAD_UID
      value: "workload-abc-123"
    - name: POD_UID
      valueFrom:
        fieldRef:
          fieldPath: metadata.uid
    
    # 框架特征
    - name: FRAMEWORK
      value: "primus"
    - name: PRIMUS_CONFIG
      value: "/workspace/config.yaml"
```

## 验证上报成功

### 检查 wandb-exporter 日志

```bash
# 查看统计信息
grep "API Reporter.*Stats" your-training-log.txt
```

### 查询 telemetry-processor

```bash
# 查询框架检测结果
curl http://telemetry-processor:8080/api/v1/workloads/${WORKLOAD_UID}/framework-detection
```

### 查看 telemetry-processor 日志

```bash
kubectl logs -f deployment/primus-lens-telemetry-processor | grep WandB
```

应该看到：
```
Processing WandB detection for workload workload-123
✓ Detected framework from WandB: primus (confidence: 0.80)
Processing WandB metrics: 10 metrics
```

## 故障排查

### 问题：没有看到 "API reporting enabled"

**原因**：API 上报未启用

**解决**：
```bash
export PRIMUS_LENS_WANDB_API_REPORTING="true"
```

### 问题：报错 "WORKLOAD_UID not set"

**原因**：缺少必需环境变量

**解决**：
```bash
export WORKLOAD_UID="your-workload-uid"
export POD_UID="your-pod-uid"
```

### 问题：数据没有上报到服务器

**检查**：
1. API 地址是否正确：`echo $PRIMUS_LENS_API_BASE_URL`
2. telemetry-processor 服务是否运行
3. 网络连接是否正常

**调试**：
```bash
# 测试 API 连接
curl -X POST http://localhost:8080/api/v1/wandb/detection \
  -H "Content-Type: application/json" \
  -d '{"test": "data"}'
```

## 更多文档

- **[API_REPORTING.md](API_REPORTING.md)** - 完整的 API 上报文档
- **[example_api_reporting.py](example_api_reporting.py)** - 示例代码
- **[README.md](README.md)** - 主文档

## 核心特性

✅ 零代码修改  
✅ 异步上报（不阻塞训练）  
✅ 自动批量处理  
✅ 失败容忍（不影响训练）  
✅ 多源证据采集  
✅ 智能预判断  

开始使用吧！🚀

