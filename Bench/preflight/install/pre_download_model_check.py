#!/usr/bin/env python3
"""
Pre-download model (config, tokenizer) and data (dataset, tokenized cache) for model_check.
Runs during Docker build to avoid runtime network downloads.
"""
import os
import sys

# Add model_check to path and set cwd for config loading
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
MODEL_CHECK_DIR = os.path.normpath(os.path.join(SCRIPT_DIR, "..", "node", "model_check"))
sys.path.insert(0, MODEL_CHECK_DIR)
os.chdir(MODEL_CHECK_DIR)

def main():
    from config import get_default_config
    from transformers import AutoConfig, AutoTokenizer
    from data.build_dataset import build_dataset

    config = get_default_config()
    model_path = config.model.model_path

    print(f"[1/3] Downloading model config: {model_path}")
    AutoConfig.from_pretrained(
        model_path,
        trust_remote_code=getattr(config, "trust_remote_code", True),
    )
    print("  Config cached.")

    print(f"[2/3] Downloading tokenizer: {model_path}")
    AutoTokenizer.from_pretrained(model_path, use_fast=True)
    print("  Tokenizer cached.")

    print(f"[3/3] Preparing dataset: {config.data.dataset_name}")
    build_dataset(config, use_cache=True, local_files_only=False)
    print("  Dataset cached.")

    print("Pre-download completed: model + data ready.")
    return 0

if __name__ == "__main__":
    sys.exit(main())
