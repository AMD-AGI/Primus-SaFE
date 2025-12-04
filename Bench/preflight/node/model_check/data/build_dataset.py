###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

import os
import hashlib
import json
import fcntl
import time
from itertools import islice
from pathlib import Path

import torch
from datasets import load_dataset
from torch.utils.data import DataLoader, Dataset
from tqdm import tqdm
from transformers import AutoTokenizer


# Cache directory configuration
# Priority: 
# 1. DATASET_CACHE_DIR environment variable
# 2. .cache/datasets in current directory (auto-created)
DEFAULT_CACHE_DIR = ".cache/datasets"
CACHE_DIR = os.environ.get("DATASET_CACHE_DIR", DEFAULT_CACHE_DIR)

# Create cache directory if it doesn't exist
Path(CACHE_DIR).mkdir(parents=True, exist_ok=True)


class CausalLMDataset(Dataset):
    def __init__(self, token_ids: list[int], context_length: int):
        self.context_length = context_length
        total_len = (len(token_ids) // (context_length + 1)) * (context_length + 1)
        self.token_ids = torch.tensor(token_ids[:total_len], dtype=torch.long)

    def __len__(self):
        return len(self.token_ids) // (self.context_length + 1)

    def __getitem__(self, idx):
        i = idx * (self.context_length + 1)
        chunk = self.token_ids[i : i + self.context_length + 1]
        input_ids = chunk[:-1]
        labels = chunk[1:]
        return {
            "input_ids": input_ids,
            "labels": labels
        }

    def __repr__(self):
        return f"CausalLMDataset(total_tokens={len(self.token_ids)}, num_samples={len(self)})"


def get_cache_key(tokenizer_name: str, dataset_name: str, split: str, max_samples: int) -> str:
    """Generate a unique cache key based on dataset parameters."""
    params = {
        "tokenizer": tokenizer_name,
        "dataset": dataset_name,
        "split": split,
        "max_samples": max_samples,
    }
    param_str = json.dumps(params, sort_keys=True)
    return hashlib.md5(param_str.encode()).hexdigest()


def get_cache_path(cache_key: str, cache_dir: str = None) -> Path:
    """Get the cache file path for a given cache key."""
    cache_dir = Path(cache_dir or CACHE_DIR)
    cache_dir.mkdir(parents=True, exist_ok=True)
    return cache_dir / f"{cache_key}.pt"


def load_from_cache(cache_path: Path) -> list[int]:
    """Load tokenized data from cache."""
    if cache_path.exists():
        try:
            data = torch.load(cache_path, weights_only=True)
            return data["token_ids"]
        except Exception as e:
            print(f"[Cache] Failed to load cache: {e}")
    return None


def save_to_cache(cache_path: Path, token_ids: list[int]):
    """Save tokenized data to cache with file locking."""
    lock_path = cache_path.with_suffix(".lock")
    
    try:
        # Use file locking to prevent race conditions
        with open(lock_path, 'w') as lock_file:
            fcntl.flock(lock_file.fileno(), fcntl.LOCK_EX)
            try:
                # Double check if another process already saved
                if not cache_path.exists():
                    torch.save({"token_ids": token_ids}, cache_path)
                    print(f"[Cache] Saved to {cache_path}")
            finally:
                fcntl.flock(lock_file.fileno(), fcntl.LOCK_UN)
    except Exception as e:
        print(f"[Cache] Failed to save cache: {e}")
    finally:
        # Clean up lock file
        try:
            lock_path.unlink(missing_ok=True)
        except:
            pass


def wait_for_cache(cache_path: Path, timeout: int = 300) -> list[int]:
    """Wait for cache file to be created by another process."""
    lock_path = cache_path.with_suffix(".lock")
    start_time = time.time()
    
    while time.time() - start_time < timeout:
        if cache_path.exists():
            data = load_from_cache(cache_path)
            if data is not None:
                return data
        if not lock_path.exists():
            # No lock file and no cache - need to create
            return None
        print(f"[Cache] Waiting for cache file (another process is building)...")
        time.sleep(5)
    
    print(f"[Cache] Timeout waiting for cache, will build dataset")
    return None


def load_and_tokenize(
    tokenizer, 
    dataset_name: str, 
    split: str, 
    max_samples: int,
    use_cache: bool = True,
    cache_dir: str = None
):
    """Load and tokenize dataset with caching support."""
    
    # Generate cache key
    tokenizer_name = tokenizer.name_or_path
    cache_key = get_cache_key(tokenizer_name, dataset_name, split, max_samples)
    cache_path = get_cache_path(cache_key, cache_dir)
    
    if use_cache:
        # Try to load from cache
        cached_data = load_from_cache(cache_path)
        if cached_data is not None:
            print(f"[Cache] Loaded {split} data from cache ({len(cached_data):,} tokens)")
            return cached_data
        
        # Check if another process is building the cache
        lock_path = cache_path.with_suffix(".lock")
        if lock_path.exists():
            cached_data = wait_for_cache(cache_path)
            if cached_data is not None:
                return cached_data
    
    # Build dataset
    if dataset_name == "wikitext":
        dataset = load_dataset("wikitext", "wikitext-2-raw-v1", split=split)
        text_key = "text"
        texts = [sample[text_key] for sample in dataset if sample[text_key].strip()]
        texts = texts[:max_samples]

    elif dataset_name == "c4":
        dataset = load_dataset("allenai/c4", "en", split=split, streaming=True)
        text_key = "text"

        def filtered_texts():
            for sample in dataset:
                if sample[text_key].strip():
                    yield sample[text_key]

        texts = list(islice(filtered_texts(), max_samples))

    else:
        raise ValueError(f"Unsupported dataset: {dataset_name}")
    
    # Tokenize
    token_ids = []
    for text in tqdm(texts, desc=f"Encoding ({split})", ncols=80):
        ids = tokenizer.encode(text, add_special_tokens=True)
        token_ids.extend(ids)

    # Save to cache
    if use_cache:
        save_to_cache(cache_path, token_ids)

    return token_ids


def get_dataloaders(
    config,
    max_train_samples=500000,
    max_val_samples=1000,
    use_cache: bool = True,
    cache_dir: str = None,
):
    """Get train and validation dataloaders with caching support."""
    tokenizer = AutoTokenizer.from_pretrained(config.model_path, use_fast=True)
    tokenizer.pad_token = tokenizer.eos_token

    train_ids = load_and_tokenize(
        tokenizer,
        config.dataset_name,
        "train",
        max_train_samples,
        use_cache=use_cache,
        cache_dir=cache_dir,
    )
    val_ids = load_and_tokenize(
        tokenizer,
        config.dataset_name,
        "validation",
        max_val_samples,
        use_cache=use_cache,
        cache_dir=cache_dir,
    )

    train_dataset = CausalLMDataset(train_ids, config.context_length)
    val_dataset = CausalLMDataset(val_ids, config.context_length)

    print(train_dataset)
    print(val_dataset)

    train_loader = DataLoader(
        train_dataset,
        batch_size=config.batch_size,
        shuffle=True,
        drop_last=True,
        num_workers=2,
        pin_memory=True,
    )

    val_loader = DataLoader(
        val_dataset,
        batch_size=config.batch_size,
        shuffle=False,
        drop_last=True,
        num_workers=2,
        pin_memory=True,
    )

    return train_loader, val_loader


def build_dataset(
    config,
    use_cache: bool = True,
    cache_dir: str = None,
):
    """
    Build dataset for training with caching support.
    
    Args:
        config: Configuration object with data settings
        use_cache: Whether to use caching (default: True)
        cache_dir: Custom cache directory (default: ./cache/datasets)
        
    Returns:
        Training dataset
    """
    # Extract model path for tokenizer
    model_path = config.model.model_path if hasattr(config, 'model') else config.model_path
    
    # Create tokenizer
    tokenizer = AutoTokenizer.from_pretrained(model_path, use_fast=True)
    tokenizer.pad_token = tokenizer.eos_token
    
    # Get dataset name and other settings
    dataset_name = config.data.dataset_name if hasattr(config, 'data') else config.dataset_name
    context_length = config.training.context_length if hasattr(config, 'training') else config.context_length
    max_train_samples = getattr(config, 'max_train_samples', 500000)
    
    # Load and tokenize data with caching
    train_ids = load_and_tokenize(
        tokenizer,
        dataset_name,
        "train",
        max_train_samples,
        use_cache=use_cache,
        cache_dir=cache_dir,
    )
    
    # Create dataset
    train_dataset = CausalLMDataset(train_ids, context_length)
    
    print(f"[Dataset] {train_dataset}")
    
    return train_dataset
