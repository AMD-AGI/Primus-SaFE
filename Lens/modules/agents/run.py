#!/usr/bin/env python
"""Entry point for running the GPU Usage Analysis Agent API."""

import sys
import os

# Ensure /app is in the Python path at the very beginning
sys.path.insert(0, '/app')
os.chdir('/app')

# Debug: Print Python path and check module locations
print("=" * 60)
print("Starting GPU Usage Analysis Agent")
print("=" * 60)
print(f"Python version: {sys.version}")
print(f"Working directory: {os.getcwd()}")
print(f"Python path: {sys.path}")
print()

# Verify required modules exist
print("Checking required modules...")
required_dirs = ['api', 'config', 'cache', 'storage', 'gpu_usage_agent']
for dir_name in required_dirs:
    dir_path = os.path.join('/app', dir_name)
    init_file = os.path.join(dir_path, '__init__.py')
    print(f"  {dir_name}: {'✓' if os.path.isdir(dir_path) else '✗'} (dir)")
    print(f"    __init__.py: {'✓' if os.path.isfile(init_file) else '✗'}")

print()

# Test import config module
try:
    print("Testing config module import...")
    import config
    print(f"  config module location: {config.__file__}")
    print(f"  config module contents: {dir(config)}")
    from config import load_config, get_config
    print("  ✓ Successfully imported load_config and get_config")
except Exception as e:
    print(f"  ✗ Error importing config: {e}")
    import traceback
    traceback.print_exc()
    sys.exit(1)

print()
print("=" * 60)
print("Starting API server...")
print("=" * 60)
print()

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "api.main:app",
        host="0.0.0.0",
        port=8001,
        log_level="info"
    )

