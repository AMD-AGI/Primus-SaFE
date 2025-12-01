#!/usr/bin/env python3
"""
Main training script for LLaMA model.
Refactored with better structure and configuration management.
"""

import os
import sys
import argparse
import json
from pathlib import Path
from typing import Optional
import warnings

import torch
from transformers import AutoConfig

# Add project root to path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

# Suppress PyTorch backward hook warning
warnings.filterwarnings(
    "ignore",
    message=".*Full backward hook is firing when gradients are computed.*",
    category=UserWarning
)

# Import custom modules
from config import Config, get_default_config
from utils import get_logger, get_progress_logger, setup_logger
from models.build_model import build_model
from data.build_dataset import build_dataset
from engine.trainer import Trainer


def setup_environment(config: Config):
    """Set up training environment"""
    logger = get_logger("setup")
    
    # Disable tokenizers parallelism to avoid warnings with DataLoader
    os.environ["TOKENIZERS_PARALLELISM"] = "false"
    
    # Set random seeds for reproducibility
    if config.system.deterministic:
        import random
        import numpy as np
        
        random.seed(config.system.seed)
        np.random.seed(config.system.seed)
        torch.manual_seed(config.system.seed)
        
        if torch.cuda.is_available():
            torch.cuda.manual_seed(config.system.seed)
            torch.cuda.manual_seed_all(config.system.seed)
            torch.backends.cudnn.deterministic = True
            torch.backends.cudnn.benchmark = False
        
        logger.info(f"Set random seed to {config.system.seed} for reproducibility")
    
    # Set up device
    if config.system.device == "cuda":
        if torch.cuda.is_available():
            torch.cuda.set_device(config.system.device_id)
            # Get GPU ID from environment or use device_id
            gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', config.system.device_id))
            logger.info(f"[GPU {gpu_id}] Using CUDA device {config.system.device_id}: {torch.cuda.get_device_name(config.system.device_id)}")
            logger.info(f"[GPU {gpu_id}] CUDA memory: {torch.cuda.get_device_properties(config.system.device_id).total_memory / 1e9:.2f} GB")
        else:
            logger.warning("CUDA requested but not available, using CPU")
            config.system.device = "cpu"
    
    # Set HuggingFace token if provided
    if config.hf_token:
        os.environ["HF_TOKEN"] = config.hf_token
        logger.info("HuggingFace token set")
    
    # Create necessary directories
    for path_attr in ['output_dir', 'log_dir']:
        path = getattr(config.system, path_attr)
        if path:
            Path(path).mkdir(parents=True, exist_ok=True)
    
    if config.training.checkpoint_dir:
        Path(config.training.checkpoint_dir).mkdir(parents=True, exist_ok=True)


def load_model_config(config: Config) -> AutoConfig:
    """Load model configuration from HuggingFace"""
    logger = get_logger("model")
    
    logger.info(f"Loading model configuration from: {config.model.model_path}")
    
    hf_config = AutoConfig.from_pretrained(
        config.model.model_path,
        trust_remote_code=config.trust_remote_code,
    )
    
    # Override with our settings
    hf_config.num_hidden_layers = config.model.num_layers
    
    logger.info(f"Model configuration loaded:")
    logger.info(f"  - Model type: {hf_config.model_type}")
    logger.info(f"  - Hidden size: {hf_config.hidden_size}")
    logger.info(f"  - Num layers: {hf_config.num_hidden_layers}")
    logger.info(f"  - Num heads: {hf_config.num_attention_heads}")
    logger.info(f"  - Vocab size: {hf_config.vocab_size}")
    
    return hf_config


def initialize_training(config: Config, hf_config):
    """Initialize model, dataset, and trainer"""
    logger = get_logger("training")
    
    # Build model
    logger.info("Building model...")
    model = build_model(config, hf_config)
    
    # Build dataset
    logger.info("Building dataset...")
    train_dataset = build_dataset(config)
    
    # Initialize trainer
    logger.info("Initializing trainer...")
    trainer = Trainer(
        model=model,
        train_dataset=train_dataset,
        config=config,
    )
    
    return trainer


def main():
    """Main training entry point"""
    # Parse arguments
    parser = argparse.ArgumentParser(description="LLaMA model training")
    parser.add_argument(
        "--config", 
        type=str, 
        help="Path to configuration file"
    )
    parser.add_argument(
        "--preset",
        type=str,
        choices=["debug", "production"],
        help="Load a preset configuration (debug for testing, production for training)"
    )
    parser.add_argument(
        "--model-path",
        type=str,
        default="meta-llama/Llama-3.1-8B-Instruct",
        help="HuggingFace model path"
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable debug mode with verbose output"
    )
    parser.add_argument(
        "--log-level",
        type=str,
        default="INFO",
        choices=["DEBUG", "INFO", "WARNING", "ERROR"],
        help="Logging level"
    )
    parser.add_argument(
        "--log-file",
        type=str,
        help="Path to log file"
    )
    parser.add_argument(
        "--experiment-name",
        type=str,
        default="llama_training",
        help="Name for this experiment"
    )
    parser.add_argument(
        "--checkpoint",
        type=str,
        help="Path to checkpoint to resume from"
    )
    parser.add_argument(
        "--hf-token",
        type=str,
        default=os.environ.get("HF_TOKEN"),
        help="HuggingFace API token"
    )
    
    args = parser.parse_args()
    
    # Load or create configuration
    if args.config:
        config = Config.load(args.config)
        logger = get_logger("main")
        logger.info(f"Loaded configuration from {args.config}")
    elif args.preset:
        config = get_default_config(preset=args.preset, debug_mode=args.debug)
        logger = get_logger("main")
        logger.info(f"Loaded preset configuration: {args.preset}")
    else:
        config = get_default_config(debug_mode=args.debug)
        config.model.model_path = args.model_path
        config.experiment_name = args.experiment_name
        
    # Override with command line arguments
    if args.hf_token:
        config.hf_token = args.hf_token
    if args.debug:
        config.debug.enabled = True
        config.debug.log_level = "DEBUG"
    
    # Set up logging
    setup_logger(
        "main",
        level=args.log_level or config.debug.log_level,
        log_file=args.log_file,
        console=True
    )
    
    logger = get_logger("main")
    # Get GPU ID for display
    gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', ''))
    gpu_info = f" [GPU {gpu_id}]" if gpu_id else ""
    logger.info("=" * 80)
    logger.info(f"Starting experiment: {config.experiment_name}{gpu_info}")
    logger.info("=" * 80)
    
    # Validate configuration
    try:
        config.validate()
        logger.info("Configuration validated successfully")
    except Exception as e:
        logger.error(f"Configuration validation failed: {e}")
        sys.exit(1)
    
    # Set up environment
    setup_environment(config)
    
    # Save configuration
    config_path = Path(config.system.output_dir) / f"{config.experiment_name}_config.json"
    config.save(config_path)
    logger.info(f"Configuration saved to {config_path}")
    
    # Load model configuration
    hf_config = load_model_config(config)
    
    # Initialize training components
    trainer = initialize_training(config, hf_config)
    
    # Resume from checkpoint if provided
    if args.checkpoint:
        logger.info(f"Resuming from checkpoint: {args.checkpoint}")
        trainer.load_checkpoint(args.checkpoint)
    
    # Start training
    logger.info("Starting training...")
    try:
        trainer.train()
        logger.info("Training completed successfully!")
    except KeyboardInterrupt:
        logger.warning("Training interrupted by user")
        trainer.save_checkpoint("interrupted")
    except Exception as e:
        logger.error(f"Training failed with error: {e}", exc_info=True)
        trainer.save_checkpoint("error")
        sys.exit(1)


if __name__ == "__main__":
    main()