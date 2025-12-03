#!/usr/bin/env python3
"""
Simple WandB Exporter Test Example

This is a minimal test example for quickly verifying that wandb-exporter is working properly.
For more comprehensive testing, please use test_real_scenario.py.

Usage:
    python example_simple_test.py
"""

import os
import sys
import tempfile
import time

# Set environment variables
os.environ["PRIMUS_LENS_WANDB_HOOK"] = "true"
os.environ["PRIMUS_LENS_WANDB_ENHANCE_METRICS"] = "true"
os.environ["PRIMUS_LENS_WANDB_SAVE_LOCAL"] = "true"
os.environ["PRIMUS_LENS_WANDB_OUTPUT_PATH"] = tempfile.mkdtemp(prefix="wandb_simple_test_")
os.environ["PRIMUS_LENS_WANDB_API_REPORTING"] = "false"  # Disable API reporting (not needed for local testing)
os.environ["WANDB_MODE"] = "offline"  # Use offline mode, no real upload to W&B
os.environ["WANDB_SILENT"] = "true"

print("="*60)
print("WandB Exporter Simple Test")
print("="*60)
print()

# Import wandb
print("1. Importing wandb...")
try:
    import wandb
    print("   ✓ wandb imported successfully")
except ImportError:
    print("   ✗ wandb not installed")
    print("   Please run: pip install wandb")
    sys.exit(1)

# Check if hooked
print("\n2. Checking hook status...")
if hasattr(wandb, '_primus_lens_patched'):
    print("   ✓ WandB successfully hooked by Primus Lens")
    print(f"   wandb.log type: {type(wandb.log)}")
    print(f"   wandb.log name: {wandb.log.__name__ if hasattr(wandb.log, '__name__') else 'N/A'}")
    # Try calling directly to test
    print("   Testing direct wandb.log call:")
    try:
        wandb.log({"test": 123})
        print("   ✓ wandb.log() callable")
    except Exception as e:
        print(f"   ! wandb.log() call failed: {e}")
else:
    print("   ✗ WandB not hooked")
    print("   Please run: python install_hook.py install")
    sys.exit(1)

# Initialize wandb
print("\n3. Initializing WandB run...")
try:
    run = wandb.init(
        project="simple-test",
        name="test-run",
        config={"test": True}
    )
    print(f"   ✓ Run initialized successfully: {run.name}")
except Exception as e:
    print(f"   ✗ Initialization failed: {e}")
    sys.exit(1)

# Log some metrics
print("\n4. Logging training metrics...")
try:
    for step in range(5):
        print(f"   Logging step {step}...")
        wandb.log({
            "loss": 1.0 - (step * 0.1),
            "accuracy": 0.5 + (step * 0.08),
        }, step=step)
    print(f"   ✓ Successfully logged 5 steps of metrics")
except Exception as e:
    print(f"   ✗ Logging failed: {e}")
    import traceback
    traceback.print_exc()
    sys.exit(1)

# Finish run
print("\n5. Finishing WandB run...")
wandb.finish()
print("   ✓ Run finished")

# Wait for file writing
time.sleep(0.5)

# Verify output files
print("\n6. Verifying output files...")
output_path = os.environ["PRIMUS_LENS_WANDB_OUTPUT_PATH"]
print(f"   Output directory: {output_path}")

# Check directory structure
if os.path.exists(output_path):
    print(f"   ✓ Output directory exists")
    # List all files
    for root, dirs, files in os.walk(output_path):
        level = root.replace(output_path, '').count(os.sep)
        indent = ' ' * 2 * level
        print(f"   {indent}{os.path.basename(root)}/")
        subindent = ' ' * 2 * (level + 1)
        for file in files:
            print(f"   {subindent}{file}")
else:
    print(f"   ✗ Output directory does not exist")

# In non-distributed environment, LOCAL_RANK defaults to -1
metrics_file = os.path.join(output_path, "node_0", "rank_-1", "wandb_metrics.jsonl")
print(f"   Expected file: {metrics_file}")

if os.path.exists(metrics_file):
    with open(metrics_file, 'r') as f:
        lines = f.readlines()
    print(f"   ✓ Metrics file generated: {metrics_file}")
    print(f"   ✓ Contains {len(lines)} records")
    
    # Display first record
    import json
    first_record = json.loads(lines[0])
    print(f"\n   First record example:")
    print(f"   - Timestamp: {first_record['timestamp']}")
    print(f"   - Step: {first_record['step']}")
    print(f"   - Metric count: {len(first_record['data'])}")
    
    # Check for Primus Lens marker
    if "_primus_lens_enabled" in first_record['data']:
        print(f"   ✓ Contains Primus Lens marker")
    
    # Check system metrics
    sys_metrics = [k for k in first_record['data'].keys() if k.startswith('_primus_sys_')]
    if sys_metrics:
        print(f"   ✓ Contains system metrics: {', '.join(sys_metrics)}")
else:
    print(f"   ✗ Metrics file not generated")
    sys.exit(1)

# Cleanup
print(f"\n7. Cleaning up temporary files...")
import shutil
try:
    shutil.rmtree(output_path)
    print(f"   ✓ Cleaned up: {output_path}")
except:
    print(f"   ⚠ Cleanup failed (can be deleted manually): {output_path}")

# Summary
print("\n" + "="*60)
print("🎉 Test Successful! WandB Exporter is working properly!")
print("="*60)
print()
print("Next steps:")
print("  1. Run full test: python test_real_scenario.py")
print("  2. View test guide: cat TEST_GUIDE.md")
print("  3. Use in your training script (no code modifications needed)")
print()
