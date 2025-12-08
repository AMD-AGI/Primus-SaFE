#!/usr/bin/env python3
"""
Megatron Inspector Script

This script detects Megatron-LM configurations in a running Python process.
"""
import os
import json
import sys
import traceback


def inspect_megatron():
    """Detect Megatron configurations"""
    results = {
        "detected": False
    }
    
    try:
        from megatron import get_args
        args = get_args()
        results["detected"] = True
        results["args"] = {
            "tensor_model_parallel_size": getattr(args, 'tensor_model_parallel_size', None),
            "pipeline_model_parallel_size": getattr(args, 'pipeline_model_parallel_size', None),
            "seq_length": getattr(args, 'seq_length', None),
            "hidden_size": getattr(args, 'hidden_size', None),
            "num_layers": getattr(args, 'num_layers', None),
            "num_attention_heads": getattr(args, 'num_attention_heads', None),
            "global_batch_size": getattr(args, 'global_batch_size', None),
            "micro_batch_size": getattr(args, 'micro_batch_size', None),
            "lr": getattr(args, 'lr', None),
            "min_lr": getattr(args, 'min_lr', None),
            "lr_decay_style": getattr(args, 'lr_decay_style', None),
            "lr_warmup_fraction": getattr(args, 'lr_warmup_fraction', None),
            "train_iters": getattr(args, 'train_iters', None),
        }
    except ImportError:
        results["detected"] = False
    except Exception as e:
        results["error"] = str(e)
        results["traceback"] = traceback.format_exc()
    
    return results


if __name__ == "__main__":
    try:
        # Read output file path from environment variable
        output_file = os.environ.get('INSPECTOR_OUTPUT_FILE', '/tmp/inspection_result.json')
        
        sys.stderr.write(f"[Megatron Inspector] Starting inspection\n")
        sys.stderr.write(f"[Megatron Inspector] Output file: {output_file}\n")
        sys.stderr.flush()
        
        result = inspect_megatron()
        
        sys.stderr.write(f"[Megatron Inspector] Inspection completed: detected={result['detected']}\n")
        sys.stderr.flush()
        
        # Ensure directory exists
        output_dir = os.path.dirname(output_file)
        if not os.path.exists(output_dir):
            os.makedirs(output_dir, exist_ok=True)
        
        # Write result
        with open(output_file, 'w') as f:
            json.dump(result, f, indent=2)
        
        sys.stderr.write(f"[Megatron Inspector] Result written to {output_file}\n")
        sys.stderr.flush()
        
    except Exception as e:
        sys.stderr.write(f"[Megatron Inspector] FATAL ERROR: {str(e)}\n")
        sys.stderr.write(f"[Megatron Inspector] Traceback:\n{traceback.format_exc()}\n")
        sys.stderr.flush()
        
        try:
            error_result = {
                "detected": False,
                "error": str(e),
                "traceback": traceback.format_exc()
            }
            with open(output_file, 'w') as f:
                json.dump(error_result, f, indent=2)
        except:
            pass

