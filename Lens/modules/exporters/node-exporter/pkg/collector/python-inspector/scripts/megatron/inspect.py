#!/usr/bin/env python3
"""
Megatron Inspector Script

This script detects Megatron-LM configurations in a running Python process.
"""
import sys
import json


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
    
    return results


if __name__ == "__main__":
    output_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/inspection_result.json"
    
    result = inspect_megatron()
    
    with open(output_file, 'w') as f:
        json.dump(result, f, indent=2)

