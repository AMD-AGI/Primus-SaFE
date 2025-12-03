# WandB Integration User Guide

## Overview

This guide is for users who want to use WandB for metric tracking in training tasks. Primus Lens provides full support for WandB, allowing you to continue using the familiar WandB API while automatically synchronizing training metrics to the Lens system for visualization and analysis.

## Core Features

✅ **Zero Code Changes** - No need to modify existing training code  
✅ **Auto-Interception** - Automatically captures `wandb.init()` and `wandb.log()` calls  
✅ **Framework Detection** - Automatically identifies training frameworks (Primus, Megatron, DeepSpeed, etc.)  
✅ **Metrics Sync** - Training metrics automatically synced to Lens system  
✅ **Distributed Support** - Perfect support for multi-node, multi-GPU training  
✅ **Real-time Visualization** - View metrics in real-time in Lens UI and Grafana

## Quick Start

### Step 1: Install WandB Exporter

Install `primus-lens-wandb-exporter` in your training environment:

```bash
# Method 1: Using installation script (recommended)
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh | bash

# Method 2: If you cannot access external networks, download the script first then execute
curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/refs/heads/feature/training-tracing/Lens/modules/exporters/wandb-exporter/install.sh -o install.sh
chmod +x install.sh
./install.sh

# Method 3: If you have a local package or PyPI release version
pip install primus-lens-wandb-exporter
```

**That's it!** After installation, WandB interception functionality is automatically enabled.

### Step 2: Configure Environment Variables

Add necessary environment variables to your training Job configuration file:

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
        # ===== Required environment variables =====
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        
        # ===== Recommended environment variables =====
        - name: WORKLOAD_UID
          value: "my-training-workload-12345"
        
        # ===== Lens API address (if default is incorrect) =====
        - name: PRIMUS_LENS_API_BASE_URL
          value: "http://primus-lens-telemetry-processor:8080/api/v1"
        
        # ===== Optional: Feature toggles (all default to true, can be explicitly set) =====
        - name: PRIMUS_LENS_WANDB_HOOK
          value: "true"  # Enable WandB Hook interception
        - name: PRIMUS_LENS_WANDB_API_REPORTING
          value: "true"  # Enable API reporting
        
        # ===== Optional: Debug toggle =====
        - name: PRIMUS_LENS_WANDB_DEBUG
          value: "false"
```

### Step 3: Use WandB Normally

In your training code, continue using the standard WandB API without any modifications:

```python
import wandb

# Initialize WandB (will be automatically intercepted)
run = wandb.init(
    project="my-awesome-project",
    name="experiment-001",
    config={
        "learning_rate": 0.001,
        "batch_size": 32,
        "epochs": 100,
    }
)

# Training loop
for epoch in range(100):
    for batch in dataloader:
        loss = train_step(batch)
        accuracy = evaluate()
        
        # Log metrics (will be automatically intercepted and synced to Lens)
        wandb.log({
            "loss": loss,
            "accuracy": accuracy,
            "learning_rate": scheduler.get_lr()[0],
        })

# Finish training
wandb.finish()
```

**It's that simple!** Your training metrics will be automatically synced to the Lens system.

## Viewing Training Metrics

### Method 1: View Through Lens UI

1. Log in to Lens Web UI
2. Navigate to **Workloads** > **Your Workload**
3. Click the **Metrics** tab
4. Select data source: **WandB**
5. View real-time training metric charts

### Method 2: View Through Grafana

If your cluster has Grafana configured:

1. Log in to Grafana
2. Select **Primus Lens Metrics** data source
3. Create a new Dashboard
4. Add a Panel, select your Workload and metrics

**Example Query**:
```
Workload UID: my-training-workload-12345
Data Source: wandb
Metrics: loss, accuracy, learning_rate
```

### Method 3: Query Through API

You can also directly call the Lens API to get metric data:

```bash
# Get available metrics list
curl "http://lens-api:8080/api/v1/workloads/my-training-workload-12345/metrics/available?data_source=wandb"

# Get metrics data
curl "http://lens-api:8080/api/v1/workloads/my-training-workload-12345/metrics/data?data_source=wandb&metrics=loss,accuracy"
```

## Common Usage Scenarios

### Scenario 1: Single Machine Training

The simplest scenario, single-machine single-GPU or single-machine multi-GPU training:

```python
# train.py
import wandb
import torch

def main():
    # Initialize WandB
    wandb.init(project="my-project", name="single-gpu-run")
    
    # Training
    model = MyModel()
    for epoch in range(100):
        loss = train_epoch(model)
        wandb.log({"loss": loss, "epoch": epoch})
    
    wandb.finish()

if __name__ == "__main__":
    main()
```

**Launch Command**:
```bash
export POD_NAME="training-pod-0"
export WORKLOAD_UID="single-gpu-training-001"
python train.py
```

### Scenario 2: Distributed Training (Single Node Multi-GPU)

Using `torch.distributed` or `torchrun` for single-node multi-GPU training:

```python
# train_ddp.py
import wandb
import torch
import torch.distributed as dist

def main():
    # Initialize distributed environment
    dist.init_process_group(backend="nccl")
    rank = dist.get_rank()
    
    # Only initialize WandB on rank 0
    if rank == 0:
        wandb.init(project="my-project", name="ddp-training")
    
    # Training
    model = MyModel().to(rank)
    model = torch.nn.parallel.DistributedDataParallel(model, device_ids=[rank])
    
    for epoch in range(100):
        loss = train_epoch(model, rank)
        
        # Only log metrics on rank 0
        if rank == 0:
            wandb.log({"loss": loss, "epoch": epoch})
    
    if rank == 0:
        wandb.finish()

if __name__ == "__main__":
    main()
```

**Launch Command**:
```bash
export POD_NAME="training-pod-0"
export WORKLOAD_UID="ddp-training-001"
export WORLD_SIZE=8
export RANK=0
export LOCAL_RANK=0

torchrun --nproc_per_node=8 train_ddp.py
```

### Scenario 3: Multi-Node Distributed Training

Large-scale distributed training across multiple nodes:

```python
# train_multinode.py
import wandb
import torch
import torch.distributed as dist
import os

def main():
    # Initialize distributed environment
    dist.init_process_group(backend="nccl")
    
    rank = int(os.environ.get("RANK", 0))
    local_rank = int(os.environ.get("LOCAL_RANK", 0))
    world_size = int(os.environ.get("WORLD_SIZE", 1))
    
    # Only initialize WandB on global rank 0
    if rank == 0:
        wandb.init(
            project="multi-node-training",
            name=f"nodes-{world_size//8}-gpus-{world_size}"
        )
    
    # Training
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

**Kubernetes Job Configuration**:
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: multi-node-training
spec:
  parallelism: 4  # 4 nodes
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
        # RANK and LOCAL_RANK are set by launch script
```

### Scenario 4: Using PyTorch Lightning

If you use the PyTorch Lightning framework:

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
        self.log("train_loss", loss)  # Automatically logged to WandB
        return loss
    
    def validation_step(self, batch, batch_idx):
        loss = self.model(batch)
        self.log("val_loss", loss)
        return loss

def main():
    # Create WandB Logger
    wandb_logger = WandbLogger(
        project="lightning-project",
        name="lightning-run"
    )
    
    # Create Trainer
    trainer = pl.Trainer(
        max_epochs=100,
        logger=wandb_logger,
        accelerator="gpu",
        devices=8,
        strategy="ddp"
    )
    
    # Training
    model = MyLightningModule()
    trainer.fit(model)

if __name__ == "__main__":
    main()
```

**Lightning automatically calls `wandb.log()`, so metrics are automatically synced to Lens.**

### Scenario 5: Using Primus Framework

If you use the Primus enterprise training framework:

```python
# train_primus.py
import wandb
from primus import Trainer, TrainerConfig

def main():
    # Primus automatically sets up distributed environment
    config = TrainerConfig.from_file("primus_config.yaml")
    
    # Initialize WandB (only on master rank)
    if config.is_master:
        wandb.init(
            project="primus-training",
            name=f"primus-{config.experiment_name}",
            config={
                "framework": "primus",
                "base_framework": config.backend,  # megatron, deepspeed, etc.
            }
        )
    
    # Create Trainer
    trainer = Trainer(config)
    
    # Register metrics callback
    def log_metrics(metrics):
        if config.is_master:
            wandb.log(metrics)
    
    trainer.register_callback("on_step_end", log_metrics)
    
    # Training
    trainer.train()
    
    if config.is_master:
        wandb.finish()

if __name__ == "__main__":
    main()
```

**Primus is automatically detected as a wrapper framework, and Lens will identify the underlying base framework (such as Megatron).**

## Advanced Configuration

### Configuration Options

All configuration is controlled through environment variables:

#### Required Configuration

| Environment Variable | Description | Example |
|---------|------|------|
| `POD_NAME` | Pod name (required) | `training-pod-0` |

#### Recommended Configuration

| Environment Variable | Description | Default Value |
|---------|------|--------|
| `WORKLOAD_UID` | Workload unique identifier | None (parsed from PodName) |
| `PRIMUS_LENS_API_BASE_URL` | Lens API address | `http://primus-lens-telemetry-processor:8080/api/v1` |

#### Optional Configuration

| Environment Variable | Description | Default Value |
|---------|------|--------|
| `PRIMUS_LENS_WANDB_HOOK` | Enable/disable Hook | `true` |
| `PRIMUS_LENS_WANDB_API_REPORTING` | Enable/disable API reporting | `true` |
| `PRIMUS_LENS_WANDB_SAVE_LOCAL` | Enable/disable local saving | `true` |
| `PRIMUS_LENS_WANDB_OUTPUT_PATH` | Local save path | None (no saving) |
| `PRIMUS_LENS_WANDB_ENHANCE_METRICS` | Add system metrics (CPU/GPU) | `false` |
| `PRIMUS_LENS_WANDB_DEBUG` | Enable debug logging | `false` |

#### Distributed Training Configuration

| Environment Variable | Description | Example |
|---------|------|------|
| `RANK` | Global rank | `0` |
| `LOCAL_RANK` | Node-local rank | `0` |
| `NODE_RANK` | Node rank | `0` |
| `WORLD_SIZE` | Total number of processes | `8` |

### Local File Saving

If you want to also save metrics to local files (for backup or offline analysis):

```yaml
env:
- name: PRIMUS_LENS_WANDB_SAVE_LOCAL
  value: "true"
- name: PRIMUS_LENS_WANDB_OUTPUT_PATH
  value: "/mnt/training-output"

# Mount persistent volume
volumeMounts:
- name: output-volume
  mountPath: /mnt/training-output
```

Metrics will be saved in JSONL format:
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

### Adding System Metrics

Enable system metrics enhancement to automatically add CPU, memory, GPU utilization:

```yaml
env:
- name: PRIMUS_LENS_WANDB_ENHANCE_METRICS
  value: "true"
```

**Note**: If you need GPU metrics support, you need to install additional dependencies:
```bash
# On top of the installed wandb-exporter, install GPU support
pip install nvidia-ml-py3>=7.352.0
```

Automatically added metrics:
- `_primus_sys_cpu_percent`: CPU utilization
- `_primus_sys_memory_percent`: Memory utilization
- `_primus_gpu_0_util`: GPU 0 utilization
- `_primus_gpu_0_mem_used_mb`: GPU 0 memory usage (MB)
- ... (each GPU has corresponding metrics)

### Debug Mode

When encountering issues, enable debug mode to view detailed logs:

```yaml
env:
- name: PRIMUS_LENS_WANDB_DEBUG
  value: "true"
```

Debug logs will be output to stderr, including:
- WandB Hook installation status
- Intercepted `wandb.init()` and `wandb.log()` calls
- Framework detection results
- API reporting status
- Error details

## FAQ

### Q1: Metrics not appearing in Lens after installation?

**Troubleshooting Steps**:

1. **Check installation**
   ```python
   import wandb
   print(hasattr(wandb, '_primus_lens_patched'))
   # Should output: True
   ```

2. **Check environment variables**
   ```bash
   echo $POD_NAME
   # Must have a value
   ```

3. **Enable debug mode**
   ```bash
   export PRIMUS_LENS_WANDB_DEBUG=true
   python train.py 2>&1 | grep "Primus Lens"
   ```

4. **Check network connectivity**
   ```bash
   # Test from training Pod
   curl http://primus-lens-telemetry-processor:8080/health
   ```

### Q2: Only seeing some metrics?

**Possible Causes**:

1. **Non-numeric metrics are filtered**
   ```python
   # ✓ Supported
   wandb.log({"loss": 0.5, "accuracy": 0.95})
   
   # ✗ Not supported (non-numeric types)
   wandb.log({"model_name": "bert-large", "status": "running"})
   ```

2. **Metadata fields are filtered**
   - `step`, `run_id`, `source`, `history`, `created_at`, `updated_at` are reserved fields
   - Avoid using these names as metric names

### Q3: Duplicate metrics in distributed training?

**Solution**: Ensure only rank 0 logs metrics

```python
import torch.distributed as dist

def should_log():
    if not dist.is_initialized():
        return True  # Single GPU training
    return dist.get_rank() == 0  # Only log on rank 0

# Usage
if should_log():
    wandb.log({"loss": loss})
```

### Q4: How to view historical training data?

**Query historical data through API**:

```bash
# Get data for specific time range
curl "http://lens-api:8080/api/v1/workloads/my-workload/metrics/data?\
data_source=wandb&\
metrics=loss,accuracy&\
start=1704067200000&\
end=1704153600000"
```

**Set time range in Grafana**:
- Use Grafana's time picker
- Select relative time (e.g., "Last 24 hours") or absolute time

### Q5: How to disable Lens sync but keep WandB functionality?

If you only want to use WandB's native functionality without syncing to Lens:

```bash
# Method 1: Completely disable Hook
export PRIMUS_LENS_WANDB_HOOK=false

# Method 2: Only disable API reporting
export PRIMUS_LENS_WANDB_API_REPORTING=false

# Method 3: Uninstall exporter
pip uninstall primus-lens-wandb-exporter
```

### Q6: Too much WandB Hook related output in training logs?

**Reduce log output**:

```bash
# Disable debug mode (if previously enabled)
export PRIMUS_LENS_WANDB_DEBUG=false

# Or filter logs in code
import logging
logging.getLogger("primus_lens_wandb_exporter").setLevel(logging.WARNING)
```

### Q7: Multiple experiments sharing the same WorkloadUID?

**Differentiate experiments**:

```python
# Method 1: Use different WandB run names
wandb.init(
    project="my-project",
    name=f"exp-{experiment_id}-{timestamp}"
)

# Method 2: Set different WORKLOAD_UID for each Job in Kubernetes
env:
- name: WORKLOAD_UID
  value: "training-exp-001-$(date +%s)"
```

### Q8: How to use in Jupyter Notebook?

**Usage in Notebook**:

```python
# In the first cell of the Notebook
import os
os.environ["POD_NAME"] = "jupyter-notebook-pod"
os.environ["WORKLOAD_UID"] = "jupyter-experiment-001"

# If needed, enable debug
os.environ["PRIMUS_LENS_WANDB_DEBUG"] = "true"

# Import and install Hook
import primus_lens_wandb_exporter.wandb_hook
primus_lens_wandb_exporter.wandb_hook.install_wandb_hook()

# Use WandB normally
import wandb
wandb.init(project="notebook-project")
wandb.log({"loss": 0.5})
```

### Q9: How to verify data has been successfully reported?

**Verification Methods**:

1. **View debug logs**
   ```bash
   export PRIMUS_LENS_WANDB_DEBUG=true
   python train.py 2>&1 | grep "✓"
   # Should see:
   # ✓ Detection data queued for reporting
   # ✓ Metrics data queued for reporting
   ```

2. **Query API**
   ```bash
   # Check data source
   curl "http://lens-api:8080/api/v1/workloads/my-workload/metrics/sources"
   
   # Should include wandb
   {
     "data_sources": [
       {"name": "wandb", "count": 1500}
     ]
   }
   ```

3. **Check database** (requires database access)
   ```sql
   SELECT COUNT(*) 
   FROM training_performance 
   WHERE workload_uid = 'my-workload' 
   AND data_source = 'wandb';
   ```

### Q10: Which training frameworks are supported?

**Currently Supported Frameworks**:

**Wrapper Frameworks**:
- ✅ Primus (Enterprise training framework)
- ✅ PyTorch Lightning
- ✅ Hugging Face Trainer

**Base Frameworks**:
- ✅ Megatron-LM
- ✅ DeepSpeed
- ✅ JAX
- ✅ Transformers
- ✅ PyTorch (Native)
- ✅ TensorFlow (Partial support)

**Automatic Framework Detection**:
- The system automatically identifies the framework you're using
- Supports dual-layer framework detection (e.g., Primus + Megatron)
- No manual configuration required

## System Requirements

### Software Dependencies

| Component | Minimum Version | Recommended Version |
|------|---------|---------|
| Python | 3.7+ | 3.9+ |
| wandb | 0.12.0+ | Latest |
| psutil | 5.8.0+ | Latest (for system metrics) |
| nvidia-ml-py3 | 7.352.0+ | Latest (GPU metrics, optional) |

### Environment Requirements

- Kubernetes cluster (recommended) or standalone server
- Access to Lens telemetry-processor service
- Network bandwidth: ≥ 100 Mbps recommended (large-scale training)
- Storage: Sufficient persistent volume space in local save mode

## Getting Help

### Documentation Resources

- **Technical Documentation**: `docs/wandb-integration-technical.md`
- **API Documentation**: `http://lens-api:8080/api/docs`
- **Lens User Guide**: `docs/user-guide.md`

### View Logs

```bash
# Training Pod logs
kubectl logs <pod-name> --tail=100 -f

# Telemetry Processor logs
kubectl logs -l app=primus-lens-telemetry-processor --tail=100 -f

# API service logs
kubectl logs -l app=primus-lens-api --tail=100 -f
```

### Contact Support

If you encounter problems, please provide the following information:
- Training environment (single machine/distributed, number of GPUs)
- Framework and version used
- Error logs (enable `PRIMUS_LENS_WANDB_DEBUG=true`)
- Training code snippet (optional)

## Language Versions

- [English Documentation](./wandb-integration-user-guide.md) (Current)
- [中文文档](./wandb-integration-user-guide-ZH.md)

---

**Happy Training!** 🚀

**Documentation Version**: 1.0  
**Last Updated**: 2024-12-03  
**Maintainer**: Primus Lens Team

