# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
Debug Logging Switch Demo

This script demonstrates the effect of the PRIMUS_LENS_WANDB_DEBUG environment variable.

Usage:
    # Enable debug logging
    PRIMUS_LENS_WANDB_DEBUG=true python example_debug_demo.py

    # Disable debug logging (default)
    PRIMUS_LENS_WANDB_DEBUG=false python example_debug_demo.py
    # or
    python example_debug_demo.py
"""

import os
import time

print("\n" + "=" * 80)
print("Debug Logging Switch Demo")
print("=" * 80)

# Display current configuration
debug_env = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false")
print(f"\nCurrent environment variable: PRIMUS_LENS_WANDB_DEBUG={debug_env}")

if debug_env.lower() in ("true", "1", "yes"):
    print("✅ Debug logging enabled - you will see detailed [Primus Lens] log messages")
else:
    print("✅ Debug logging disabled - you will only see essential output (recommended for production)")

print("\n" + "-" * 80)
print("Starting training simulation...")
print("-" * 80 + "\n")

try:
    # Set WandB to offline mode
    os.environ['WANDB_MODE'] = 'offline'
    os.environ['WANDB_SILENT'] = 'true'
    
    import wandb
    
    # Initialize WandB
    print(">>> wandb.init(project='debug-demo', name='test-run')")
    run = wandb.init(
        project="debug-demo",
        name="test-run",
        config={
            "learning_rate": 0.001,
            "epochs": 10,
        },
        reinit=True
    )
    
    # Simulate training loop
    print("\n>>> Starting training loop...")
    for epoch in range(3):
        print(f"\nEpoch {epoch + 1}/3")
        
        # Simulate training metrics
        loss = 1.0 / (epoch + 1)
        accuracy = 0.5 + (epoch * 0.15)
        
        print(f"  loss: {loss:.4f}, accuracy: {accuracy:.4f}")
        
        # Log metrics
        wandb.log({
            "epoch": epoch + 1,
            "loss": loss,
            "accuracy": accuracy,
        }, step=epoch + 1)
        
        time.sleep(0.1)  # Simulate training time
    
    print("\n>>> wandb.finish()")
    wandb.finish()
    
    print("\n" + "-" * 80)
    print("Training completed!")
    print("-" * 80)
    
except ImportError:
    print("❌ WandB not installed")
    print("\nInstallation command:")
    print("  pip install wandb")
    exit(1)

except Exception as e:
    print(f"❌ Error occurred: {e}")
    import traceback
    traceback.print_exc()
    exit(1)

# Summary
print("\n" + "=" * 80)
print("Demo Completed")
print("=" * 80)

if debug_env.lower() in ("true", "1", "yes"):
    print("""
💡 Key Observations (debug mode):
   - You should see many [Primus Lens WandB] prefixed logs
   - Including hook success, initialization info, detailed log information
   - These messages are helpful for debugging, but may be redundant in production

🔄 Try disabling debug logging:
   export PRIMUS_LENS_WANDB_DEBUG=false
   python example_debug_demo.py
""")
else:
    print("""
💡 Key Observations (normal mode):
   - You should NOT see [Primus Lens] prefixed logs
   - Clean output with only training-related information
   - This is the recommended configuration for production

🔍 If debugging is needed, enable debug logging:
   export PRIMUS_LENS_WANDB_DEBUG=true
   python example_debug_demo.py
""")

print("\n📚 For more information, see: DEBUG_LOGGING.md")
print("=" * 80 + "\n")
