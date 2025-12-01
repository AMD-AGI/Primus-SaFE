###############################################################################
# Copyright (c) 2025 Meta Platforms, Inc. and affiliates.
# All rights reserved.
#
# Modification Copyright© 2025 Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

import torch
import torch.nn as nn
import torch.nn.functional as F
from torch.nn.attention import SDPBackend, sdpa_kernel
from transformers import LlamaConfig
import sys
import traceback
import math

from .build_model import register_model
from .rope import apply_rotary_emb, precompute_freqs_cis

# Global flag to enable debug mode when NaN is detected
NAN_DEBUG_ENABLED = False

def check_nan(tensor, name="tensor", location="", debug_info_callback=None):
    """Check if tensor contains NaN values, exit immediately if found"""
    if torch.isnan(tensor).any():
        global NAN_DEBUG_ENABLED
        print("\n" + "="*80)
        print(f"[NaN DETECTED - ENABLING DEBUG MODE] {location} - {name}")
        print("="*80)
        
        # Enable debug mode globally
        if not NAN_DEBUG_ENABLED:
            NAN_DEBUG_ENABLED = True
            print("[INFO] Debug mode automatically enabled for detailed diagnostics")
        print(f"  Tensor shape: {tensor.shape}")
        print(f"  Tensor dtype: {tensor.dtype}")
        print(f"  NaN count: {torch.isnan(tensor).sum().item()}")
        print(f"  Total elements: {tensor.numel()}")
        print(f"  NaN ratio: {torch.isnan(tensor).sum().item() / tensor.numel():.4%}")
        
        # Print sample of tensor values
        print(f"\n  Tensor sample (first 30 values):")
        flat_tensor = tensor.flatten()
        sample_values = flat_tensor[:30].tolist()
        for i in range(0, len(sample_values), 10):
            print(f"    [{i:4d}-{i+9:4d}]: {sample_values[i:i+10]}")
        
        # Find and show where NaN values are
        nan_mask = torch.isnan(tensor)
        nan_indices = torch.where(nan_mask.flatten())[0]
        if len(nan_indices) > 0:
            print(f"\n  NaN locations (first 20 indices): {nan_indices[:20].tolist()}")
            
            # Show neighboring values around first NaN
            first_nan_idx = nan_indices[0].item()
            start_idx = max(0, first_nan_idx - 5)
            end_idx = min(tensor.numel(), first_nan_idx + 6)
            print(f"\n  Values around first NaN (index {first_nan_idx}):")
            neighbor_values = flat_tensor[start_idx:end_idx].tolist()
            print(f"    Indices [{start_idx}-{end_idx-1}]: {neighbor_values}")
        
        # Check for Inf values
        inf_count = torch.isinf(tensor).sum().item()
        if inf_count > 0:
            print(f"\n  [WARNING] Also found {inf_count} Inf values!")
            inf_indices = torch.where(torch.isinf(tensor.flatten()))[0]
            print(f"  Inf locations (first 10): {inf_indices[:10].tolist()}")
            
            # Show some inf values
            inf_values = tensor.flatten()[inf_indices[:5]]
            print(f"  Inf values (first 5): {inf_values.tolist()}")
        
        # Print additional debug information
        if (~torch.isnan(tensor) & ~torch.isinf(tensor)).any():
            valid_tensor = tensor[~torch.isnan(tensor) & ~torch.isinf(tensor)]
            print(f"\n  Statistics (non-NaN/non-Inf values):")
            print(f"    Min value: {valid_tensor.min().item():.6e}")
            print(f"    Max value: {valid_tensor.max().item():.6e}")  
            print(f"    Mean value: {valid_tensor.mean().item():.6e}")
            print(f"    Std value: {valid_tensor.std().item():.6e}")
            
            # Check for extreme values
            extreme_threshold = 1e6
            extreme_count = (valid_tensor.abs() > extreme_threshold).sum().item()
            if extreme_count > 0:
                print(f"    [WARNING] {extreme_count} values with abs > {extreme_threshold:.0e}")
                extreme_vals = valid_tensor[valid_tensor.abs() > extreme_threshold][:5]
                print(f"    Extreme values (first 5): {extreme_vals.tolist()}")
        
        # Print call stack for debugging
        print("\n" + "="*80)
        print("[CALL STACK]")
        print("="*80)
        traceback.print_stack()
        
        # Call debug info callback if provided to print additional context
        if debug_info_callback is not None and callable(debug_info_callback):
            print("\n[DEBUG] Additional context information:")
            print("="*80)
            debug_info_callback()
            print("="*80)
        
        print("\n" + "="*80)
        print("[EXITING] Program terminated due to NaN detection")
        print("="*80)
        
        # Exit program with error code 1
        sys.exit(1)
    return False


def check_grad_nan(grad, name="gradient", location=""):
    """Check if gradient contains NaN values"""
    if grad is not None and torch.isnan(grad).any():
        print("\n" + "="*80)
        print(f"[GRAD NaN DETECTED - EXITING] {location} - {name}")
        print("="*80)
        print(f"  Grad shape: {grad.shape}")
        print(f"  Grad dtype: {grad.dtype}")
        print(f"  NaN count: {torch.isnan(grad).sum().item()}")
        print(f"  Total elements: {grad.numel()}")
        print(f"  NaN ratio: {torch.isnan(grad).sum().item() / grad.numel():.4%}")
        
        # Print sample of gradient values
        print(f"\n  Gradient tensor sample (first 30 values):")
        flat_grad = grad.flatten()
        sample_values = flat_grad[:30].tolist()
        for i in range(0, len(sample_values), 10):
            print(f"    [{i:4d}-{i+9:4d}]: {sample_values[i:i+10]}")
        
        # Find and show where NaN values are
        nan_mask = torch.isnan(grad)
        nan_indices = torch.where(nan_mask.flatten())[0]
        if len(nan_indices) > 0:
            print(f"\n  NaN locations (first 20 indices): {nan_indices[:20].tolist()}")
            
            # Show pattern of NaN distribution
            if grad.dim() > 1:
                # For 2D or higher, show which rows/columns have NaN
                if grad.dim() == 2:
                    nan_rows = torch.any(nan_mask, dim=1)
                    nan_cols = torch.any(nan_mask, dim=0)
                    print(f"  Rows with NaN: {torch.where(nan_rows)[0][:10].tolist()} (first 10)")
                    print(f"  Cols with NaN: {torch.where(nan_cols)[0][:10].tolist()} (first 10)")
                elif grad.dim() == 3:
                    # For 3D tensors (batch, seq, hidden)
                    nan_batch = torch.any(nan_mask.view(grad.shape[0], -1), dim=1)
                    print(f"  Batches with NaN: {torch.where(nan_batch)[0].tolist()}")
        
        # Check for Inf values as well
        inf_count = torch.isinf(grad).sum().item()
        if inf_count > 0:
            print(f"\n  [WARNING] Also found {inf_count} Inf values in gradient!")
            inf_indices = torch.where(torch.isinf(grad.flatten()))[0]
            print(f"  Inf locations (first 10): {inf_indices[:10].tolist()}")
        
        # Print gradient statistics
        if (~torch.isnan(grad) & ~torch.isinf(grad)).any():
            valid_grad = grad[~torch.isnan(grad) & ~torch.isinf(grad)]
            print(f"\n  Statistics (non-NaN/non-Inf values):")
            print(f"    Min grad: {valid_grad.min().item():.6e}")
            print(f"    Max grad: {valid_grad.max().item():.6e}")
            print(f"    Mean grad: {valid_grad.mean().item():.6e}")
            print(f"    Std grad: {valid_grad.std().item():.6e}")
            
            # Show percentiles (sample if tensor is too large)
            if valid_grad.numel() > 100:
                # Sample the tensor if it's too large for quantile
                if valid_grad.numel() > 10000000:  # 10M elements
                    # Randomly sample 1M elements for percentile calculation
                    sample_size = min(1000000, valid_grad.numel())
                    indices = torch.randperm(valid_grad.numel(), device=valid_grad.device)[:sample_size]
                    sampled_grad = valid_grad.flatten()[indices]
                    percentiles = torch.quantile(sampled_grad.float(), torch.tensor([0.01, 0.25, 0.5, 0.75, 0.99]).to(valid_grad.device))
                    print(f"    Percentiles [1%, 25%, 50%, 75%, 99%] (sampled): {percentiles.tolist()}")
                else:
                    percentiles = torch.quantile(valid_grad.float(), torch.tensor([0.01, 0.25, 0.5, 0.75, 0.99]).to(valid_grad.device))
                    print(f"    Percentiles [1%, 25%, 50%, 75%, 99%]: {percentiles.tolist()}")
        
    print("\n" + "="*80)
    print("[CALL STACK]")
    print("="*80)
    traceback.print_stack()
    
    print("="*80)
    print("[EXITING] Program terminated due to gradient NaN detection")
    print("="*80)
    
    sys.exit(1)
    return False


def backward_hook(module, grad_input, grad_output, module_name=""):
    """Backward hook to check gradients during backpropagation"""
    has_nan = False
    
    # Check grad_output (gradients w.r.t. output)
    if grad_output is not None:
        for i, grad in enumerate(grad_output):
            if grad is not None:
                # Check for inf values first
                if torch.isinf(grad).any():
                    print(f"\n[ERROR] Inf detected in {module_name} grad_output[{i}]")
                    inf_count = torch.isinf(grad).sum().item()
                    print(f"  Inf count: {inf_count}/{grad.numel()}")
                    print(f"  Max abs value: {grad[~torch.isinf(grad)].abs().max().item() if (~torch.isinf(grad)).any() else 'All inf'}")
                    
                    # Print sample of the gradient tensor
                    print(f"\n  Sample of grad_output[{i}] (first 10 values):")
                    flat_grad = grad.flatten()
                    print(f"  {flat_grad[:10].tolist()}")
                    
                    # Find location of inf values
                    inf_indices = torch.where(torch.isinf(grad.flatten()))[0]
                    if len(inf_indices) > 0:
                        print(f"  Inf at indices: {inf_indices[:10].tolist()} (showing first 10)")
                
                # Check for NaN
                if torch.isnan(grad).any():
                    has_nan = True
                    print(f"\n[NaN FOUND] in {module_name} grad_output[{i}]")
                    nan_count = torch.isnan(grad).sum().item()
                    print(f"  Shape: {grad.shape}")
                    print(f"  NaN count: {nan_count}/{grad.numel()}")
                    
                    # Print sample of the gradient tensor
                    print(f"\n  Sample of grad_output[{i}] (first 20 values):")
                    flat_grad = grad.flatten()
                    print(f"  {flat_grad[:20].tolist()}")
                    
                    # Find location of NaN values
                    nan_indices = torch.where(torch.isnan(grad.flatten()))[0]
                    if len(nan_indices) > 0:
                        print(f"  NaN at indices: {nan_indices[:10].tolist()} (showing first 10)")
                    
                    # Print statistics of non-NaN values
                    if (~torch.isnan(grad)).any():
                        print(f"\n  Non-NaN statistics:")
                        print(f"    Min: {grad[~torch.isnan(grad)].min().item():.6e}")
                        print(f"    Max: {grad[~torch.isnan(grad)].max().item():.6e}")
                        print(f"    Mean: {grad[~torch.isnan(grad)].mean().item():.6e}")
                        print(f"    Std: {grad[~torch.isnan(grad)].std().item():.6e}")
                
                # Print statistics for large gradients
                elif grad.abs().max() > 1000:
                    print(f"[WARNING] Large gradient detected in {module_name} grad_output[{i}]: max={grad.abs().max().item():.2e}")
                    
                    # Print sample of large values
                    large_mask = grad.abs() > 1000
                    if large_mask.any():
                        large_values = grad[large_mask].flatten()[:10]
                        print(f"  Sample large values: {large_values.tolist()}")
    
    # Check grad_input (gradients w.r.t. input)
    if grad_input is not None:
        for i, grad in enumerate(grad_input):
            if grad is not None:
                # Check for inf values first
                if torch.isinf(grad).any():
                    print(f"\n[ERROR] Inf detected in {module_name} grad_input[{i}]")
                    inf_count = torch.isinf(grad).sum().item()
                    print(f"  Inf count: {inf_count}/{grad.numel()}")
                    print(f"  Max abs value: {grad[~torch.isinf(grad)].abs().max().item() if (~torch.isinf(grad)).any() else 'All inf'}")
                    
                    # Print sample of the gradient tensor
                    print(f"\n  Sample of grad_input[{i}] (first 10 values):")
                    flat_grad = grad.flatten()
                    print(f"  {flat_grad[:10].tolist()}")
                
                # Check for NaN
                if torch.isnan(grad).any():
                    has_nan = True
                    print(f"\n[NaN FOUND] in {module_name} grad_input[{i}]")
                    nan_count = torch.isnan(grad).sum().item()
                    print(f"  Shape: {grad.shape}")
                    print(f"  NaN count: {nan_count}/{grad.numel()}")
                    
                    # Print sample of the gradient tensor
                    print(f"\n  Sample of grad_input[{i}] (first 20 values):")
                    flat_grad = grad.flatten()
                    print(f"  {flat_grad[:20].tolist()}")
                    
                    # Find location of NaN values
                    nan_indices = torch.where(torch.isnan(grad.flatten()))[0]
                    if len(nan_indices) > 0:
                        print(f"  NaN at indices: {nan_indices[:10].tolist()} (showing first 10)")
                    
                    # Print statistics of non-NaN values
                    if (~torch.isnan(grad)).any():
                        print(f"\n  Non-NaN statistics:")
                        print(f"    Min: {grad[~torch.isnan(grad)].min().item():.6e}")
                        print(f"    Max: {grad[~torch.isnan(grad)].max().item():.6e}")
                        print(f"    Mean: {grad[~torch.isnan(grad)].mean().item():.6e}")
                        print(f"    Std: {grad[~torch.isnan(grad)].std().item():.6e}")
                
                # Print statistics for large gradients
                elif grad.abs().max() > 1000:
                    print(f"[WARNING] Large gradient detected in {module_name} grad_input[{i}]: max={grad.abs().max().item():.2e}")
                    
                    # Print sample of large values
                    large_mask = grad.abs() > 1000
                    if large_mask.any():
                        large_values = grad[large_mask].flatten()[:10]
                        print(f"  Sample large values: {large_values.tolist()}")
    
    # If NaN was found, print module weights for debugging
    if has_nan:
        print(f"\n[DEBUG] Module {module_name} parameters:")
        if hasattr(module, 'weight'):
            weight = module.weight
            print(f"  Weight shape: {weight.shape}")
            print(f"  Weight has NaN: {torch.isnan(weight).any()}")
            print(f"  Weight has Inf: {torch.isinf(weight).any()}")
            if not torch.isnan(weight).any() and not torch.isinf(weight).any():
                print(f"  Weight min: {weight.min().item():.6e}")
                print(f"  Weight max: {weight.max().item():.6e}")
                print(f"  Weight mean: {weight.mean().item():.6e}")
                print(f"  Weight std: {weight.std().item():.6e}")
            
            # Print sample of weights
            print(f"  Weight sample (first 10 values): {weight.flatten()[:10].tolist()}")
        
        if hasattr(module, 'bias') and module.bias is not None:
            bias = module.bias
            print(f"  Bias shape: {bias.shape}")
            print(f"  Bias has NaN: {torch.isnan(bias).any()}")
            print(f"  Bias has Inf: {torch.isinf(bias).any()}")
            if not torch.isnan(bias).any() and not torch.isinf(bias).any():
                print(f"  Bias values (first 10): {bias.flatten()[:10].tolist()}")
        
        # Call check_grad_nan to trigger exit
        check_grad_nan(grad, f"grad with NaN", module_name)


def repeat_kv(x: torch.Tensor, n_rep: int) -> torch.Tensor:
    """torch.repeat_interleave(x, dim=2, repeats=n_rep)"""
    bs, slen, n_kv_heads, head_dim = x.shape
    if n_rep == 1:
        return x
    return (
        torch.unsqueeze(x, dim=3)
        .expand(bs, slen, n_kv_heads, n_rep, head_dim)
        .reshape(bs, slen, n_kv_heads * n_rep, head_dim)
    )


def repeat_kv_for_sdpa(x: torch.Tensor, n_rep: int) -> torch.Tensor:
    """Repeat KV heads for scaled_dot_product_attention (expects bs, n_heads, seqlen, head_dim)"""
    bs, n_kv_heads, slen, head_dim = x.shape
    if n_rep == 1:
        return x
    return (
        x.unsqueeze(2)  # (bs, n_kv_heads, 1, seqlen, head_dim)
        .expand(bs, n_kv_heads, n_rep, slen, head_dim)
        .reshape(bs, n_kv_heads * n_rep, slen, head_dim)
    )


# ===== Basic Transformer Layer (Torch Native) =====
class BasicAttention(torch.nn.Module):
    def __init__(
        self,
        config: LlamaConfig,
        layer_id: int = None,
        debug_enabled: bool = False,
    ):
        super().__init__()
        self.debug_enabled = debug_enabled
        self.hidden_size = config.hidden_size
        self.n_heads = config.num_attention_heads
        self.n_kv_heads = config.num_key_value_heads
        self.head_dim = self.hidden_size // self.n_heads
        self.n_rep = self.n_heads // self.n_kv_heads
        
        # Always try to use flash attention first
        self.attn_backend = 'flash_attn'
        # Only print backend info once for the first layer in debug mode
        if layer_id == 0 and self.debug_enabled:
            import os
            gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', ''))
            gpu_info = f"[GPU {gpu_id}] " if gpu_id else ""
            print(f"{gpu_info}[INFO] Using attention backend: {self.attn_backend}")

        self.wq = nn.Linear(self.hidden_size, self.n_heads * self.head_dim, bias=False)
        self.wk = nn.Linear(self.hidden_size, self.n_kv_heads * self.head_dim, bias=False)
        self.wv = nn.Linear(self.hidden_size, self.n_kv_heads * self.head_dim, bias=False)
        self.wo = nn.Linear(self.n_heads * self.head_dim, self.hidden_size, bias=False)

    def forward(
        self,
        x: torch.Tensor,
        freqs_cis: torch.Tensor,
    ):
        bs, seqlen, _ = x.shape
        
        # Check for issues in input
        x_nan = torch.isnan(x).sum().item()
        x_inf = torch.isinf(x).sum().item()
        x_extreme = (x.abs() > 100).sum().item()
        
        if self.debug_enabled and (x_nan > 0 or x_inf > 0 or x_extreme > 0):
            print(f"\n[WARNING] Input has issues:")
            if x_nan > 0:
                print(f"  NaN: {x_nan} values")
            if x_inf > 0:
                print(f"  Inf: {x_inf} values")
            if x_extreme > 0:
                print(f"  Extreme (>100): {x_extreme} values")
        
        # Create debug callback to print tensor state
        def debug_callback():
            # print(f"\n[DEBUG INFO] Current tensor states:")
            # print(f"  Input shape: {x.shape}, dtype: {x.dtype}")
            # print(f"  Input min: {x.min().item():.6f}, max: {x.max().item():.6f}")
            # print(f"  Input mean: {x.mean().item():.6f}, std: {x.std().item():.6f}")
            if hasattr(self, '_last_forward_state'):
                print(f"\n[DEBUG INFO] Previous forward state:")
                for key, value in self._last_forward_state.items():
                    print(f"  {key}: {value}")
        
        check_nan(x, "input", "BasicAttention", debug_callback)
        
        
        # QKV
        xq, xk, xv = self.wq(x), self.wk(x), self.wv(x)
        
        
        # Store state for debugging
        self._last_forward_state = {
            'query_shape': xq.shape,
            'key_shape': xk.shape,
            'value_shape': xv.shape,
            'query_stats': f"min={xq.min().item():.4f}, max={xq.max().item():.4f}",
            'key_stats': f"min={xk.min().item():.4f}, max={xk.max().item():.4f}",
            'value_stats': f"min={xv.min().item():.4f}, max={xv.max().item():.4f}",
        }
        
        check_nan(xq, "query (after wq)", "BasicAttention")
        check_nan(xk, "key (after wk)", "BasicAttention")
        check_nan(xv, "value (after wv)", "BasicAttention")
        
        xq = xq.view(bs, seqlen, -1, self.head_dim)
        xk = xk.view(bs, seqlen, -1, self.head_dim)
        xv = xv.view(bs, seqlen, -1, self.head_dim)

        # ROPE
        xq, xk = apply_rotary_emb(xq, xk, freqs_cis=freqs_cis)
        check_nan(xq, "query (after ROPE)", "BasicAttention")
        check_nan(xk, "key (after ROPE)", "BasicAttention")
        

        # Attention computation
        if self.attn_backend == 'flash_attn':
            # Original flash_attn implementation (requires flash_attn package)
            # Note: Flash attention requires fp16 or bf16
            try:
                from flash_attn import flash_attn_func
                
                # Convert to bf16 for flash attention if needed
                orig_dtype = xq.dtype
                if orig_dtype not in [torch.float16, torch.bfloat16]:
                    xq = xq.to(torch.bfloat16)
                    xk = xk.to(torch.bfloat16)
                    xv = xv.to(torch.bfloat16)
                
                attn_output = flash_attn_func(xq, xk, xv, causal=True)
                
                # Convert back to original dtype if needed
                if orig_dtype not in [torch.float16, torch.bfloat16]:
                    attn_output = attn_output.to(orig_dtype)
                
                
                # Check for NaN/Inf
                if self.debug_enabled:
                    nan_count = torch.isnan(attn_output).sum().item()
                    inf_count = torch.isinf(attn_output).sum().item()
                    if nan_count > 0 or inf_count > 0:
                        print(f"\n[ERROR] Found NaN: {nan_count}, Inf: {inf_count} in attention output!")
                        # Show where NaN/Inf are located
                        if nan_count > 0:
                            nan_indices = torch.where(torch.isnan(attn_output.flatten()))[0][:10]
                            print(f"  NaN at indices (first 10): {nan_indices.tolist()}")
                        if inf_count > 0:
                            inf_indices = torch.where(torch.isinf(attn_output.flatten()))[0][:10]
                            print(f"  Inf at indices (first 10): {inf_indices.tolist()}")
                    
                
            except ImportError as e:
                if self.debug_enabled:
                    print(f"[WARNING] flash_attn not available ({e}), falling back to native flash attention in PyTorch")
                self.attn_backend = 'native_flash'
            except RuntimeError as e:
                if self.debug_enabled:
                    print(f"[WARNING] flash_attn failed ({e}), falling back to native flash attention in PyTorch")
                self.attn_backend = 'native_flash'
        
        if self.attn_backend in ['native_math', 'native_flash']:
            # Using torch native scaled_dot_product_attention
            orig_dtype = xq.dtype
            
            # For native_flash, also convert to bf16 if needed
            if self.attn_backend == 'native_flash' and orig_dtype not in [torch.float16, torch.bfloat16]:
                xq = xq.to(torch.bfloat16)
                xk = xk.to(torch.bfloat16)
                xv = xv.to(torch.bfloat16)
            
            # Convert from (bs, seqlen, n_heads, head_dim) to (bs, n_heads, seqlen, head_dim)
            xq = xq.transpose(1, 2)
            xk = xk.transpose(1, 2)
            xv = xv.transpose(1, 2)
            
            # Check for extreme values before attention
            if self.debug_enabled:
                q_max = xq.abs().max().item()
                k_max = xk.abs().max().item()
                v_max = xv.abs().max().item()
                if q_max > 10 or k_max > 10 or v_max > 10:
                    print(f"[WARNING] Large values before SDPA - Q max: {q_max:.4f}, K max: {k_max:.4f}, V max: {v_max:.4f}")
            
            # Apply GQA (Group Query Attention) if needed
            if self.n_rep > 1:
                # Repeat k and v to match the number of query heads
                xk = repeat_kv_for_sdpa(xk, self.n_rep)
                xv = repeat_kv_for_sdpa(xv, self.n_rep)
            
            # Compute scale factor
            scale = 1.0 / math.sqrt(self.head_dim)
            
            # Compute attention scores
            scores = torch.matmul(xq, xk.transpose(-2, -1)) * scale  # (bs, n_heads, seqlen, seqlen)
            
            # Check for extreme scores
            extreme_scores = (scores.abs() > 10).sum().item()
            if self.debug_enabled and extreme_scores > 0:
                print(f"  [WARNING] {extreme_scores} extreme scores (>10) found!")
                print(f"  Max score value: {scores.max().item():.2f}")
                print(f"  Min score value: {scores.min().item():.2f}")
            
            # Choose backend
            if self.attn_backend == 'native_math':
                # MATH backend is more numerically stable
                backends = [SDPBackend.MATH]
            else:  # native_flash
                # Try FLASH_ATTENTION backend (might be less stable)
                backends = [SDPBackend.FLASH_ATTENTION, SDPBackend.EFFICIENT_ATTENTION, SDPBackend.MATH]
            
            if self.debug_enabled:
                print(f"\n[INFO] Using {self.attn_backend} for attention computation")
                print(f"  Scale factor: {scale:.6f}")
                print(f"  Q shape: {xq.shape}, K shape: {xk.shape}, V shape: {xv.shape}")
            
            with sdpa_kernel(backends):
                attn_output = F.scaled_dot_product_attention(
                    xq, xk, xv,
                    is_causal=True,
                    scale=scale,
                    dropout_p=0.0
                )
            
            
            # Check for NaN/Inf in attention output
            nan_count = torch.isnan(attn_output).sum().item()
            inf_count = torch.isinf(attn_output).sum().item()
            if self.debug_enabled and (nan_count > 0 or inf_count > 0):
                print(f"\n[ERROR] NaN/Inf detected in attention output with {self.attn_backend}!")
                print(f"  NaN count: {nan_count}/{attn_output.numel()} ({100*nan_count/attn_output.numel():.2f}%)")
                print(f"  Inf count: {inf_count}/{attn_output.numel()} ({100*inf_count/attn_output.numel():.2f}%)")
                
                # Show where NaN/Inf are located
                if nan_count > 0:
                    nan_indices = torch.where(torch.isnan(attn_output.flatten()))[0][:10]
                    print(f"  NaN at indices (first 10): {nan_indices.tolist()}")
                if inf_count > 0:
                    inf_indices = torch.where(torch.isinf(attn_output.flatten()))[0][:10]
                    print(f"  Inf at indices (first 10): {inf_indices.tolist()}")
                
            
            # Convert back from (bs, n_heads, seqlen, head_dim) to (bs, seqlen, n_heads, head_dim)
            attn_output = attn_output.transpose(1, 2).contiguous()
            
            # Convert back to original dtype if needed
            if self.attn_backend == 'native_flash' and orig_dtype not in [torch.float16, torch.bfloat16]:
                attn_output = attn_output.to(orig_dtype)
        
        check_nan(attn_output, "attention output", "BasicAttention")
        
        # Reshape and apply output projection
        attn_output = attn_output.view(bs, seqlen, -1)
        
        output = self.wo(attn_output)
        
        
        check_nan(output, "output (after wo)", "BasicAttention")
        
        return output

    def init_weights(self, init_std: float):
        for linear in (self.wq, self.wk, self.wv):
            nn.init.trunc_normal_(linear.weight, mean=0.0, std=0.02)
        nn.init.trunc_normal_(self.wo.weight, mean=0.0, std=init_std)


class BasicMLP(torch.nn.Module):
    def __init__(
        self,
        config: LlamaConfig,
    ):
        super().__init__()
        self.w1 = nn.Linear(config.hidden_size, config.intermediate_size, bias=False)
        self.w2 = nn.Linear(config.intermediate_size, config.hidden_size, bias=False)
        self.w3 = nn.Linear(config.hidden_size, config.intermediate_size, bias=False)

    def forward(self, x):
        # Check input
        check_nan(x, "input", "BasicMLP")
        
        # Gate path
        w1_out = self.w1(x)
        check_nan(w1_out, "w1 output", "BasicMLP")
        
        # Activation
        silu_out = F.silu(w1_out)
        check_nan(silu_out, "SiLU activation", "BasicMLP")
        
        # Up projection
        w3_out = self.w3(x)
        check_nan(w3_out, "w3 output", "BasicMLP")
        
        # Multiplication
        mult_out = silu_out * w3_out
        check_nan(mult_out, "multiplication (silu * w3)", "BasicMLP")
        
        # Down projection
        output = self.w2(mult_out)
        check_nan(output, "output (after w2)", "BasicMLP")
        
        return output

    def init_weights(self, init_std: float):
        nn.init.trunc_normal_(self.w1.weight, mean=0.0, std=0.02)
        for linear in (self.w2, self.w3):
            nn.init.trunc_normal_(linear.weight, mean=0.0, std=init_std)


class BasicTransformerBlock(torch.nn.Module):
    def __init__(
        self,
        layer_id: int,
        config: LlamaConfig,
        debug_enabled: bool = False,
    ):
        super().__init__()
        self.layer_id = layer_id
        self.attention = BasicAttention(config, layer_id, debug_enabled)
        self.mlp = BasicMLP(config)

        self.attention_norm = nn.RMSNorm(config.hidden_size, eps=config.rms_norm_eps)
        self.mlp_norm = nn.RMSNorm(config.hidden_size, eps=config.rms_norm_eps)

        # TODO:
        self.weight_init_std = 0.02 / (2 * (layer_id + 1)) ** 0.5

    def forward(
        self,
        x: torch.Tensor,
        freqs_cis: torch.Tensor,
    ):
        
        # Check input
        check_nan(x, "input", f"BasicTransformerBlock_{self.layer_id}")
        
        # Attention block
        attn_norm_out = self.attention_norm(x)
        check_nan(attn_norm_out, "attention_norm output", "BasicTransformerBlock")
        
        attn_out = self.attention(attn_norm_out, freqs_cis)
        check_nan(attn_out, "attention output", "BasicTransformerBlock")
        
        h = x + attn_out
        check_nan(h, "residual after attention", "BasicTransformerBlock")
        
        # MLP block
        mlp_norm_out = self.mlp_norm(h)
        check_nan(mlp_norm_out, "mlp_norm output", "BasicTransformerBlock")
        
        mlp_out = self.mlp(mlp_norm_out)
        check_nan(mlp_out, "mlp output", "BasicTransformerBlock")
        
        out = h + mlp_out
        check_nan(out, "final output", "BasicTransformerBlock")
        
        return out

    def init_weights(self):
        for norm in (self.attention_norm, self.mlp_norm):
            norm.reset_parameters()
        self.attention.init_weights(self.weight_init_std)
        self.mlp.init_weights(self.weight_init_std)


@register_model("llama", "torch")
class LlamaBasicModel(nn.Module):
    def __init__(
        self,
        config: LlamaConfig,
        debug_enabled=False,  # Simple debug flag
        attention_backend='flash_attn',  # Attention backend (not used, always flash_attn)
    ):
        super().__init__()
        self.config = config
        self.debug_enabled = debug_enabled
        # Always use flash_attn, ignore the parameter
        self.attention_backend = 'flash_attn'
        
        import os
        gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', ''))
        gpu_info = f"[GPU {gpu_id}] " if gpu_id else ""
        
        if self.debug_enabled:
            print(f"\n{gpu_info}[MODEL INIT] Initializing LlamaBasicModel:")
            print(f"{gpu_info}  - vocab_size: {config.vocab_size}")
            print(f"{gpu_info}  - hidden_size: {config.hidden_size}")
            print(f"{gpu_info}  - num_attention_heads: {config.num_attention_heads}")
            print(f"{gpu_info}  - max_position_embeddings: {config.max_position_embeddings}")

        self.tok_embed = nn.Embedding(config.vocab_size, config.hidden_size)
        if self.debug_enabled:
            print(f"{gpu_info}  - Embedding matrix shape: {self.tok_embed.weight.shape}")

        # TODO: persistent = False
        self.register_buffer(
            "freqs_cis",
            self._precompute_freqs_cis(
                config.hidden_size,
                config.num_attention_heads,
                config.max_position_embeddings,
                config.rope_theta,
            ),
            persistent=True,
        )

        # Only test 4 layers for now
        self.layers = nn.ModuleList([BasicTransformerBlock(layer_id, config, debug_enabled) for layer_id in range(4)])
        self.norm = nn.RMSNorm(config.hidden_size)
        self.output = nn.Linear(config.hidden_size, config.vocab_size, bias=False)
        self.init_weights()
        
        # Register backward hooks for gradient checking (only in debug mode)
        # This avoids the PyTorch warning about backward hooks when inputs don't require gradients
        if self.debug_enabled:
            self.register_backward_hooks()
            print(f"{gpu_info}[DEBUG] Gradient checking hooks registered")

    def _precompute_freqs_cis(self, hidden_size, n_heads, max_seq_len, rope_theta) -> torch.Tensor:
        return precompute_freqs_cis(
            hidden_size // n_heads,
            max_seq_len,
            rope_theta,
        )

    def register_backward_hooks(self):
        """Register backward hooks to check gradients during backpropagation
        
        Note: This can cause PyTorch warnings when inputs don't require gradients.
        Only enable in debug mode when needed.
        """
        
        # Register hook for embedding layer
        self.tok_embed.register_full_backward_hook(
            lambda module, grad_input, grad_output: backward_hook(
                module, grad_input, grad_output, "tok_embed"
            )
        )
        
        # Register hooks for each transformer layer
        for i, layer in enumerate(self.layers):
            # Attention sub-modules
            layer.attention.wq.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.attention.wq"
                )
            )
            layer.attention.wk.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.attention.wk"
                )
            )
            layer.attention.wv.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.attention.wv"
                )
            )
            layer.attention.wo.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.attention.wo"
                )
            )
            
            # MLP sub-modules
            layer.mlp.w1.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.mlp.w1"
                )
            )
            layer.mlp.w2.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.mlp.w2"
                )
            )
            layer.mlp.w3.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.mlp.w3"
                )
            )
            
            # Norm layers
            layer.attention_norm.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.attention_norm"
                )
            )
            layer.mlp_norm.register_full_backward_hook(
                lambda module, grad_input, grad_output, idx=i: backward_hook(
                    module, grad_input, grad_output, f"layer_{idx}.mlp_norm"
                )
            )
        
        # Register hook for final norm
        self.norm.register_full_backward_hook(
            lambda module, grad_input, grad_output: backward_hook(
                module, grad_input, grad_output, "final_norm"
            )
        )
        
        # Register hook for output projection
        self.output.register_full_backward_hook(
            lambda module, grad_input, grad_output: backward_hook(
                module, grad_input, grad_output, "output_projection"
            )
        )
        
    
    def check_all_gradients(self):
        """Check all parameter gradients after backward pass"""
        has_nan = False
        has_large_grad = False
        
        for name, param in self.named_parameters():
            if param.grad is not None:
                # Check for NaN
                if torch.isnan(param.grad).any():
                    nan_count = torch.isnan(param.grad).sum().item()
                    print(f"[ERROR] NaN in gradient for {name}: {nan_count}/{param.grad.numel()} elements")
                    has_nan = True
                
                # Check for large gradients
                grad_max = param.grad.abs().max().item()
                if grad_max > 100:
                    print(f"[WARNING] Large gradient for {name}: max={grad_max:.2e}, mean={param.grad.abs().mean().item():.2e}")
                    has_large_grad = True
                    
                # Check for zero gradients (might indicate dead neurons)
                if param.grad.abs().max().item() < 1e-8:
                    print(f"[INFO] Zero/tiny gradient for {name}: max={grad_max:.2e}")
        
        if has_nan:
            print("[ERROR] Found NaN in gradients! Exiting...")
            sys.exit(1)
        elif has_large_grad:
            print("[WARNING] Found large gradients - potential gradient explosion!")

    def init_weights(self):
        if self.tok_embed is not None:
            import os
            gpu_id = os.environ.get('GPU_RANK', os.environ.get('CUDA_VISIBLE_DEVICES', ''))
            gpu_info = f"[GPU {gpu_id}] " if gpu_id else ""
            print(f"{gpu_info}[INIT] Initializing embeddings with shape: {self.tok_embed.weight.shape}")
            nn.init.normal_(self.tok_embed.weight)
            
            # Check if initialization worked properly
            if torch.isnan(self.tok_embed.weight).any():
                nan_count = torch.isnan(self.tok_embed.weight).sum().item()
                print(f"[ERROR] Found {nan_count} NaN values in embedding after init!")
                # Find which token IDs have NaN
                nan_rows = torch.any(torch.isnan(self.tok_embed.weight), dim=1)
                nan_ids = torch.where(nan_rows)[0]
                print(f"[ERROR] Token IDs with NaN embeddings: {nan_ids[:20].tolist()}")
        for layer in self.layers:
            if layer is not None:
                layer.init_weights()
        if self.norm is not None:
            self.norm.reset_parameters()

        final_out_std = self.config.hidden_size**-0.5
        cutoff_factor = 3
        if self.output is not None:
            nn.init.trunc_normal_(
                self.output.weight,
                mean=0.0,
                std=final_out_std,
                a=-cutoff_factor * final_out_std,
                b=cutoff_factor * final_out_std,
            )

    def check_weight_updates(self):
        """Check if weight updates after optimizer.step() caused NaN"""
        has_nan = False
        
        for name, param in self.named_parameters():
            if torch.isnan(param).any():
                nan_count = torch.isnan(param).sum().item()
                print(f"[ERROR] NaN in parameter {name}: {nan_count}/{param.numel()} elements")
                has_nan = True
                
                # If embedding layer has NaN, identify which token IDs
                if "tok_embed" in name and "weight" in name:
                    nan_rows = torch.any(torch.isnan(param), dim=1)
                    nan_token_ids = torch.where(nan_rows)[0]
                    print(f"[ERROR] Token IDs with NaN embeddings: {nan_token_ids[:50].tolist()}")
                    print(f"[ERROR] Total token IDs with NaN: {nan_rows.sum().item()}")
        
        if has_nan:
            print("[ERROR] Found NaN in weights after update! Exiting...")
            sys.exit(1)

    def forward(self, input_ids):
        
        # Check for invalid token IDs
        invalid_mask = (input_ids < 0) | (input_ids >= self.tok_embed.num_embeddings)
        if invalid_mask.any():
            invalid_count = invalid_mask.sum().item()
            print(f"[WARNING] Found {invalid_count} invalid token IDs (out of embedding range)!")
            invalid_ids = input_ids[invalid_mask]
            print(f"[WARNING] Invalid IDs sample (first 20): {invalid_ids[:20].tolist()}")
            print(f"[WARNING] Invalid IDs unique: {torch.unique(invalid_ids)[:20].tolist()}")
            
            # Show distribution
            high_ids = input_ids[input_ids >= 100000]
            if len(high_ids) > 0:
                print(f"[WARNING] {len(high_ids)} IDs >= 100000, max={high_ids.max().item()}")
        
        # Try to handle out-of-range IDs
        try:
            x = self.tok_embed(input_ids)  # (bs, seqlen, hidden)
        except Exception as e:
            print(f"[ERROR] Embedding lookup failed: {e}")
            # Try to clamp IDs to valid range
            clamped_ids = torch.clamp(input_ids, 0, self.tok_embed.num_embeddings - 1)
            print(f"[INFO] Clamping IDs to range [0, {self.tok_embed.num_embeddings - 1}]")
            x = self.tok_embed(clamped_ids)
        
        check_nan(x, "token embeddings", "LlamaBasicModel")

        # Transformer layers
        for i, layer in enumerate(self.layers):
            x = layer(x, self.freqs_cis)
            check_nan(x, f"layer {i} output", "LlamaBasicModel")

        # Final norm
        x = self.norm(x)
        check_nan(x, "final norm output", "LlamaBasicModel")
        
        # Output projection
        logits = self.output(x)  # (bs, seqlen, vocab)
        check_nan(logits, "logits", "LlamaBasicModel")
        
        return logits
