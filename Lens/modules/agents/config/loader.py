"""Configuration loader for GPU Usage Analysis Agent."""

import os
import yaml
from pathlib import Path
from typing import Any, Dict, Optional
import logging

logger = logging.getLogger(__name__)

# Global configuration cache
_config_cache: Optional[Dict[str, Any]] = None


def load_config(config_path: Optional[str] = None) -> Dict[str, Any]:
    """
    Load configuration file
    
    Args:
        config_path: Configuration file path. If not specified, search in the following order:
                    1. Environment variable CONFIG_FILE
                    2. config/config.local.yaml (local configuration)
                    3. config/config.yaml (default configuration)
    
    Returns:
        Configuration dictionary
    """
    global _config_cache
    
    if _config_cache is not None:
        return _config_cache
    
    # Determine configuration file path
    if config_path is None:
        # Get from environment variable
        config_path = os.getenv("CONFIG_FILE")
        
        if config_path is None:
            # Auto-search for configuration file
            base_dir = Path(__file__).parent
            
            # Prefer local configuration
            local_config = base_dir / "config.local.yaml"
            default_config = base_dir / "config.yaml"
            
            if local_config.exists():
                config_path = str(local_config)
                logger.info(f"Using local configuration file: {config_path}")
            elif default_config.exists():
                config_path = str(default_config)
                logger.info(f"Using default configuration file: {config_path}")
            else:
                logger.warning("Configuration file not found, using default values")
                _config_cache = _get_default_config()
                return _config_cache
    
    # Load configuration file
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)
            logger.info(f"Successfully loaded configuration file: {config_path}")
    except Exception as e:
        logger.error(f"Failed to load configuration file: {e}")
        config = _get_default_config()
    
    # Override configuration with environment variables
    config = _override_with_env(config)
    
    _config_cache = config
    return config


def _get_default_config() -> Dict[str, Any]:
    """Get default configuration"""
    return {
        "api": {
            "host": "0.0.0.0",
            "port": 8001,
            "title": "GPU Usage Analysis Agent API",
            "version": "1.0.0",
            "cors": {
                "enabled": True,
                "origins": ["*"]
            }
        },
        "lens": {
            "api_url": "http://localhost:30182",
            "cluster_name": None,
            "timeout": 30
        },
        "llm": {
            "provider": "openai",
            "model": "gpt-4",
            "api_key": "",
            "base_url": None,
            "temperature": 0,
            "max_tokens": 2000
        },
        "agent": {
            "max_iterations": 10,
            "timeout": 120
        },
        "logging": {
            "level": "INFO",
            "format": "json",
            "output": "stdout"
        }
    }


def _override_with_env(config: Dict[str, Any]) -> Dict[str, Any]:
    """
    Override configuration with environment variables
    
    Environment variables have the highest priority and will override values in configuration files
    """
    # API configuration
    if os.getenv("API_HOST"):
        config.setdefault("api", {})["host"] = os.getenv("API_HOST")
    if os.getenv("API_PORT"):
        config.setdefault("api", {})["port"] = int(os.getenv("API_PORT"))
    
    # Lens API configuration
    if os.getenv("LENS_API_URL"):
        config.setdefault("lens", {})["api_url"] = os.getenv("LENS_API_URL")
    if os.getenv("CLUSTER_NAME"):
        config.setdefault("lens", {})["cluster_name"] = os.getenv("CLUSTER_NAME")
    if os.getenv("LENS_TIMEOUT"):
        config.setdefault("lens", {})["timeout"] = int(os.getenv("LENS_TIMEOUT"))
    
    # LLM configuration
    if os.getenv("LLM_PROVIDER"):
        config.setdefault("llm", {})["provider"] = os.getenv("LLM_PROVIDER")
    if os.getenv("LLM_MODEL"):
        config.setdefault("llm", {})["model"] = os.getenv("LLM_MODEL")
    if os.getenv("LLM_API_KEY"):
        config.setdefault("llm", {})["api_key"] = os.getenv("LLM_API_KEY")
    if os.getenv("LLM_BASE_URL"):
        config.setdefault("llm", {})["base_url"] = os.getenv("LLM_BASE_URL")
    if os.getenv("LLM_TEMPERATURE"):
        config.setdefault("llm", {})["temperature"] = float(os.getenv("LLM_TEMPERATURE"))
    if os.getenv("LLM_MAX_TOKENS"):
        config.setdefault("llm", {})["max_tokens"] = int(os.getenv("LLM_MAX_TOKENS"))
    
    # Agent configuration
    if os.getenv("AGENT_MAX_ITERATIONS"):
        config.setdefault("agent", {})["max_iterations"] = int(os.getenv("AGENT_MAX_ITERATIONS"))
    if os.getenv("AGENT_TIMEOUT"):
        config.setdefault("agent", {})["timeout"] = int(os.getenv("AGENT_TIMEOUT"))
    
    # Logging configuration
    if os.getenv("LOG_LEVEL"):
        config.setdefault("logging", {})["level"] = os.getenv("LOG_LEVEL")
    
    # Storage configuration - PostgreSQL
    if os.getenv("PG_HOST"):
        config.setdefault("storage", {}).setdefault("pg", {})["host"] = os.getenv("PG_HOST")
    if os.getenv("PG_PORT"):
        config.setdefault("storage", {}).setdefault("pg", {})["port"] = int(os.getenv("PG_PORT"))
    if os.getenv("PG_DATABASE"):
        config.setdefault("storage", {}).setdefault("pg", {})["database"] = os.getenv("PG_DATABASE")
    if os.getenv("PG_USER"):
        config.setdefault("storage", {}).setdefault("pg", {})["user"] = os.getenv("PG_USER")
    if os.getenv("PG_PASSWORD"):
        config.setdefault("storage", {}).setdefault("pg", {})["password"] = os.getenv("PG_PASSWORD")
    if os.getenv("PG_SCHEMA"):
        config.setdefault("storage", {}).setdefault("pg", {})["schema"] = os.getenv("PG_SCHEMA")
    if os.getenv("PG_SSLMODE"):
        config.setdefault("storage", {}).setdefault("pg", {})["sslmode"] = os.getenv("PG_SSLMODE")
    
    return config


def get_config(key_path: str, default: Any = None) -> Any:
    """
    Get configuration value
    
    Args:
        key_path: Configuration key path, separated by dots, e.g. "llm.model"
        default: Default value
    
    Returns:
        Configuration value
    
    Examples:
        >>> get_config("llm.model")
        "gpt-4"
        >>> get_config("llm.api_key", "default-key")
        "your-api-key"
    """
    config = load_config()
    
    keys = key_path.split(".")
    value = config
    
    for key in keys:
        if isinstance(value, dict):
            value = value.get(key)
            if value is None:
                return default
        else:
            return default
    
    return value if value is not None else default


def reload_config():
    """Reload configuration file"""
    global _config_cache
    _config_cache = None
    return load_config()

