"""
Unified logging system for the training framework.
Provides structured logging with different verbosity levels.
"""

import logging
import sys
import os
from typing import Optional, Union, Any
from pathlib import Path
import torch
import numpy as np
from datetime import datetime


class ColoredFormatter(logging.Formatter):
    """Custom formatter with colors for terminal output"""
    
    def __init__(self, *args, gpu_id=None, **kwargs):
        super().__init__(*args, **kwargs)
        self.gpu_id = gpu_id
        
        # Detect if terminal supports colors
        self.use_colors = (
            hasattr(sys.stdout, 'isatty') and sys.stdout.isatty() and
            os.environ.get('TERM', '') != 'dumb' and
            os.environ.get('NO_COLOR', '') != '1'
        )
        
        if self.use_colors:
            self.COLORS = {
                'DEBUG': '\033[36m',    # Cyan
                'INFO': '\033[32m',     # Green
                'WARNING': '\033[33m',  # Yellow
                'ERROR': '\033[31m',    # Red
                'CRITICAL': '\033[35m', # Magenta
            }
            self.RESET = '\033[0m'
        else:
            # No colors when not supported
            self.COLORS = {
                'DEBUG': '',
                'INFO': '',
                'WARNING': '',
                'ERROR': '',
                'CRITICAL': '',
            }
            self.RESET = ''
    
    def format(self, record):
        log_color = self.COLORS.get(record.levelname, self.RESET)
        # Add GPU ID if available
        if self.gpu_id is not None:
            if self.use_colors:
                record.levelname = f"{log_color}GPU{self.gpu_id}:{record.levelname}{self.RESET}"
            else:
                record.levelname = f"GPU{self.gpu_id}:{record.levelname}"
        else:
            if self.use_colors:
                record.levelname = f"{log_color}{record.levelname}{self.RESET}"
            else:
                # Keep levelname unchanged when no colors
                pass
        return super().format(record)


class TensorLogger:
    """Specialized logger for tensor debugging"""
    
    def __init__(self, logger: logging.Logger):
        self.logger = logger
    
    def log_tensor(self, tensor: torch.Tensor, name: str, level: int = logging.DEBUG):
        """Log tensor statistics and sample values"""
        if not self.logger.isEnabledFor(level):
            return
        
        info = [
            f"[{name}]",
            f"Shape: {tensor.shape}",
            f"Device: {tensor.device}",
            f"Dtype: {tensor.dtype}",
        ]
        
        # Check for special values
        nan_count = torch.isnan(tensor).sum().item()
        inf_count = torch.isinf(tensor).sum().item()
        
        if nan_count > 0 or inf_count > 0:
            info.append(f"⚠️  NaN: {nan_count}, Inf: {inf_count}")
        
        # Statistics
        if tensor.numel() > 0 and not torch.isnan(tensor).all():
            valid_tensor = tensor[~torch.isnan(tensor) & ~torch.isinf(tensor)]
            if valid_tensor.numel() > 0:
                info.extend([
                    f"Min: {valid_tensor.min().item():.6f}",
                    f"Max: {valid_tensor.max().item():.6f}",
                    f"Mean: {valid_tensor.mean().item():.6f}",
                    f"Std: {valid_tensor.std().item():.6f}",
                ])
        
        # Sample values
        if tensor.numel() > 0:
            flat = tensor.flatten()
            sample_size = min(10, flat.numel())
            sample = flat[:sample_size].tolist()
            info.append(f"Sample: {sample}")
        
        self.logger.log(level, " | ".join(info))
    
    def log_gradient(self, module_name: str, grad: torch.Tensor):
        """Log gradient information during backpropagation"""
        self.log_tensor(grad, f"Grad[{module_name}]", logging.DEBUG)
    
    def log_weights(self, module_name: str, weights: torch.Tensor):
        """Log weight matrix information"""
        self.log_tensor(weights, f"Weights[{module_name}]", logging.DEBUG)


class ProgressLogger:
    """Logger for training progress"""
    
    def __init__(self, logger: logging.Logger):
        self.logger = logger
        self.start_time = None
        self.step_times = []
        # Get GPU ID for display
        self.gpu_id = None
        if 'GPU_RANK' in os.environ:
            self.gpu_id = os.environ['GPU_RANK']
        elif 'CUDA_VISIBLE_DEVICES' in os.environ:
            self.gpu_id = os.environ['CUDA_VISIBLE_DEVICES']
        elif torch.cuda.is_available():
            try:
                self.gpu_id = torch.cuda.current_device()
            except:
                pass
    
    def start(self):
        """Start timing"""
        self.start_time = datetime.now()
        gpu_info = f"[GPU {self.gpu_id}] " if self.gpu_id is not None else ""
        self.logger.info(f"{gpu_info}Training started")
    
    def log_step(self, step: int, total_steps: int, loss: float, 
                 learning_rate: float, grad_norm: Optional[float] = None):
        """Log training step progress"""
        if self.start_time:
            elapsed = (datetime.now() - self.start_time).total_seconds()
            steps_per_sec = (step + 1) / elapsed if elapsed > 0 else 0
            eta_seconds = (total_steps - step) / steps_per_sec if steps_per_sec > 0 else 0
            eta_str = f"{int(eta_seconds // 60)}:{int(eta_seconds % 60):02d}"
        else:
            eta_str = "N/A"
        
        # Add GPU ID to progress display
        gpu_info = f"[GPU {self.gpu_id}] " if self.gpu_id is not None else ""
        progress = f"{gpu_info}Step [{step}/{total_steps}]"
        metrics = [
            f"Loss: {loss:.4f}",
            f"LR: {learning_rate:.2e}",
        ]
        
        if grad_norm is not None:
            metrics.append(f"Grad Norm: {grad_norm:.4f}")
        
        metrics.append(f"ETA: {eta_str}")
        
        self.logger.info(f"{progress} | {' | '.join(metrics)}")
    
    def log_epoch(self, epoch: int, total_epochs: int, avg_loss: float):
        """Log epoch summary"""
        gpu_info = f"[GPU {self.gpu_id}] " if self.gpu_id is not None else ""
        self.logger.info(f"{gpu_info}Epoch [{epoch}/{total_epochs}] completed | Avg Loss: {avg_loss:.4f}")


def setup_logger(
    name: str = "training",
    level: Union[str, int] = "INFO",
    log_file: Optional[str] = None,
    console: bool = True,
) -> logging.Logger:
    """
    Set up a logger with file and console handlers.
    
    Args:
        name: Logger name
        level: Logging level (DEBUG, INFO, WARNING, ERROR, CRITICAL)
        log_file: Path to log file (if None, only console output)
        console: Whether to output to console
    
    Returns:
        Configured logger instance
    """
    
    # Get GPU ID from environment or CUDA device
    gpu_id = None
    if 'GPU_RANK' in os.environ:
        gpu_id = os.environ['GPU_RANK']
    elif 'CUDA_VISIBLE_DEVICES' in os.environ:
        gpu_id = os.environ['CUDA_VISIBLE_DEVICES']
    elif torch.cuda.is_available():
        try:
            gpu_id = torch.cuda.current_device()
        except:
            pass
    
    # Create logger
    logger = logging.getLogger(name)
    logger.setLevel(level)
    logger.handlers.clear()  # Clear existing handlers
    
    # Create formatters with GPU ID
    if gpu_id is not None:
        file_formatter = logging.Formatter(
            f'%(asctime)s | GPU{gpu_id} | %(name)s | %(levelname)s | %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )
        console_formatter = ColoredFormatter(
            '%(levelname)s | %(message)s',
            gpu_id=gpu_id
        )
    else:
        file_formatter = logging.Formatter(
            '%(asctime)s | %(name)s | %(levelname)s | %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )
        console_formatter = ColoredFormatter(
            '%(levelname)s | %(message)s'
        )
    
    # Add console handler
    if console:
        console_handler = logging.StreamHandler(sys.stdout)
        console_handler.setFormatter(console_formatter)
        logger.addHandler(console_handler)
    
    # Add file handler
    if log_file:
        log_path = Path(log_file)
        log_path.parent.mkdir(parents=True, exist_ok=True)
        file_handler = logging.FileHandler(log_file)
        file_handler.setFormatter(file_formatter)
        logger.addHandler(file_handler)
    
    return logger


class LoggerManager:
    """Centralized logger management"""
    
    _instance = None
    _loggers = {}
    
    def __new__(cls):
        if cls._instance is None:
            cls._instance = super(LoggerManager, cls).__new__(cls)
        return cls._instance
    
    def get_logger(self, name: str = "main") -> logging.Logger:
        """Get or create a logger"""
        if name not in self._loggers:
            self._loggers[name] = setup_logger(name)
        return self._loggers[name]
    
    def get_tensor_logger(self, name: str = "tensor") -> TensorLogger:
        """Get a tensor logger"""
        return TensorLogger(self.get_logger(name))
    
    def get_progress_logger(self, name: str = "progress") -> ProgressLogger:
        """Get a progress logger"""
        return ProgressLogger(self.get_logger(name))
    
    def set_level(self, level: Union[str, int], logger_name: Optional[str] = None):
        """Set logging level for specific or all loggers"""
        if logger_name:
            if logger_name in self._loggers:
                self._loggers[logger_name].setLevel(level)
        else:
            for logger in self._loggers.values():
                logger.setLevel(level)
    
    def add_file_handler(self, log_file: str, logger_name: Optional[str] = None):
        """Add file handler to specific or all loggers"""
        formatter = logging.Formatter(
            '%(asctime)s | %(name)s | %(levelname)s | %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )
        
        file_handler = logging.FileHandler(log_file)
        file_handler.setFormatter(formatter)
        
        if logger_name:
            if logger_name in self._loggers:
                self._loggers[logger_name].addHandler(file_handler)
        else:
            for logger in self._loggers.values():
                logger.addHandler(file_handler)


# Global logger manager instance
logger_manager = LoggerManager()


# Convenience functions
def get_logger(name: str = "main") -> logging.Logger:
    """Get a logger instance"""
    return logger_manager.get_logger(name)


def get_tensor_logger(name: str = "tensor") -> TensorLogger:
    """Get a tensor logger instance"""
    return logger_manager.get_tensor_logger(name)


def get_progress_logger(name: str = "progress") -> ProgressLogger:
    """Get a progress logger instance"""
    return logger_manager.get_progress_logger(name)


def debug(msg: str, *args, **kwargs):
    """Convenience function for debug logging"""
    get_logger().debug(msg, *args, **kwargs)


def info(msg: str, *args, **kwargs):
    """Convenience function for info logging"""
    get_logger().info(msg, *args, **kwargs)


def warning(msg: str, *args, **kwargs):
    """Convenience function for warning logging"""
    get_logger().warning(msg, *args, **kwargs)


def error(msg: str, *args, **kwargs):
    """Convenience function for error logging"""
    get_logger().error(msg, *args, **kwargs)


def critical(msg: str, *args, **kwargs):
    """Convenience function for critical logging"""
    get_logger().critical(msg, *args, **kwargs)
