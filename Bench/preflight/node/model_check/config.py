"""
Configuration management for the training system.
Centralized configuration for better maintainability.
"""

from dataclasses import dataclass, field
from typing import Optional, Dict, Any, Union
import torch
import logging
from pathlib import Path


def str_to_torch_dtype(dtype_str: Union[str, torch.dtype]) -> torch.dtype:
    """Convert string representation to torch.dtype"""
    if isinstance(dtype_str, torch.dtype):
        return dtype_str
    
    dtype_str = str(dtype_str).lower()
    if "bfloat16" in dtype_str:
        return torch.bfloat16
    elif "float16" in dtype_str or "half" in dtype_str:
        return torch.float16
    elif "float32" in dtype_str or "float" in dtype_str:
        return torch.float32
    elif "float64" in dtype_str or "double" in dtype_str:
        return torch.float64
    else:
        # Default to float32
        return torch.float32


@dataclass
class DebugConfig:
    """Debug configuration - simplified to single control"""
    enabled: bool = False  # Master switch for all debug features
    log_level: str = "INFO"  # DEBUG, INFO, WARNING, ERROR


@dataclass
class ModelConfig:
    """Model architecture configuration"""
    model_type: str = "llama"
    model_path: str = "meta-llama/Llama-3.1-8B-Instruct"
    
    # Architecture parameters
    num_layers: int = 4  # Using only 4 layers for testing
    hidden_size: int = 4096
    num_attention_heads: int = 32
    num_key_value_heads: int = 8
    intermediate_size: int = 14336
    vocab_size: int = 128256
    max_position_embeddings: int = 131072
    
    # Attention configuration
    attention_backend: str = "flash_attn"  # flash_attn, native_math, native_flash
    use_flash_attention: bool = True
    rope_theta: float = 500000.0
    
    # Initialization
    init_std: float = 0.02
    tie_word_embeddings: bool = False


@dataclass
class TrainingConfig:
    """Training hyperparameters"""
    # Basic training params
    batch_size: int = 2
    grad_accum_nums: int = 4
    context_length: int = 8192
    max_steps: int = 100
    warmup_steps: int = 50
    
    # Optimizer settings
    learning_rate: float = 1e-5
    weight_decay: float = 0.0
    adam_beta1: float = 0.9
    adam_beta2: float = 0.999
    adam_epsilon: float = 1e-8
    
    # Gradient clipping
    max_grad_norm: float = 1.0
    
    # Mixed precision
    use_amp: bool = True
    amp_dtype: torch.dtype = torch.bfloat16
    
    # Checkpointing
    save_interval: int = 100
    eval_interval: int = 50
    checkpoint_dir: str = "./.checkpoints"  # Hidden directory, auto-created
    
    # Early stopping
    patience: int = 10
    min_delta: float = 1e-4


@dataclass
class DataConfig:
    """Data loading configuration"""
    dataset_name: str = "wikitext"  # c4, wikitext
    data_path: Optional[str] = None
    
    # Data processing
    num_workers: int = 4
    pin_memory: bool = True
    prefetch_factor: int = 2
    
    # Tokenization
    tokenizer_name: Optional[str] = None  # If None, uses model_path
    max_seq_length: int = 8192
    
    # Data augmentation
    use_augmentation: bool = False
    augmentation_prob: float = 0.1


@dataclass
class SystemConfig:
    """System and hardware configuration"""
    # Device settings
    device: str = "cuda" if torch.cuda.is_available() else "cpu"
    device_id: int = 0
    
    # Distributed training
    use_distributed: bool = False
    world_size: int = 1
    rank: int = 0
    backend: str = "nccl"
    
    # Memory management
    empty_cache_interval: int = 10
    gradient_checkpointing: bool = False
    
    # Reproducibility
    seed: int = 42
    deterministic: bool = True
    
    # Paths (auto-created, not tracked in git)
    output_dir: str = "./.outputs"
    log_dir: str = "./.logs"


@dataclass
class Config:
    """Main configuration combining all sub-configs"""
    # Sub-configurations
    debug: DebugConfig = field(default_factory=DebugConfig)
    model: ModelConfig = field(default_factory=ModelConfig)
    training: TrainingConfig = field(default_factory=TrainingConfig)
    data: DataConfig = field(default_factory=DataConfig)
    system: SystemConfig = field(default_factory=SystemConfig)
    
    # HuggingFace settings
    hf_token: Optional[str] = None
    trust_remote_code: bool = True
    
    # Experiment tracking
    experiment_name: str = "llama_training"
    run_name: Optional[str] = None
    tags: Dict[str, Any] = field(default_factory=dict)
    
    @classmethod
    def from_dict(cls, config_dict: Dict[str, Any]) -> "Config":
        """Create config from dictionary"""
        config = cls()
        
        # Update sub-configs
        if "debug" in config_dict:
            config.debug = DebugConfig(**config_dict["debug"])
        if "model" in config_dict:
            config.model = ModelConfig(**config_dict["model"])
        if "training" in config_dict:
            training_dict = config_dict["training"]
            # Convert string dtype back to torch.dtype
            if "amp_dtype" in training_dict:
                training_dict["amp_dtype"] = str_to_torch_dtype(training_dict["amp_dtype"])
            config.training = TrainingConfig(**training_dict)
        if "data" in config_dict:
            config.data = DataConfig(**config_dict["data"])
        if "system" in config_dict:
            config.system = SystemConfig(**config_dict["system"])
        
        # Update main config fields
        for key, value in config_dict.items():
            if key not in ["debug", "model", "training", "data", "system"]:
                if hasattr(config, key):
                    setattr(config, key, value)
        
        return config
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert config to dictionary"""
        return {
            "debug": self.debug.__dict__,
            "model": self.model.__dict__,
            "training": self.training.__dict__,
            "data": self.data.__dict__,
            "system": self.system.__dict__,
            "hf_token": self.hf_token,
            "trust_remote_code": self.trust_remote_code,
            "experiment_name": self.experiment_name,
            "run_name": self.run_name,
            "tags": self.tags,
        }
    
    def save(self, path: str):
        """Save configuration to file"""
        import json
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        
        config_dict = self.to_dict()
        # Convert torch dtypes to strings for JSON serialization
        if isinstance(config_dict["training"]["amp_dtype"], torch.dtype):
            dtype_val = config_dict["training"]["amp_dtype"]
            if dtype_val == torch.bfloat16:
                config_dict["training"]["amp_dtype"] = "torch.bfloat16"
            elif dtype_val == torch.float16:
                config_dict["training"]["amp_dtype"] = "torch.float16"
            elif dtype_val == torch.float32:
                config_dict["training"]["amp_dtype"] = "torch.float32"
            else:
                config_dict["training"]["amp_dtype"] = str(dtype_val)
        
        with open(path, 'w') as f:
            json.dump(config_dict, f, indent=2)
    
    @classmethod
    def load(cls, path: str) -> "Config":
        """Load configuration from file"""
        import json
        with open(path, 'r') as f:
            config_dict = json.load(f)
        
        # Convert string dtypes back to torch dtypes
        if "training" in config_dict and "amp_dtype" in config_dict["training"]:
            config_dict["training"]["amp_dtype"] = str_to_torch_dtype(config_dict["training"]["amp_dtype"])
        
        return cls.from_dict(config_dict)
    
    def validate(self):
        """Validate configuration settings"""
        # Check device
        if self.system.device == "cuda" and not torch.cuda.is_available():
            logging.warning("CUDA requested but not available, falling back to CPU")
            self.system.device = "cpu"
        
        # Check paths
        if self.system.output_dir:
            Path(self.system.output_dir).mkdir(parents=True, exist_ok=True)
        if self.system.log_dir:
            Path(self.system.log_dir).mkdir(parents=True, exist_ok=True)
        if self.training.checkpoint_dir:
            Path(self.training.checkpoint_dir).mkdir(parents=True, exist_ok=True)
        
        # Validate model settings
        assert self.model.num_attention_heads % self.model.num_key_value_heads == 0, \
            "num_attention_heads must be divisible by num_key_value_heads"
        
        # Validate training settings
        assert self.training.batch_size > 0, "Batch size must be positive"
        assert self.training.learning_rate > 0, "Learning rate must be positive"
        assert 0 <= self.training.warmup_steps <= self.training.max_steps, \
            "Warmup steps must be between 0 and max_steps"


def get_default_config(debug_mode: bool = False, preset: Optional[str] = None) -> Config:
    """
    Get configuration for debug or production mode.
    
    Args:
        debug_mode: If True, use debug preset (for backward compatibility)
        preset: 'debug' or 'production' (if not specified, defaults to 'production' unless debug_mode is True)
    
    Returns:
        Config object
    """
    # Determine which preset to use
    if preset:
        if preset not in ["debug", "production"]:
            raise ValueError(f"Invalid preset '{preset}'. Must be 'debug' or 'production'")
    elif debug_mode:
        preset = "debug"
    else:
        preset = "production"
    
    # Load the preset
    preset_path = Path(__file__).parent / "config" / "presets" / f"{preset}.json"
    if preset_path.exists():
        config = Config.load(preset_path)
    else:
        # Fallback to default if preset file doesn't exist
        config = Config()
        if preset == "debug" or debug_mode:
            config.debug.enabled = True
            config.debug.log_level = "DEBUG"
            config.model.num_layers = 2
            config.training.max_steps = 10
            config.training.context_length = 512
            config.training.learning_rate = 1e-7
    
    config.validate()
    return config
