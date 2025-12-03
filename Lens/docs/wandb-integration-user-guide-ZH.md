# WandB 集成用户使用指南

## 概述

本指南面向希望在训练任务中使用 WandB 进行指标追踪的用户。Primus Lens 提供了对 WandB 的完整支持，让您可以继续使用熟悉的 WandB API，同时自动将训练指标同步到 Lens 系统进行可视化和分析。

## 核心特性

✅ **零代码修改** - 无需改变现有训练代码  
✅ **自动拦截** - 自动捕获 `wandb.init()` 和 `wandb.log()` 调用  
✅ **框架检测** - 自动识别训练框架（Primus, Megatron, DeepSpeed 等）  
✅ **指标同步** - 训练指标自动同步到 Lens 系统  
✅ **分布式支持** - 完美支持多节点、多卡训练  
✅ **实时可视化** - 在 Lens UI 和 Grafana 中实时查看指标

## 快速开始

### 第一步：安装 WandB Exporter

在训练环境中安装 `primus-lens-wandb-exporter`：

```bash
# 方法 1: 使用安装脚本（推荐）
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh | bash

# 方法 2: 如果无法访问外网，先下载脚本再执行
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh -o install.sh
chmod +x install.sh
./install.sh

# 方法 3: 如果有本地包或 PyPI 发布版本
pip install primus-lens-wandb-exporter
```

**就这么简单！** 安装后会自动启用 WandB 拦截功能。

### 第二步：配置环境变量

在训练 Job 的配置文件中添加必要的环境变量：

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: my-training-job
spec:
  template:
    spec:
      containers:
      - name: training
        image: my-training-image:latest
        env:
        # ===== 必需的环境变量 =====
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        
        # ===== 推荐的环境变量 =====
        - name: WORKLOAD_UID
          value: "my-training-workload-12345"
        
        # ===== Lens API 地址（如果默认值不对）=====
        - name: PRIMUS_LENS_API_BASE_URL
          value: "http://primus-lens-telemetry-processor:8080/api/v1"
        
        # ===== 可选：功能开关（默认都是 true，可以显式设置）=====
        - name: PRIMUS_LENS_WANDB_HOOK
          value: "true"  # 启用 WandB Hook 拦截
        - name: PRIMUS_LENS_WANDB_API_REPORTING
          value: "true"  # 启用 API 上报
        
        # ===== 可选：调试开关 =====
        - name: PRIMUS_LENS_WANDB_DEBUG
          value: "false"
```

### 第三步：正常使用 WandB

在训练代码中，继续使用标准的 WandB API，无需任何修改：

```python
import wandb

# 初始化 WandB（会被自动拦截）
run = wandb.init(
    project="my-awesome-project",
    name="experiment-001",
    config={
        "learning_rate": 0.001,
        "batch_size": 32,
        "epochs": 100,
    }
)

# 训练循环
for epoch in range(100):
    for batch in dataloader:
        loss = train_step(batch)
        accuracy = evaluate()
        
        # 记录指标（会被自动拦截并同步到 Lens）
        wandb.log({
            "loss": loss,
            "accuracy": accuracy,
            "learning_rate": scheduler.get_lr()[0],
        })

# 结束训练
wandb.finish()
```

**就是这么简单！** 您的训练指标会自动同步到 Lens 系统。

## 查看训练指标

### 方式一：通过 Lens UI 查看

1. 登录 Lens Web UI
2. 导航到 **Workloads** > **Your Workload**
3. 点击 **Metrics** 标签页
4. 选择数据源：**WandB**
5. 查看实时训练指标图表

### 方式二：通过 Grafana 查看

如果您的集群配置了 Grafana：

1. 登录 Grafana
2. 选择 **Primus Lens Metrics** 数据源
3. 创建新的 Dashboard
4. 添加 Panel，选择您的 Workload 和指标

**示例查询**:
```
Workload UID: my-training-workload-12345
Data Source: wandb
Metrics: loss, accuracy, learning_rate
```

### 方式三：通过 API 查询

您也可以直接调用 Lens API 获取指标数据：

```bash
# 获取可用的指标列表
curl "http://lens-api:8080/api/v1/workloads/my-training-workload-12345/metrics/available?data_source=wandb"

# 获取指标数据
curl "http://lens-api:8080/api/v1/workloads/my-training-workload-12345/metrics/data?data_source=wandb&metrics=loss,accuracy"
```

## 常见使用场景

### 场景 1：单机训练

最简单的场景，单机单卡或单机多卡训练：

```python
# train.py
import wandb
import torch

def main():
    # 初始化 WandB
    wandb.init(project="my-project", name="single-gpu-run")
    
    # 训练
    model = MyModel()
    for epoch in range(100):
        loss = train_epoch(model)
        wandb.log({"loss": loss, "epoch": epoch})
    
    wandb.finish()

if __name__ == "__main__":
    main()
```

**启动命令**:
```bash
export POD_NAME="training-pod-0"
export WORKLOAD_UID="single-gpu-training-001"
python train.py
```

### 场景 2：分布式训练（单节点多卡）

使用 `torch.distributed` 或 `torchrun` 进行单节点多卡训练：

```python
# train_ddp.py
import wandb
import torch
import torch.distributed as dist

def main():
    # 初始化分布式环境
    dist.init_process_group(backend="nccl")
    rank = dist.get_rank()
    
    # 只在 rank 0 初始化 WandB
    if rank == 0:
        wandb.init(project="my-project", name="ddp-training")
    
    # 训练
    model = MyModel().to(rank)
    model = torch.nn.parallel.DistributedDataParallel(model, device_ids=[rank])
    
    for epoch in range(100):
        loss = train_epoch(model, rank)
        
        # 只在 rank 0 记录指标
        if rank == 0:
            wandb.log({"loss": loss, "epoch": epoch})
    
    if rank == 0:
        wandb.finish()

if __name__ == "__main__":
    main()
```

**启动命令**:
```bash
export POD_NAME="training-pod-0"
export WORKLOAD_UID="ddp-training-001"
export WORLD_SIZE=8
export RANK=0
export LOCAL_RANK=0

torchrun --nproc_per_node=8 train_ddp.py
```

### 场景 3：多节点分布式训练

跨多个节点的大规模分布式训练：

```python
# train_multinode.py
import wandb
import torch
import torch.distributed as dist
import os

def main():
    # 初始化分布式环境
    dist.init_process_group(backend="nccl")
    
    rank = int(os.environ.get("RANK", 0))
    local_rank = int(os.environ.get("LOCAL_RANK", 0))
    world_size = int(os.environ.get("WORLD_SIZE", 1))
    
    # 只在全局 rank 0 初始化 WandB
    if rank == 0:
        wandb.init(
            project="multi-node-training",
            name=f"nodes-{world_size//8}-gpus-{world_size}"
        )
    
    # 训练
    model = MyModel().to(local_rank)
    model = torch.nn.parallel.DistributedDataParallel(
        model, device_ids=[local_rank]
    )
    
    for epoch in range(100):
        loss = train_epoch(model, local_rank)
        
        if rank == 0:
            wandb.log({
                "loss": loss,
                "epoch": epoch,
                "total_gpus": world_size,
            })
    
    if rank == 0:
        wandb.finish()

if __name__ == "__main__":
    main()
```

**Kubernetes Job 配置**:
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: multi-node-training
spec:
  parallelism: 4  # 4 个节点
  completions: 1
  template:
    spec:
      containers:
      - name: training
        image: my-training-image:latest
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: WORKLOAD_UID
          value: "multi-node-training-001"
        - name: WORLD_SIZE
          value: "32"  # 4 nodes × 8 GPUs
        - name: MASTER_ADDR
          value: "master-0.training-service"
        - name: MASTER_PORT
          value: "29500"
        # RANK 和 LOCAL_RANK 由启动脚本设置
```

### 场景 4：使用 PyTorch Lightning

如果您使用 PyTorch Lightning 框架：

```python
# train_lightning.py
import wandb
import pytorch_lightning as pl
from pytorch_lightning.loggers import WandbLogger

class MyLightningModule(pl.LightningModule):
    def __init__(self):
        super().__init__()
        self.model = MyModel()
    
    def training_step(self, batch, batch_idx):
        loss = self.model(batch)
        self.log("train_loss", loss)  # 自动记录到 WandB
        return loss
    
    def validation_step(self, batch, batch_idx):
        loss = self.model(batch)
        self.log("val_loss", loss)
        return loss

def main():
    # 创建 WandB Logger
    wandb_logger = WandbLogger(
        project="lightning-project",
        name="lightning-run"
    )
    
    # 创建 Trainer
    trainer = pl.Trainer(
        max_epochs=100,
        logger=wandb_logger,
        accelerator="gpu",
        devices=8,
        strategy="ddp"
    )
    
    # 训练
    model = MyLightningModule()
    trainer.fit(model)

if __name__ == "__main__":
    main()
```

**Lightning 会自动调用 `wandb.log()`，因此指标会自动同步到 Lens。**

### 场景 5：使用 Primus 框架

如果您使用 Primus 企业级训练框架：

```python
# train_primus.py
import wandb
from primus import Trainer, TrainerConfig

def main():
    # Primus 会自动设置分布式环境
    config = TrainerConfig.from_file("primus_config.yaml")
    
    # 初始化 WandB（只在 master rank）
    if config.is_master:
        wandb.init(
            project="primus-training",
            name=f"primus-{config.experiment_name}",
            config={
                "framework": "primus",
                "base_framework": config.backend,  # megatron, deepspeed, etc.
            }
        )
    
    # 创建 Trainer
    trainer = Trainer(config)
    
    # 注册指标回调
    def log_metrics(metrics):
        if config.is_master:
            wandb.log(metrics)
    
    trainer.register_callback("on_step_end", log_metrics)
    
    # 训练
    trainer.train()
    
    if config.is_master:
        wandb.finish()

if __name__ == "__main__":
    main()
```

**Primus 会自动检测为包装框架，Lens 会识别底层的基础框架（如 Megatron）。**

## 高级配置

### 配置选项

所有配置通过环境变量控制：

#### 必需配置

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `POD_NAME` | Pod 名称（必需） | `training-pod-0` |

#### 推荐配置

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `WORKLOAD_UID` | 工作负载唯一标识 | 无（从 PodName 解析） |
| `PRIMUS_LENS_API_BASE_URL` | Lens API 地址 | `http://primus-lens-telemetry-processor:8080/api/v1` |

#### 可选配置

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `PRIMUS_LENS_WANDB_HOOK` | 启用/禁用 Hook | `true` |
| `PRIMUS_LENS_WANDB_API_REPORTING` | 启用/禁用 API 上报 | `true` |
| `PRIMUS_LENS_WANDB_SAVE_LOCAL` | 启用/禁用本地保存 | `true` |
| `PRIMUS_LENS_WANDB_OUTPUT_PATH` | 本地保存路径 | 无（不保存） |
| `PRIMUS_LENS_WANDB_ENHANCE_METRICS` | 添加系统指标（CPU/GPU） | `false` |
| `PRIMUS_LENS_WANDB_DEBUG` | 启用调试日志 | `false` |

#### 分布式训练配置

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `RANK` | 全局 rank | `0` |
| `LOCAL_RANK` | 节点内 rank | `0` |
| `NODE_RANK` | 节点 rank | `0` |
| `WORLD_SIZE` | 总进程数 | `8` |

### 本地文件保存

如果您希望同时将指标保存到本地文件（用于备份或离线分析）：

```yaml
env:
- name: PRIMUS_LENS_WANDB_SAVE_LOCAL
  value: "true"
- name: PRIMUS_LENS_WANDB_OUTPUT_PATH
  value: "/mnt/training-output"

# 挂载持久卷
volumeMounts:
- name: output-volume
  mountPath: /mnt/training-output
```

指标将保存为 JSONL 格式：
```
/mnt/training-output/
  node_0/
    rank_0/
      wandb_metrics.jsonl
    rank_1/
      wandb_metrics.jsonl
  node_1/
    rank_0/
      wandb_metrics.jsonl
    ...
```

### 添加系统指标

启用系统指标增强，自动添加 CPU、内存、GPU 使用率：

```yaml
env:
- name: PRIMUS_LENS_WANDB_ENHANCE_METRICS
  value: "true"
```

**注意**: 如果需要 GPU 指标支持，需要额外安装依赖：
```bash
# 在已安装 wandb-exporter 的基础上，安装 GPU 支持
pip install nvidia-ml-py3>=7.352.0
```

自动添加的指标：
- `_primus_sys_cpu_percent`: CPU 使用率
- `_primus_sys_memory_percent`: 内存使用率
- `_primus_gpu_0_util`: GPU 0 使用率
- `_primus_gpu_0_mem_used_mb`: GPU 0 显存使用（MB）
- ... （每个 GPU 都有对应指标）

### 调试模式

遇到问题时，启用调试模式查看详细日志：

```yaml
env:
- name: PRIMUS_LENS_WANDB_DEBUG
  value: "true"
```

调试日志会输出到 stderr，包括：
- WandB Hook 安装状态
- 拦截的 `wandb.init()` 和 `wandb.log()` 调用
- 框架检测结果
- API 上报状态
- 错误详情

## 常见问题

### Q1: 安装后指标没有出现在 Lens 中？

**排查步骤**:

1. **检查安装**
   ```python
   import wandb
   print(hasattr(wandb, '_primus_lens_patched'))
   # 应该输出: True
   ```

2. **检查环境变量**
   ```bash
   echo $POD_NAME
   # 必须有值
   ```

3. **启用调试模式**
   ```bash
   export PRIMUS_LENS_WANDB_DEBUG=true
   python train.py 2>&1 | grep "Primus Lens"
   ```

4. **检查网络连通性**
   ```bash
   # 在训练 Pod 中测试
   curl http://primus-lens-telemetry-processor:8080/health
   ```

### Q2: 只看到部分指标？

**可能原因**:

1. **非数值指标被过滤**
   ```python
   # ✓ 支持
   wandb.log({"loss": 0.5, "accuracy": 0.95})
   
   # ✗ 不支持（非数值类型）
   wandb.log({"model_name": "bert-large", "status": "running"})
   ```

2. **元数据字段被过滤**
   - `step`, `run_id`, `source`, `history`, `created_at`, `updated_at` 是保留字段
   - 避免使用这些名称作为指标名

### Q3: 分布式训练时指标重复？

**解决方案**: 确保只在 rank 0 记录指标

```python
import torch.distributed as dist

def should_log():
    if not dist.is_initialized():
        return True  # 单卡训练
    return dist.get_rank() == 0  # 只在 rank 0 记录

# 使用
if should_log():
    wandb.log({"loss": loss})
```

### Q4: 如何查看历史训练数据？

**通过 API 查询历史数据**:

```bash
# 获取特定时间范围的数据
curl "http://lens-api:8080/api/v1/workloads/my-workload/metrics/data?\
data_source=wandb&\
metrics=loss,accuracy&\
start=1704067200000&\
end=1704153600000"
```

**在 Grafana 中设置时间范围**:
- 使用 Grafana 的时间选择器
- 选择相对时间（如 "Last 24 hours"）或绝对时间

### Q5: 如何禁用 Lens 同步但保留 WandB 功能？

如果您只想使用 WandB 原生功能，不同步到 Lens：

```bash
# 方法 1: 完全禁用 Hook
export PRIMUS_LENS_WANDB_HOOK=false

# 方法 2: 只禁用 API 上报
export PRIMUS_LENS_WANDB_API_REPORTING=false

# 方法 3: 卸载 exporter
pip uninstall primus-lens-wandb-exporter
```

### Q6: 训练日志中有很多 WandB Hook 相关输出？

**减少日志输出**:

```bash
# 禁用调试模式（如果之前启用）
export PRIMUS_LENS_WANDB_DEBUG=false

# 或者在代码中过滤日志
import logging
logging.getLogger("primus_lens_wandb_exporter").setLevel(logging.WARNING)
```

### Q7: 多个实验共享相同的 WorkloadUID？

**区分不同实验**:

```python
# 方法 1: 使用不同的 WandB run name
wandb.init(
    project="my-project",
    name=f"exp-{experiment_id}-{timestamp}"
)

# 方法 2: 在 Kubernetes 中为每个 Job 设置不同的 WORKLOAD_UID
env:
- name: WORKLOAD_UID
  value: "training-exp-001-$(date +%s)"
```

### Q8: 如何在 Jupyter Notebook 中使用？

**Notebook 中的使用**:

```python
# 在 Notebook 最开始的 cell
import os
os.environ["POD_NAME"] = "jupyter-notebook-pod"
os.environ["WORKLOAD_UID"] = "jupyter-experiment-001"

# 如果需要，启用调试
os.environ["PRIMUS_LENS_WANDB_DEBUG"] = "true"

# 导入并安装 Hook
import primus_lens_wandb_exporter.wandb_hook
primus_lens_wandb_exporter.wandb_hook.install_wandb_hook()

# 正常使用 WandB
import wandb
wandb.init(project="notebook-project")
wandb.log({"loss": 0.5})
```

### Q9: 如何验证数据已成功上报？

**验证方法**:

1. **查看调试日志**
   ```bash
   export PRIMUS_LENS_WANDB_DEBUG=true
   python train.py 2>&1 | grep "✓"
   # 应该看到:
   # ✓ Detection data queued for reporting
   # ✓ Metrics data queued for reporting
   ```

2. **查询 API**
   ```bash
   # 检查数据源
   curl "http://lens-api:8080/api/v1/workloads/my-workload/metrics/sources"
   
   # 应该包含 wandb
   {
     "data_sources": [
       {"name": "wandb", "count": 1500}
     ]
   }
   ```

3. **检查数据库**（需要数据库访问权限）
   ```sql
   SELECT COUNT(*) 
   FROM training_performance 
   WHERE workload_uid = 'my-workload' 
   AND data_source = 'wandb';
   ```

### Q10: 支持哪些训练框架？

**当前支持的框架**:

**包装框架（Wrapper Frameworks）**:
- ✅ Primus（企业级训练框架）
- ✅ PyTorch Lightning
- ✅ Hugging Face Trainer

**基础框架（Base Frameworks）**:
- ✅ Megatron-LM
- ✅ DeepSpeed
- ✅ JAX
- ✅ Transformers
- ✅ PyTorch（原生）
- ✅ TensorFlow（部分支持）

**框架自动检测**:
- 系统会自动识别您使用的框架
- 支持双层框架检测（如 Primus + Megatron）
- 无需手动配置

## 系统要求

### 软件依赖

| 组件 | 最低版本 | 推荐版本 |
|------|---------|---------|
| Python | 3.7+ | 3.9+ |
| wandb | 0.12.0+ | 最新版 |
| psutil | 5.8.0+ | 最新版（用于系统指标） |
| nvidia-ml-py3 | 7.352.0+ | 最新版（GPU 指标，可选） |

### 环境要求

- Kubernetes 集群（推荐）或独立服务器
- 可访问 Lens telemetry-processor 服务
- 网络带宽：建议 ≥ 100 Mbps（大规模训练）
- 存储：本地保存模式下需要足够的持久卷空间

## 获取帮助

### 文档资源

- **技术文档**: `docs/wandb-integration-technical.md`
- **API 文档**: `http://lens-api:8080/api/docs`
- **Lens 用户指南**: `docs/user-guide.md`

### 查看日志

```bash
# 训练 Pod 日志
kubectl logs <pod-name> --tail=100 -f

# Telemetry Processor 日志
kubectl logs -l app=primus-lens-telemetry-processor --tail=100 -f

# API 服务日志
kubectl logs -l app=primus-lens-api --tail=100 -f
```

### 联系支持

如果遇到问题，请提供以下信息：
- 训练环境（单机/分布式，GPU 数量）
- 使用的框架和版本
- 错误日志（启用 `PRIMUS_LENS_WANDB_DEBUG=true`）
- 训练代码片段（可选）

## 语言版本

- [中文文档](./wandb-integration-user-guide-ZH.md)（当前）
- [English Documentation](./wandb-integration-user-guide.md)

---

**祝训练愉快！** 🚀

**文档版本**: 1.0  
**最后更新**: 2024-12-03  
**维护者**: Primus Lens Team

