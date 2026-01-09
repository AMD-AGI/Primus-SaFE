# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
Logger Module - Unified logging output control
"""
import os
import sys
from typing import Any


# Read debug switch from environment variable, defaults to false
_DEBUG_ENABLED = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false").lower() in ("true", "1", "yes")


def is_debug_enabled() -> bool:
    """
    Check if debug mode is enabled
    
    Returns:
        bool: True if debug mode is enabled, otherwise False
    """
    return _DEBUG_ENABLED


def debug_log(message: str, *args, **kwargs):
    """
    Print debug log (only when debug mode is enabled)
    
    Args:
        message: Log message
        *args: Additional arguments passed to print
        **kwargs: Keyword arguments passed to print
    
    Environment Variables:
        PRIMUS_LENS_WANDB_DEBUG: Set to "true" to enable debug logging, defaults to "false"
    
    Examples:
        >>> # Set environment variable
        >>> # export PRIMUS_LENS_WANDB_DEBUG=true
        >>> debug_log("[Primus Lens] Starting...")
        [Primus Lens] Starting...
        
        >>> # When unset or set to false, nothing is printed
        >>> # export PRIMUS_LENS_WANDB_DEBUG=false
        >>> debug_log("[Primus Lens] Debug info")
        # (nothing will be printed)
    """
    if _DEBUG_ENABLED:
        print(message, *args, **kwargs)


def info_log(message: str, *args, **kwargs):
    """
    Print info log (only when debug mode is enabled)
    This is an alias for debug_log, used for semantic clarity
    
    Args:
        message: Log message
        *args: Additional arguments passed to print
        **kwargs: Keyword arguments passed to print
    """
    debug_log(message, *args, **kwargs)


def error_log(message: str, *args, **kwargs):
    """
    Print error log (always prints, not affected by debug switch)
    
    Args:
        message: Error message
        *args: Additional arguments passed to print
        **kwargs: Keyword arguments passed to print
    """
    print(message, *args, file=kwargs.pop('file', sys.stderr), **kwargs)


def warning_log(message: str, *args, **kwargs):
    """
    Print warning log (only when debug mode is enabled)
    
    Args:
        message: Warning message
        *args: Additional arguments passed to print
        **kwargs: Keyword arguments passed to print
    """
    debug_log(message, *args, **kwargs)


# For backward compatibility, provide a function to reload configuration
def reload_config():
    """
    Reload logging configuration (mainly used for testing)
    """
    global _DEBUG_ENABLED
    _DEBUG_ENABLED = os.environ.get("PRIMUS_LENS_WANDB_DEBUG", "false").lower() in ("true", "1", "yes")

