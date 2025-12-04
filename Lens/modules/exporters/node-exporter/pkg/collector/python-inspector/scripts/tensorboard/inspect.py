#!/usr/bin/env python3
"""
TensorBoard Inspector Script

This script detects TensorBoard SummaryWriter instances in a running Python process
and extracts configuration information.
"""
import sys
import json
import gc


def inspect_tensorboard():
    """Detect TensorBoard SummaryWriter instances"""
    results = {
        "enabled": False,
        "instances": []
    }
    
    try:
        # Search all objects in memory
        for obj in gc.get_objects():
            # PyTorch TensorBoard
            if obj.__class__.__name__ == 'SummaryWriter':
                results["enabled"] = True
                instance = {
                    "log_dir": getattr(obj, 'log_dir', None),
                    "comment": getattr(obj, 'comment', ''),
                    "flush_secs": getattr(obj, 'flush_secs', 120),
                }
                results["instances"].append(instance)
            
            # TensorFlow TensorBoard
            elif 'FileWriter' in obj.__class__.__name__:
                results["enabled"] = True
                try:
                    log_dir = obj.get_logdir() if hasattr(obj, 'get_logdir') else None
                except:
                    log_dir = None
                
                instance = {
                    "log_dir": log_dir,
                }
                results["instances"].append(instance)
    except Exception as e:
        results["error"] = str(e)
    
    return results


if __name__ == "__main__":
    output_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/inspection_result.json"
    
    result = inspect_tensorboard()
    
    with open(output_file, 'w') as f:
        json.dump(result, f, indent=2)

