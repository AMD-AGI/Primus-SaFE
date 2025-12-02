"""
Model building and registration system.
Provides a clean interface for model instantiation.
"""

import importlib
from typing import Type, Dict, Tuple, Optional
import torch.nn as nn

from utils import get_logger


# Global model registry
MODEL_REGISTRY: Dict[Tuple[str, str], Type[nn.Module]] = {}


def register_model(model_type: str, backend: str = "torch"):
    """
    Decorator to register a model class.
    
    Args:
        model_type: Type of model (e.g., 'llama', 'gpt2')
        backend: Backend implementation ('torch', 'flash', etc.)
    
    Example:
        @register_model("llama", "torch")
        class LlamaModel(nn.Module):
            ...
    """
    def decorator(model_class: Type[nn.Module]) -> Type[nn.Module]:
        key = (model_type, backend)
        if key in MODEL_REGISTRY:
            logger = get_logger("models")
            logger.warning(f"Overwriting existing model registration: {key}")
        MODEL_REGISTRY[key] = model_class
        return model_class
    return decorator


def get_available_models() -> Dict[str, list]:
    """Get all available models organized by type"""
    models = {}
    for (model_type, backend), model_class in MODEL_REGISTRY.items():
        if model_type not in models:
            models[model_type] = []
        models[model_type].append({
            'backend': backend,
            'class': model_class.__name__,
            'module': model_class.__module__
        })
    return models


def build_model(config, hf_config):
    """
    Build a model based on configuration.
    
    Args:
        config: Main configuration object
        hf_config: HuggingFace model configuration
    
    Returns:
        Instantiated model
    """
    logger = get_logger("models")
    
    # Import model modules to register them
    try:
        importlib.import_module("models.basic_llama")
        logger.debug("Imported basic_llama module")
    except ImportError as e:
        logger.error(f"Failed to import basic_llama module: {e}")
        raise
    
    # Get model type and backend from config
    model_type = hf_config.model_type
    backend = getattr(config.model, 'attention_backend', 'torch')
    
    # Map attention backend to model backend
    backend_map = {
        'flash_attn': 'torch',
        'native_math': 'torch',
        'native_flash': 'torch',
    }
    model_backend = backend_map.get(backend, backend)
    
    # Look up model in registry
    key = (model_type, model_backend)
    if key not in MODEL_REGISTRY:
        available = get_available_models()
        logger.error(f"Model {key} not found in registry")
        logger.info(f"Available models: {available}")
        raise ValueError(f"Model {key} is not registered. Available: {list(MODEL_REGISTRY.keys())}")
    
    model_class = MODEL_REGISTRY[key]
    logger.info(f"Building model: {model_class.__name__} ({model_type}, {model_backend})")
    
    # Instantiate model with configuration
    try:
        # Pass debug flag if available
        debug_enabled = config.debug.enabled if hasattr(config, 'debug') else False
        
        # Note: attention_backend configuration is handled internally by BasicAttention
        # Log the intended backend for informational purposes
        attention_backend = getattr(config.model, 'attention_backend', 'native_math')
        use_flash_attention = getattr(config.model, 'use_flash_attention', False)
        
        # Determine what backend would be used (for logging only)
        if not use_flash_attention:
            attention_backend = 'native_math'  # Force native_math if flash is disabled
        elif attention_backend == 'flash_attn' and not config.training.use_amp:
            # Flash attention requires fp16/bf16, fallback to native if AMP is disabled
            attention_backend = 'native_math'
            logger.warning("Flash attention requires fp16/bf16. Falling back to native_math since AMP is disabled.")
        
        model = model_class(hf_config, debug_enabled=debug_enabled)
        logger.info(f"Model created successfully: {model.__class__.__name__} (backend selection handled internally)")
    except TypeError:
        # Fallback for models that don't accept debug parameter
        try:
            model = model_class(hf_config)
            logger.info(f"Model created successfully (without debug): {model.__class__.__name__}")
        except Exception as e:
            logger.error(f"Failed to instantiate model: {e}")
            raise
    
    # Log model statistics
    total_params = sum(p.numel() for p in model.parameters())
    trainable_params = sum(p.numel() for p in model.parameters() if p.requires_grad)
    logger.info(f"Model parameters: {total_params:,} total, {trainable_params:,} trainable")
    logger.info(f"Model size: {total_params * 4 / 1e9:.2f} GB (fp32)")
    
    return model


def list_registered_models():
    """List all registered models"""
    logger = get_logger("models")
    logger.info("Registered models:")
    for (model_type, backend), model_class in MODEL_REGISTRY.items():
        logger.info(f"  - {model_type} ({backend}): {model_class.__name__}")


def clear_registry():
    """Clear the model registry (useful for testing)"""
    global MODEL_REGISTRY
    MODEL_REGISTRY.clear()