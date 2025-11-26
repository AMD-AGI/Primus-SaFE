#!/usr/bin/env python3
"""
示例：使用 WandB Exporter 的 API 上报功能

这个示例展示了如何通过环境变量配置 API 上报，
并演示了框架检测数据和训练指标的异步上报。

运行方式：
    # 设置必需的环境变量
    export WORKLOAD_UID="my-workload-123"
    export POD_UID="my-pod-456"
    export PRIMUS_CONFIG="/path/to/config.yaml"  # 可选：框架特征
    export PRIMUS_LENS_API_BASE_URL="http://localhost:8080/api/v1"  # API 地址
    
    # 运行示例
    python example_api_reporting.py
"""

import os
import time
import wandb

# ========== 配置环境变量 ==========

# 1. Workload 标识（必需）
os.environ["WORKLOAD_UID"] = "example-workload-123"
os.environ["POD_NAME"] = "example-pod"
os.environ["POD_NAMESPACE"] = "default"

# 2. 框架特征（可选，用于框架检测）
os.environ["PRIMUS_CONFIG"] = "/config/primus.yaml"
os.environ["PRIMUS_VERSION"] = "1.2.3"

# 3. API 配置
os.environ["PRIMUS_LENS_API_BASE_URL"] = "http://primus-lens-telemetry-processor:8080/api/v1"
os.environ["PRIMUS_LENS_WANDB_API_REPORTING"] = "true"

# 4. 本地文件保存（可选）
os.environ["PRIMUS_LENS_WANDB_SAVE_LOCAL"] = "true"
os.environ["PRIMUS_LENS_WANDB_OUTPUT_PATH"] = "/tmp/wandb_metrics"

print("=" * 60)
print("Primus Lens WandB Exporter - API Reporting Example")
print("=" * 60)
print()
print("Environment Configuration:")
print(f"  WORKLOAD_UID: {os.environ.get('WORKLOAD_UID')}")
print(f"  POD_NAME: {os.environ.get('POD_NAME')}")
print(f"  PRIMUS_CONFIG: {os.environ.get('PRIMUS_CONFIG')}")
print(f"  API_BASE_URL: {os.environ.get('PRIMUS_LENS_API_BASE_URL')}")
print()

# ========== 初始化 WandB ==========

print("Initializing WandB...")
print()

# 初始化 wandb - 这会触发框架检测数据的采集和上报
run = wandb.init(
    project="primus-training-exp",
    name="example-run",
    config={
        "framework": "primus",  # 这会被作为 hint 使用
        "learning_rate": 0.001,
        "batch_size": 32,
        "epochs": 10,
    },
    tags=["training", "primus", "example"],
)

print()
print(f"✓ WandB run initialized: {run.name}")
print(f"  Project: {run.project}")
print(f"  Run ID: {run.id}")
print()
print("→ Framework detection data has been queued for async reporting")
print()

# ========== 模拟训练过程 ==========

print("Starting training simulation...")
print()

num_steps = 20
for step in range(num_steps):
    # 模拟训练指标
    loss = 2.5 - (step * 0.1)  # 逐渐下降
    accuracy = 0.5 + (step * 0.02)  # 逐渐上升
    
    # 记录指标 - 这会触发指标数据的异步上报
    wandb.log({
        "loss": loss,
        "accuracy": accuracy,
        "learning_rate": 0.001,
        "step": step,
    }, step=step)
    
    # 每5步打印一次进度
    if step % 5 == 0:
        print(f"  Step {step:3d}: loss={loss:.3f}, accuracy={accuracy:.3f}")
    
    # 模拟训练耗时
    time.sleep(0.1)

print()
print(f"✓ Training completed: {num_steps} steps")
print()
print("→ All metrics have been queued for async reporting")
print()

# ========== 结束 WandB Run ==========

print("Finishing WandB run...")
wandb.finish()

print()
print("✓ WandB run finished")
print()

# ========== 等待异步上报完成 ==========

print("Waiting for async reporter to flush data...")
time.sleep(2)  # 等待后台线程完成上报

print()
print("=" * 60)
print("Example completed!")
print("=" * 60)
print()
print("What happened:")
print()
print("1. Framework Detection:")
print("   - Collected environment variables (PRIMUS_CONFIG, etc.)")
print("   - Collected WandB config (framework: primus)")
print("   - Generated hints (possible_frameworks: [primus])")
print("   - Sent POST /api/v1/wandb/detection")
print()
print("2. Training Metrics:")
print(f"   - Queued {num_steps} metric batches")
print("   - Sent POST /api/v1/wandb/metrics (batched)")
print()
print("3. Async Reporting:")
print("   - All data sent in background thread")
print("   - No blocking of training process")
print("   - Data flushed on program exit")
print()
print("Next steps:")
print("  - Check telemetry-processor logs for received data")
print("  - Query framework detection: GET /api/v1/workloads/{workload_uid}/framework-detection")
print()

