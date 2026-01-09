#!/usr/bin/env python3
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
Example: Using WandB Exporter's API Reporting Feature

This example demonstrates how to configure API reporting through environment variables,
and shows the async reporting of framework detection data and training metrics.

Usage:
    # Set required environment variables
    export WORKLOAD_UID="my-workload-123"
    export POD_UID="my-pod-456"
    export PRIMUS_CONFIG="/path/to/config.yaml"  # Optional: framework features
    export PRIMUS_LENS_API_BASE_URL="http://localhost:8080/api/v1"  # API endpoint
    
    # Run the example
    python example_api_reporting.py
"""

import os
import time
import wandb

# ========== Configure Environment Variables ==========

# 1. Workload identifier (required)
os.environ["WORKLOAD_UID"] = "example-workload-123"
os.environ["POD_NAME"] = "example-pod"
os.environ["POD_NAMESPACE"] = "default"

# 2. Framework features (optional, for framework detection)
os.environ["PRIMUS_CONFIG"] = "/config/primus.yaml"
os.environ["PRIMUS_VERSION"] = "1.2.3"

# 3. API configuration
os.environ["PRIMUS_LENS_API_BASE_URL"] = "http://primus-lens-telemetry-processor:8080/api/v1"
os.environ["PRIMUS_LENS_WANDB_API_REPORTING"] = "true"

# 4. Local file saving (optional)
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

# ========== Initialize WandB ==========

print("Initializing WandB...")
print()

# Initialize wandb - this will trigger framework detection data collection and reporting
run = wandb.init(
    project="primus-training-exp",
    name="example-run",
    config={
        "framework": "primus",  # This will be used as a hint
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

# ========== Simulate Training Process ==========

print("Starting training simulation...")
print()

num_steps = 20
for step in range(num_steps):
    # Simulate training metrics
    loss = 2.5 - (step * 0.1)  # Gradually decreasing
    accuracy = 0.5 + (step * 0.02)  # Gradually increasing
    
    # Log metrics - this will trigger async reporting of metric data
    wandb.log({
        "loss": loss,
        "accuracy": accuracy,
        "learning_rate": 0.001,
        "step": step,
    }, step=step)
    
    # Print progress every 5 steps
    if step % 5 == 0:
        print(f"  Step {step:3d}: loss={loss:.3f}, accuracy={accuracy:.3f}")
    
    # Simulate training time
    time.sleep(0.1)

print()
print(f"✓ Training completed: {num_steps} steps")
print()
print("→ All metrics have been queued for async reporting")
print()

# ========== Finish WandB Run ==========

print("Finishing WandB run...")
wandb.finish()

print()
print("✓ WandB run finished")
print()

# ========== Wait for Async Reporting to Complete ==========

print("Waiting for async reporter to flush data...")
time.sleep(2)  # Wait for background thread to complete reporting

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
