#!/usr/bin/env python3
"""
PyTorch Inspector Script

This script detects PyTorch models and configurations in a running Python process.
"""
import sys
import json
import gc


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
    
    return results


if __name__ == "__main__":
    output_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/inspection_result.json"
    
    result = inspect_pytorch()
    
    with open(output_file, 'w') as f:
        json.dump(result, f, indent=2)

