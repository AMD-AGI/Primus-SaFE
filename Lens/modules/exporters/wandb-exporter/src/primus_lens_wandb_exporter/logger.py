"""
Logger Module - 统一的日志输出控制
"""
import os
import sys
from typing import Any


# 从环境变量读取 debug 开关，默认为 false
_DEBUG_ENABLED = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false").lower() in ("true", "1", "yes")


def is_debug_enabled() -> bool:
    """
    检查是否启用了 debug 模式
    
    Returns:
        bool: 如果启用了 debug 模式返回 True，否则返回 False
    """
    return _DEBUG_ENABLED


def debug_log(message: str, *args, **kwargs):
    """
    打印 debug 日志（仅在 debug 模式启用时）
    
    Args:
        message: 日志消息
        *args: 传递给 print 的额外参数
        **kwargs: 传递给 print 的关键字参数
    
    Environment Variables:
        PRIMUS_LENS_WANDB_DEBUG: 设置为 "true" 启用 debug 日志，默认为 "false"
    
    Examples:
        >>> # 设置环境变量
        >>> # export PRIMUS_LENS_WANDB_DEBUG=true
        >>> debug_log("[Primus Lens] Starting...")
        [Primus Lens] Starting...
        
        >>> # 未设置或设置为 false 时不打印
        >>> # export PRIMUS_LENS_WANDB_DEBUG=false
        >>> debug_log("[Primus Lens] Debug info")
        # (不会打印任何内容)
    """
    if _DEBUG_ENABLED:
        print(message, *args, **kwargs)


def info_log(message: str, *args, **kwargs):
    """
    打印信息日志（仅在 debug 模式启用时）
    这是 debug_log 的别名，用于语义上更清晰
    
    Args:
        message: 日志消息
        *args: 传递给 print 的额外参数
        **kwargs: 传递给 print 的关键字参数
    """
    debug_log(message, *args, **kwargs)


def error_log(message: str, *args, **kwargs):
    """
    打印错误日志（总是打印，不受 debug 开关影响）
    
    Args:
        message: 错误消息
        *args: 传递给 print 的额外参数
        **kwargs: 传递给 print 的关键字参数
    """
    print(message, *args, file=kwargs.pop('file', sys.stderr), **kwargs)


def warning_log(message: str, *args, **kwargs):
    """
    打印警告日志（仅在 debug 模式启用时）
    
    Args:
        message: 警告消息
        *args: 传递给 print 的额外参数
        **kwargs: 传递给 print 的关键字参数
    """
    debug_log(message, *args, **kwargs)


# 为了向后兼容，提供一个函数来重新加载配置
def reload_config():
    """
    重新加载日志配置（主要用于测试）
    """
    global _DEBUG_ENABLED
    _DEBUG_ENABLED = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false").lower() in ("true", "1", "yes")

