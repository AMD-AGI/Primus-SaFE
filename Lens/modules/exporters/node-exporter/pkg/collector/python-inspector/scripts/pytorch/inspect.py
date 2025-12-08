#!/usr/bin/env python3
"""
PyTorch Inspector Script

This script detects PyTorch models and configurations in a running Python process.
"""
import os
import json
import gc
import sys
import traceback


def inspect_pytorch():
    """Detect PyTorch models and configurations"""
    results = {
        "detected": False,
        "models": [],
        "optimizers": []
    }
    
    try:
        import torch
        results["detected"] = True
        results["version"] = torch.__version__
        results["cuda_available"] = torch.cuda.is_available()
        
        if torch.cuda.is_available():
            results["cuda_device_count"] = torch.cuda.device_count()
        
        # Find models
        for obj in gc.get_objects():
            if isinstance(obj, torch.nn.Module):
                try:
                    model_info = {
                        "class_name": obj.__class__.__name__,
                        "total_params": sum(p.numel() for p in obj.parameters()),
                        "trainable_params": sum(p.numel() for p in obj.parameters() if p.requires_grad),
                    }
                    
                    # Get device information
                    try:
                        first_param = next(obj.parameters())
                        model_info["device"] = str(first_param.device)
                        model_info["dtype"] = str(first_param.dtype)
                    except:
                        pass
                    
                    results["models"].append(model_info)
                except:
                    pass
            
            # Find optimizers
            elif hasattr(obj, '__class__') and 'Optimizer' in obj.__class__.__name__:
                try:
                    optimizer_info = {
                        "class_name": obj.__class__.__name__,
                    }
                    
                    # Get learning rate
                    if hasattr(obj, 'param_groups') and obj.param_groups:
                        optimizer_info["lr"] = obj.param_groups[0].get('lr', None)
                    
                    results["optimizers"].append(optimizer_info)
                except:
                    pass
    
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
        
        sys.stderr.write(f"[PyTorch Inspector] Starting inspection\n")
        sys.stderr.write(f"[PyTorch Inspector] Output file: {output_file}\n")
        sys.stderr.flush()
        
        result = inspect_pytorch()
        
        sys.stderr.write(f"[PyTorch Inspector] Inspection completed: detected={result['detected']}\n")
        sys.stderr.flush()
        
        # Ensure directory exists
        output_dir = os.path.dirname(output_file)
        if not os.path.exists(output_dir):
            os.makedirs(output_dir, exist_ok=True)
        
        # Write result
        with open(output_file, 'w') as f:
            json.dump(result, f, indent=2)
        
        sys.stderr.write(f"[PyTorch Inspector] Result written to {output_file}\n")
        sys.stderr.flush()
        
    except Exception as e:
        sys.stderr.write(f"[PyTorch Inspector] FATAL ERROR: {str(e)}\n")
        sys.stderr.write(f"[PyTorch Inspector] Traceback:\n{traceback.format_exc()}\n")
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

