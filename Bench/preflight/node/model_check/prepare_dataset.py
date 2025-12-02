#!/usr/bin/env python3
"""
Prepare and cache dataset before multi-GPU training.
This ensures all GPUs can share the same cached dataset.
"""

import os
import sys
from pathlib import Path

# Add project root to path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config import get_default_config
from data.build_dataset import build_dataset
from transformers import AutoTokenizer

def main():
    """Pre-cache dataset for multi-GPU training"""
    print("=" * 60)
    print("Preparing dataset cache...")
    print("=" * 60)
    
    # Load configuration
    config = get_default_config()
    
    # Build dataset (will cache automatically)
    print(f"Dataset: {config.data.dataset_name}")
    print(f"Model: {config.model.model_path}")
    print(f"Context length: {config.training.context_length}")
    
    # This will either build and cache, or load from cache
    dataset = build_dataset(config, use_cache=True)
    
    # Get actual cache directory from environment or default
    import os
    cache_dir = os.environ.get("DATASET_CACHE_DIR", ".cache/datasets")
    
    print("=" * 60)
    print(f"Dataset ready: {len(dataset)} samples")
    print(f"Cache location: {cache_dir}")
    print("=" * 60)
    
    return 0

if __name__ == "__main__":
    sys.exit(main())
