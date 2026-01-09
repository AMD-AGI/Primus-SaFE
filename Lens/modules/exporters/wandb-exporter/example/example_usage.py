# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
Example: Demonstrating Primus Lens WandB Exporter Usage

This script demonstrates how Primus Lens automatically hooks and enhances
wandb functionality without any code modifications.
"""
import os
import time
import random
import tempfile

# Set environment variables (usually set before running the script)
os.environ.setdefault('PRIMUS_LENS_WANDB_HOOK', 'true')
os.environ.setdefault('PRIMUS_LENS_WANDB_ENHANCE_METRICS', 'true')
os.environ.setdefault('PRIMUS_LENS_WANDB_SAVE_LOCAL', 'true')

# Create temporary directory for demo
tmpdir = tempfile.mkdtemp()
os.environ.setdefault('PRIMUS_LENS_WANDB_OUTPUT_PATH', tmpdir)

print("=" * 70)
print("Primus Lens WandB Exporter - Usage Example")
print("=" * 70)
print()
print("Environment variables set:")
print(f"  PRIMUS_LENS_WANDB_HOOK: {os.environ.get('PRIMUS_LENS_WANDB_HOOK')}")
print(f"  PRIMUS_LENS_WANDB_ENHANCE_METRICS: {os.environ.get('PRIMUS_LENS_WANDB_ENHANCE_METRICS')}")
print(f"  PRIMUS_LENS_WANDB_SAVE_LOCAL: {os.environ.get('PRIMUS_LENS_WANDB_SAVE_LOCAL')}")
print(f"  PRIMUS_LENS_WANDB_OUTPUT_PATH: {os.environ.get('PRIMUS_LENS_WANDB_OUTPUT_PATH')}")
print()
print("=" * 70)
print()


def example_basic_usage():
    """Example 1: Basic Usage"""
    print("Example 1: Basic Usage")
    print("-" * 70)
    print("This example shows basic wandb logging with automatic interception.")
    print()
    
    try:
        import wandb
        
        # Use offline mode to avoid actual uploads
        os.environ['WANDB_MODE'] = 'offline'
        
        print("Initializing wandb... (will be intercepted by Primus Lens)")
        run = wandb.init(
            project="primus-lens-demo",
            name="example-basic",
            config={"learning_rate": 0.001, "batch_size": 32}
        )
        
        print(f"WandB run initialized: {run.name}")
        print()
        
        print("Logging metrics...")
        for step in range(5):
            metrics = {
                "step": step,
                "loss": random.uniform(0.1, 1.0) / (step + 1),
                "accuracy": 1.0 - (random.uniform(0.1, 1.0) / (step + 1)),
            }
            
            wandb.log(metrics, step=step)
            print(f"  Step {step}: {metrics}")
            time.sleep(0.2)
        
        print()
        print("✓ Basic usage completed")
        wandb.finish()
        
    except ImportError:
        print("⚠ wandb not installed. Install it with: pip install wandb")
    
    print()


def example_distributed_training():
    """Example 2: Simulate Distributed Training"""
    print("Example 2: Distributed Training Simulation")
    print("-" * 70)
    print("This example simulates distributed training with rank information.")
    print()
    
    try:
        import wandb
        
        # Simulate distributed training environment
        os.environ['RANK'] = '0'
        os.environ['LOCAL_RANK'] = '0'
        os.environ['NODE_RANK'] = '0'
        os.environ['WORLD_SIZE'] = '4'
        os.environ['WANDB_MODE'] = 'offline'
        
        print("Simulating rank 0 training...")
        run = wandb.init(
            project="primus-lens-distributed",
            name="rank-0",
            config={"epochs": 3}
        )
        
        for epoch in range(3):
            wandb.log({
                "epoch": epoch,
                "train_loss": random.uniform(0.5, 1.0) / (epoch + 1),
                "val_loss": random.uniform(0.4, 0.9) / (epoch + 1),
            }, step=epoch)
            print(f"  Epoch {epoch} logged")
            time.sleep(0.2)
        
        print()
        print("✓ Distributed training simulation completed")
        wandb.finish()
        
        # Clean up environment variables
        for var in ['RANK', 'LOCAL_RANK', 'NODE_RANK', 'WORLD_SIZE']:
            os.environ.pop(var, None)
        
    except ImportError:
        print("⚠ wandb not installed")
    
    print()


def example_check_output():
    """Example 3: Check Output Files"""
    print("Example 3: Check Output Files")
    print("-" * 70)
    print("Checking if metrics were saved to local files...")
    print()
    
    output_path = os.environ.get('PRIMUS_LENS_WANDB_OUTPUT_PATH')
    if output_path and os.path.exists(output_path):
        import json
        
        # Find all wandb_metrics.jsonl files
        metrics_files = []
        for root, dirs, files in os.walk(output_path):
            for file in files:
                if file == 'wandb_metrics.jsonl':
                    metrics_files.append(os.path.join(root, file))
        
        if metrics_files:
            print(f"✓ Found {len(metrics_files)} metrics file(s):")
            for mf in metrics_files:
                print(f"  - {mf}")
                
                # Read and display first few lines
                with open(mf, 'r') as f:
                    lines = f.readlines()
                    print(f"    Total entries: {len(lines)}")
                    if lines:
                        print(f"    First entry: {lines[0].strip()[:100]}...")
            print()
        else:
            print("⚠ No metrics files found")
    else:
        print("⚠ Output path not set or doesn't exist")
    
    print()


def example_verify_interception():
    """Example 4: Verify Hook Status"""
    print("Example 4: Verify Interception")
    print("-" * 70)
    
    try:
        import wandb
        
        print("Checking if wandb has been patched...")
        
        if hasattr(wandb, '_primus_lens_patched'):
            print("✓ WandB has been successfully patched by Primus Lens!")
            print(f"  wandb.init: {wandb.init}")
            print(f"  wandb.log: {wandb.log}")
        else:
            print("⚠ WandB may not be patched yet")
            print("  This is normal if wandb was imported before Primus Lens")
        
    except ImportError:
        print("⚠ wandb not installed")
    
    print()


def main():
    """Main function"""
    print("\n")
    
    # Run examples
    example_basic_usage()
    time.sleep(0.5)
    
    example_distributed_training()
    time.sleep(0.5)
    
    example_check_output()
    time.sleep(0.5)
    
    example_verify_interception()
    
    print("=" * 70)
    print("All examples completed!")
    print("=" * 70)
    print()
    print("Next steps:")
    print(f"  1. Check the metrics output at: {os.environ.get('PRIMUS_LENS_WANDB_OUTPUT_PATH')}")
    print("  2. Run your own training script - no code changes needed!")
    print("  3. Set PRIMUS_LENS_WANDB_HOOK=false to disable the hook")
    print()
    
    # Clean up temporary directory
    import shutil
    try:
        shutil.rmtree(tmpdir)
        print(f"Cleaned up temporary directory: {tmpdir}")
    except:
        pass


if __name__ == "__main__":
    main()
