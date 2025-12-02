"""
Trainer module for model training.
Handles training loop, optimization, and checkpointing.
"""

import os
import sys
import time
import traceback
from pathlib import Path
from typing import Optional, Dict, Any

import torch
import torch.nn as nn
from torch.utils.data import DataLoader
from torch.optim import AdamW
from torch.optim.lr_scheduler import CosineAnnealingLR
from torch.amp import autocast, GradScaler

from utils import get_logger, get_progress_logger, get_tensor_logger


class Trainer:
    """Main trainer class for model training"""
    
    def __init__(self, model: nn.Module, train_dataset, config):
        """
        Initialize trainer.
        
        Args:
            model: Model to train
            train_dataset: Training dataset
            config: Configuration object
        """
        self.model = model
        self.train_dataset = train_dataset
        self.config = config
        
        # Set up logging
        self.logger = get_logger("trainer")
        self.progress_logger = get_progress_logger("progress")
        self.tensor_logger = get_tensor_logger("tensor")
        
        # Device setup
        self.device = torch.device(config.system.device)
        if config.system.device == "cuda" and config.system.device_id is not None:
            self.device = torch.device(f"cuda:{config.system.device_id}")
        
        self.model = self.model.to(self.device)
        # Get GPU ID for logging
        import os
        gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', ''))
        gpu_info = f" [GPU {gpu_id}]" if gpu_id else ""
        self.logger.info(f"Model moved to {self.device}{gpu_info}")
        
        # Data loader
        self.train_loader = DataLoader(
            train_dataset,
            batch_size=config.training.batch_size,
            shuffle=True,
            num_workers=config.data.num_workers,
            pin_memory=config.data.pin_memory and self.device.type == "cuda",
            prefetch_factor=config.data.prefetch_factor if config.data.num_workers > 0 else None,
        )
        
        # Optimizer
        self.optimizer = AdamW(
            self.model.parameters(),
            lr=config.training.learning_rate,
            betas=(config.training.adam_beta1, config.training.adam_beta2),
            eps=config.training.adam_epsilon,
            weight_decay=config.training.weight_decay,
        )
        
        # Learning rate scheduler
        self.scheduler = CosineAnnealingLR(
            self.optimizer,
            T_max=config.training.max_steps,
            eta_min=config.training.learning_rate * 0.1,
        )
        
        # Mixed precision training
        self.use_amp = config.training.use_amp and self.device.type == "cuda"
        if self.use_amp:
            # Use new torch.amp.GradScaler API to avoid deprecation warning
            self.scaler = torch.amp.GradScaler(self.device.type)
            # Ensure amp_dtype is a torch.dtype, not a string
            amp_dtype = config.training.amp_dtype
            
            # Debug logging
            self.logger.debug(f"amp_dtype type: {type(amp_dtype)}")
            self.logger.debug(f"amp_dtype value: {amp_dtype}")
            
            # Convert string to torch.dtype if necessary
            if isinstance(amp_dtype, str):
                self.logger.debug(f"Converting amp_dtype string '{amp_dtype}' to torch.dtype")
                if "bfloat16" in str(amp_dtype).lower():
                    self.amp_dtype = torch.bfloat16
                elif "float16" in str(amp_dtype).lower():
                    self.amp_dtype = torch.float16
                else:
                    self.amp_dtype = torch.float32
            elif not isinstance(amp_dtype, torch.dtype):
                # Handle other types (e.g., if it's somehow an object or class)
                self.logger.warning(f"amp_dtype is not torch.dtype: {type(amp_dtype)}, defaulting to float16")
                self.amp_dtype = torch.float16
            else:
                self.amp_dtype = amp_dtype
            
            self.logger.info(f"Using AMP with dtype {self.amp_dtype} (type: {type(self.amp_dtype)})")
        else:
            self.scaler = None
            self.amp_dtype = torch.float32
        
        # Training state
        self.global_step = 0
        self.epoch = 0
        self.best_loss = float('inf')
        self.patience_counter = 0
        
        # Gradient accumulation
        self.grad_accum_steps = config.training.grad_accum_nums
        
        # Checkpointing
        self.checkpoint_dir = Path(config.training.checkpoint_dir)
        self.checkpoint_dir.mkdir(parents=True, exist_ok=True)
    
    def train_step(self) -> float:
        """Execute a single training step"""
        self.model.train()
        
        total_loss = 0.0
        accumulated_steps = 0
        
        # Gradient accumulation loop
        for i, batch in enumerate(self.train_loader):
            if i >= self.grad_accum_steps:
                break
            
            # Move batch to device
            input_ids = batch["input_ids"].to(self.device)
            labels = batch["labels"].to(self.device) if "labels" in batch else input_ids
            
            # Forward pass with mixed precision
            if self.use_amp:
                with autocast(device_type='cuda', dtype=self.amp_dtype):
                    outputs = self.model(input_ids)
                    
                    # Compute loss
                    if hasattr(outputs, 'loss'):
                        loss = outputs.loss
                    elif hasattr(outputs, 'logits'):
                        # Model returns object with logits attribute
                        shift_logits = outputs.logits[..., :-1, :].contiguous()
                        shift_labels = labels[..., 1:].contiguous()
                        loss_fn = nn.CrossEntropyLoss()
                        loss = loss_fn(
                            shift_logits.view(-1, shift_logits.size(-1)),
                            shift_labels.view(-1)
                        )
                    else:
                        # Model returns logits tensor directly
                        logits = outputs
                        shift_logits = logits[..., :-1, :].contiguous()
                        shift_labels = labels[..., 1:].contiguous()
                        loss_fn = nn.CrossEntropyLoss()
                        loss = loss_fn(
                            shift_logits.view(-1, shift_logits.size(-1)),
                            shift_labels.view(-1)
                        )
                    
                    # Scale loss for gradient accumulation
                    loss = loss / self.grad_accum_steps
            else:
                outputs = self.model(input_ids)
                
                # Compute loss
                if hasattr(outputs, 'loss'):
                    loss = outputs.loss
                elif hasattr(outputs, 'logits'):
                    # Model returns object with logits attribute
                    shift_logits = outputs.logits[..., :-1, :].contiguous()
                    shift_labels = labels[..., 1:].contiguous()
                    loss_fn = nn.CrossEntropyLoss()
                    loss = loss_fn(
                        shift_logits.view(-1, shift_logits.size(-1)),
                        shift_labels.view(-1)
                    )
                else:
                    # Model returns logits tensor directly
                    logits = outputs
                    shift_logits = logits[..., :-1, :].contiguous()
                    shift_labels = labels[..., 1:].contiguous()
                    loss_fn = nn.CrossEntropyLoss()
                    loss = loss_fn(
                        shift_logits.view(-1, shift_logits.size(-1)),
                        shift_labels.view(-1)
                    )
                
                loss = loss / self.grad_accum_steps
            
            # Check for NaN in loss
            if torch.isnan(loss):
                self.logger.error("NaN detected in loss!")
                self.handle_nan_loss()
                return float('nan')
            
            # Backward pass
            if self.use_amp:
                self.scaler.scale(loss).backward()
            else:
                loss.backward()
            
            total_loss += loss.item()
            accumulated_steps += 1
        
        # Gradient clipping and optimization
        if accumulated_steps > 0:
            # Gradient clipping
            grad_norm = None
            if self.config.training.max_grad_norm > 0:
                if self.use_amp:
                    self.scaler.unscale_(self.optimizer)
                grad_norm = nn.utils.clip_grad_norm_(
                    self.model.parameters(),
                    self.config.training.max_grad_norm
                )
                
                # Check for NaN gradient norm
                if torch.isnan(grad_norm):
                    gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', ''))
                    gpu_info = f" [GPU {gpu_id}]" if gpu_id else ""
                    
                    # Log error with full context
                    self.logger.error("=" * 80)
                    self.logger.error(f"NaN GRADIENT DETECTED{gpu_info}")
                    self.logger.error("=" * 80)
                    self.logger.error(f"Step: {self.global_step}")
                    self.logger.error(f"Loss: {total_loss}")
                    self.logger.error(f"Gradient Norm: {grad_norm}")
                    self.logger.error("Training stopped due to NaN gradients")
                    
                    # Print debug information if debug is enabled
                    if self.config.debug.enabled:
                        self.print_debug_info(loss=total_loss, print_gradients=True, print_weights=True)
                    
                    # Print call stack for debugging
                    self.logger.error("\nCall Stack:")
                    for line in traceback.format_stack()[:-1]:  # Skip the current frame
                        self.logger.error(line.strip())
                    
                    # Save checkpoint for debugging
                    if self.config.debug.enabled:
                        self.save_checkpoint(f"nan_grad_step_{self.global_step}")
                        self.save_debug_tensors(f"nan_debug_step_{self.global_step}")
                    
                    self.logger.error("=" * 80)
                    self.logger.error(f"EXITING DUE TO NaN{gpu_info}")
                    self.logger.error("=" * 80)
                    
                    # Exit with error
                    sys.exit(1)
            
            # Check gradients if debug enabled
            if self.config.debug.enabled:
                self.check_gradients()
            
            # Optimizer step
            if self.use_amp:
                self.scaler.step(self.optimizer)
                self.scaler.update()
            else:
                self.optimizer.step()
            
            # Clear gradients
            self.optimizer.zero_grad()
            
            # Update learning rate
            self.scheduler.step()
            
            # Log step
            avg_loss = total_loss
            self.progress_logger.log_step(
                self.global_step,
                self.config.training.max_steps,
                avg_loss,
                self.scheduler.get_last_lr()[0],
                grad_norm
            )
        
        return total_loss
    
    def train(self):
        """Main training loop"""
        # Get GPU ID for error messages
        gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', ''))
        self.logger.info("Starting training...")
        self.progress_logger.start()
        
        try:
            while self.global_step < self.config.training.max_steps:
                # Training step
                loss = self.train_step()
                
                # Check for NaN loss
                if torch.isnan(torch.tensor(loss)):
                    gpu_info = f" [GPU {gpu_id}]" if gpu_id else ""
                    
                    # Log error with full context
                    self.logger.error("=" * 80)
                    self.logger.error(f"NaN LOSS DETECTED{gpu_info}")
                    self.logger.error("=" * 80)
                    self.logger.error(f"Step: {self.global_step}")
                    self.logger.error(f"Loss: {loss}")
                    self.logger.error("Training stopped due to NaN loss")
                    
                    # Print debug information if debug is enabled
                    if self.config.debug.enabled:
                        self.print_debug_info(loss=loss, print_gradients=True, print_weights=True)
                    
                    # Print call stack for debugging
                    self.logger.error("\nCall Stack:")
                    for line in traceback.format_stack()[:-1]:  # Skip the current frame
                        self.logger.error(line.strip())
                    
                    # Save checkpoint for debugging
                    if self.config.debug.enabled:
                        self.save_checkpoint(f"nan_loss_step_{self.global_step}")
                        self.save_debug_tensors(f"nan_loss_debug_step_{self.global_step}")
                    
                    self.logger.error("=" * 80)
                    self.logger.error(f"EXITING DUE TO NaN{gpu_info}")
                    self.logger.error("=" * 80)
                    
                    # Exit with error to trigger multi-GPU shutdown
                    sys.exit(1)
                
                # Update step counter
                self.global_step += 1
                
                # Save checkpoint
                if self.global_step % self.config.training.save_interval == 0:
                    self.save_checkpoint(f"step_{self.global_step}")
                
                # Early stopping check
                if loss < self.best_loss:
                    self.best_loss = loss
                    self.patience_counter = 0
                else:
                    self.patience_counter += 1
                    if self.patience_counter >= self.config.training.patience:
                        self.logger.info("Early stopping triggered")
                        break
                
                # Clear cache periodically
                if self.config.system.empty_cache_interval > 0:
                    if self.global_step % self.config.system.empty_cache_interval == 0:
                        if self.device.type == "cuda":
                            torch.cuda.empty_cache()
        
        except KeyboardInterrupt:
            gpu_info = f" [GPU {gpu_id}]" if gpu_id else ""
            self.logger.warning(f"Training interrupted by user{gpu_info}")
            self.save_checkpoint("interrupted")
            raise
        except Exception as e:
            gpu_info = f" [GPU {gpu_id}]" if gpu_id else ""
            self.logger.error(f"Training error{gpu_info}: {e}", exc_info=True)
            self.save_checkpoint("error")
            raise
        finally:
            gpu_info = f" [GPU {gpu_id}]" if gpu_id else ""
            self.logger.info(f"Training ended at step {self.global_step}{gpu_info}")
    
    def print_debug_info(self, loss=None, print_gradients=True, print_weights=True):
        """Print concise debug information about problematic tensors"""
        nan_params = []
        nan_grads = []
        inf_params = []
        inf_grads = []
        
        # First pass: identify problematic tensors
        
        for name, param in self.model.named_parameters():
            if param is not None:
                # Check parameter values
                param_has_nan = torch.isnan(param).any().item()
                param_has_inf = torch.isinf(param).any().item()
                
                if param_has_nan:
                    nan_params.append((name, param))
                if param_has_inf:
                    inf_params.append((name, param))
                
                # Check gradient values
                if param.grad is not None:
                    grad_has_nan = torch.isnan(param.grad).any().item()
                    grad_has_inf = torch.isinf(param.grad).any().item()
                    
                    if grad_has_nan:
                        nan_grads.append((name, param))
                    if grad_has_inf:
                        inf_grads.append((name, param))
        
        # Only print if there are problems
        if nan_grads or inf_grads or nan_params or inf_params:
            self.logger.error("\n" + "="*40)
            self.logger.error("PROBLEMATIC TENSORS:")
            self.logger.error("="*40)
            
            # Show only the first few problematic layers with NaN gradients
            if nan_grads:
                self.logger.error(f"\n⚠️  Gradients with NaN ({len(nan_grads)} layers):")
                for name, param in nan_grads[:3]:  # Show only first 3
                    self.logger.error(f"  • {name} (shape: {param.shape})")
                    grad_flat = param.grad.flatten()
                    num_to_show = min(10, grad_flat.numel())
                    grad_values = [f'{v.item():.4f}' if not torch.isnan(v) else 'NaN' for v in grad_flat[:num_to_show]]
                    self.logger.error(f"    Grad values: {grad_values}")
                    nan_count = torch.isnan(param.grad).sum().item()
                    self.logger.error(f"    NaN count: {nan_count}/{param.grad.numel()}")
            
            # Show parameters with NaN (if any)
            if nan_params:
                self.logger.error(f"\n⚠️  Parameters with NaN ({len(nan_params)} layers):")
                for name, param in nan_params[:2]:  # Show only first 2
                    self.logger.error(f"  • {name} (shape: {param.shape})")
                    param_flat = param.data.flatten()
                    num_to_show = min(10, param_flat.numel())
                    param_values = [f'{v.item():.4f}' if not torch.isnan(v) else 'NaN' for v in param_flat[:num_to_show]]
                    self.logger.error(f"    Param values: {param_values}")
            
            # Simple summary
            self.logger.error("\n" + "="*40)
            if nan_grads:
                self.logger.error(f"Total layers with NaN gradients: {len(nan_grads)}")
                self.logger.error(f"Affected layers: {[name for name, _ in nan_grads[:5]]}")
            if nan_params:
                self.logger.error(f"Total layers with NaN parameters: {len(nan_params)}")
            
            self.logger.error("="*40)
    
    def check_gradients(self):
        """Check gradients for debugging"""
        for name, param in self.model.named_parameters():
            if param.grad is not None:
                # Check for NaN or Inf
                if torch.isnan(param.grad).any():
                    self.logger.error(f"NaN in gradient: {name}")
                    if self.config.debug.enabled:
                        self.tensor_logger.log_gradient(name, param.grad)
                elif torch.isinf(param.grad).any():
                    self.logger.error(f"Inf in gradient: {name}")
                    if self.config.debug.enabled:
                        self.tensor_logger.log_gradient(name, param.grad)
                elif param.grad.abs().max() > 100:
                    self.logger.warning(f"Large gradient in {name}: {param.grad.abs().max():.2f}")
    
    def handle_nan_loss(self):
        """Handle NaN detection in loss"""
        self.logger.error("NaN detected in loss - saving checkpoint for debugging")
        
        # Save debug info if debug is enabled
        if self.config.debug.enabled:
            self.save_checkpoint("nan_debug")
            self.save_debug_state()
    
    def save_checkpoint(self, name: str):
        """Save training checkpoint"""
        checkpoint_path = self.checkpoint_dir / f"{name}.pt"
        
        checkpoint = {
            'global_step': self.global_step,
            'epoch': self.epoch,
            'model_state_dict': self.model.state_dict(),
            'optimizer_state_dict': self.optimizer.state_dict(),
            'scheduler_state_dict': self.scheduler.state_dict(),
            'best_loss': self.best_loss,
            'config': self.config.to_dict(),
        }
        
        if self.use_amp and self.scaler is not None:
            checkpoint['scaler_state_dict'] = self.scaler.state_dict()
        
        torch.save(checkpoint, checkpoint_path)
        self.logger.info(f"Checkpoint saved to {checkpoint_path}")
    
    def load_checkpoint(self, path: str):
        """Load training checkpoint"""
        checkpoint_path = Path(path)
        if not checkpoint_path.exists():
            raise FileNotFoundError(f"Checkpoint not found: {path}")
        
        checkpoint = torch.load(checkpoint_path, map_location=self.device)
        
        self.model.load_state_dict(checkpoint['model_state_dict'])
        self.optimizer.load_state_dict(checkpoint['optimizer_state_dict'])
        self.scheduler.load_state_dict(checkpoint['scheduler_state_dict'])
        
        self.global_step = checkpoint['global_step']
        self.epoch = checkpoint.get('epoch', 0)
        self.best_loss = checkpoint.get('best_loss', float('inf'))
        
        if self.use_amp and 'scaler_state_dict' in checkpoint:
            self.scaler.load_state_dict(checkpoint['scaler_state_dict'])
        
        self.logger.info(f"Checkpoint loaded from {path}")
        self.logger.info(f"Resuming from step {self.global_step}")
    
    def save_debug_tensors(self, name: str):
        """Save model tensors and gradients for debugging"""
        import pickle
        debug_dir = self.checkpoint_dir / "debug"
        debug_dir.mkdir(parents=True, exist_ok=True)
        
        debug_data = {
            'step': self.global_step,
            'parameters': {},
            'gradients': {},
            'buffers': {},
        }
        
        # Save parameters and gradients
        for param_name, param in self.model.named_parameters():
            debug_data['parameters'][param_name] = param.detach().cpu().numpy()
            if param.grad is not None:
                debug_data['gradients'][param_name] = param.grad.detach().cpu().numpy()
        
        # Save buffers (like running mean/var in batch norm)
        for buffer_name, buffer in self.model.named_buffers():
            debug_data['buffers'][buffer_name] = buffer.detach().cpu().numpy()
        
        # Save to file
        debug_path = debug_dir / f"{name}.pkl"
        with open(debug_path, 'wb') as f:
            pickle.dump(debug_data, f)
        
        self.logger.info(f"Debug tensors saved to {debug_path}")
    
    def save_debug_state(self):
        """Save debug information for NaN debugging"""
        debug_dir = self.checkpoint_dir / "debug"
        debug_dir.mkdir(exist_ok=True)
        
        timestamp = time.strftime("%Y%m%d_%H%M%S")
        debug_path = debug_dir / f"debug_{timestamp}.pt"
        
        debug_state = {
            'model_state': self.model.state_dict(),
            'gradients': {
                name: param.grad.clone() if param.grad is not None else None
                for name, param in self.model.named_parameters()
            },
            'optimizer_state': self.optimizer.state_dict(),
            'global_step': self.global_step,
        }
        
        torch.save(debug_state, debug_path)
        self.logger.info(f"Debug state saved to {debug_path}")