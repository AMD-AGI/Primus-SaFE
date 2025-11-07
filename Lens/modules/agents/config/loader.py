"""Configuration loader for GPU Usage Analysis Agent."""

import os
import yaml
from pathlib import Path
from typing import Any, Dict, Optional
import logging

logger = logging.getLogger(__name__)

# 全局配置缓存
_config_cache: Optional[Dict[str, Any]] = None


def load_config(config_path: Optional[str] = None) -> Dict[str, Any]:
    """
    加载配置文件
    
    Args:
        config_path: 配置文件路径，如果不指定则按以下顺序查找：
                    1. 环境变量 CONFIG_FILE
                    2. config/config.local.yaml（本地配置）
                    3. config/config.yaml（默认配置）
    
    Returns:
        配置字典
    """
    global _config_cache
    
    if _config_cache is not None:
        return _config_cache
    
    # 确定配置文件路径
    if config_path is None:
        # 从环境变量获取
        config_path = os.getenv("CONFIG_FILE")
        
        if config_path is None:
            # 自动查找配置文件
            base_dir = Path(__file__).parent
            
            # 优先使用本地配置
            local_config = base_dir / "config.local.yaml"
            default_config = base_dir / "config.yaml"
            
            if local_config.exists():
                config_path = str(local_config)
                logger.info(f"使用本地配置文件: {config_path}")
            elif default_config.exists():
                config_path = str(default_config)
                logger.info(f"使用默认配置文件: {config_path}")
            else:
                logger.warning("未找到配置文件，使用默认值")
                _config_cache = _get_default_config()
                return _config_cache
    
    # 加载配置文件
    try:
        with open(config_path, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)
            logger.info(f"成功加载配置文件: {config_path}")
    except Exception as e:
        logger.error(f"加载配置文件失败: {e}")
        config = _get_default_config()
    
    # 用环境变量覆盖配置
    config = _override_with_env(config)
    
    _config_cache = config
    return config


def _get_default_config() -> Dict[str, Any]:
    """获取默认配置"""
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
    用环境变量覆盖配置
    
    环境变量优先级最高，会覆盖配置文件中的值
    """
    # API 配置
    if os.getenv("API_HOST"):
        config.setdefault("api", {})["host"] = os.getenv("API_HOST")
    if os.getenv("API_PORT"):
        config.setdefault("api", {})["port"] = int(os.getenv("API_PORT"))
    
    # Lens API 配置
    if os.getenv("LENS_API_URL"):
        config.setdefault("lens", {})["api_url"] = os.getenv("LENS_API_URL")
    if os.getenv("CLUSTER_NAME"):
        config.setdefault("lens", {})["cluster_name"] = os.getenv("CLUSTER_NAME")
    if os.getenv("LENS_TIMEOUT"):
        config.setdefault("lens", {})["timeout"] = int(os.getenv("LENS_TIMEOUT"))
    
    # LLM 配置
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
    
    # Agent 配置
    if os.getenv("AGENT_MAX_ITERATIONS"):
        config.setdefault("agent", {})["max_iterations"] = int(os.getenv("AGENT_MAX_ITERATIONS"))
    if os.getenv("AGENT_TIMEOUT"):
        config.setdefault("agent", {})["timeout"] = int(os.getenv("AGENT_TIMEOUT"))
    
    # 日志配置
    if os.getenv("LOG_LEVEL"):
        config.setdefault("logging", {})["level"] = os.getenv("LOG_LEVEL")
    
    return config


def get_config(key_path: str, default: Any = None) -> Any:
    """
    获取配置值
    
    Args:
        key_path: 配置键路径，使用点号分隔，如 "llm.model"
        default: 默认值
    
    Returns:
        配置值
    
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
    """重新加载配置文件"""
    global _config_cache
    _config_cache = None
    return load_config()

