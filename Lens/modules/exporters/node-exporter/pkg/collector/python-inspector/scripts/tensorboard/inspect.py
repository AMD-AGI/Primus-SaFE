#!/usr/bin/env python3
"""
TensorBoard Inspector Script

This script detects TensorBoard SummaryWriter instances in a running Python process
and extracts configuration information.
"""
import os
import json
import gc
import sys
import traceback


def inspect_tensorboard():
    """Detect TensorBoard SummaryWriter instances"""
    results = {
        "enabled": False,
        "instances": [],
        "scan_info": {
            "total_objects": 0,
            "candidates_found": 0
        }
    }
    
    try:
        objects = gc.get_objects()
        results["scan_info"]["total_objects"] = len(objects)
        
        # Search all objects in memory
        for obj in objects:
            class_name = obj.__class__.__name__
            module_name = obj.__class__.__module__ if hasattr(obj.__class__, '__module__') else ''
            
            # PyTorch TensorBoard (torch.utils.tensorboard.SummaryWriter)
            if class_name == 'SummaryWriter':
                results["scan_info"]["candidates_found"] += 1
                
                # 验证是否真的是TensorBoard的SummaryWriter
                if 'tensorboard' in module_name.lower() or 'torch' in module_name.lower():
                    results["enabled"] = True
                    instance = {
                        "class": class_name,
                        "module": module_name,
                        "log_dir": getattr(obj, 'log_dir', None),
                        "comment": getattr(obj, 'comment', ''),
                        "flush_secs": getattr(obj, 'flush_secs', None),
                    }
                    results["instances"].append(instance)
            
            # TensorFlow TensorBoard
            elif 'FileWriter' in class_name and 'tensorboard' in module_name.lower():
                results["scan_info"]["candidates_found"] += 1
                results["enabled"] = True
                try:
                    log_dir = obj.get_logdir() if hasattr(obj, 'get_logdir') else None
                except:
                    log_dir = None
                
                instance = {
                    "class": class_name,
                    "module": module_name,
                    "log_dir": log_dir,
                }
                results["instances"].append(instance)
            
            # 其他可能的Writer类（用于调试）
            elif 'writer' in class_name.lower() and ('log' in class_name.lower() or 'event' in class_name.lower()):
                results["scan_info"]["candidates_found"] += 1
                # 记录候选对象（但不算作enabled）
                if "other_candidates" not in results:
                    results["other_candidates"] = []
                if len(results["other_candidates"]) < 5:  # 最多记录5个
                    results["other_candidates"].append({
                        "class": class_name,
                        "module": module_name
                    })
                    
    except Exception as e:
        results["error"] = str(e)
        results["traceback"] = traceback.format_exc()
    
    return results


if __name__ == "__main__":
    try:
        # Read output file path from environment variable
        output_file = os.environ.get('INSPECTOR_OUTPUT_FILE', '/tmp/inspection_result.json')
        
        # Debug: write to stderr so we can see it in pyrasite output
        sys.stderr.write(f"[TensorBoard Inspector] Starting inspection\n")
        sys.stderr.write(f"[TensorBoard Inspector] Output file: {output_file}\n")
        sys.stderr.write(f"[TensorBoard Inspector] PID: {os.getpid()}\n")
        sys.stderr.flush()
        
        result = inspect_tensorboard()
        
        sys.stderr.write(f"[TensorBoard Inspector] Inspection completed: enabled={result['enabled']}, instances={len(result['instances'])}\n")
        sys.stderr.flush()
        
        # Ensure directory exists
        output_dir = os.path.dirname(output_file)
        if not os.path.exists(output_dir):
            sys.stderr.write(f"[TensorBoard Inspector] Creating directory: {output_dir}\n")
            os.makedirs(output_dir, exist_ok=True)
        
        # Write result
        with open(output_file, 'w') as f:
            json.dump(result, f, indent=2)
        
        sys.stderr.write(f"[TensorBoard Inspector] Result written to {output_file}\n")
        
        # Verify file was written
        if os.path.exists(output_file):
            file_size = os.path.getsize(output_file)
            sys.stderr.write(f"[TensorBoard Inspector] File verified: {output_file} ({file_size} bytes)\n")
        else:
            sys.stderr.write(f"[TensorBoard Inspector] ERROR: File not found after writing: {output_file}\n")
        
        sys.stderr.flush()
        
    except Exception as e:
        # Write error to stderr
        sys.stderr.write(f"[TensorBoard Inspector] FATAL ERROR: {str(e)}\n")
        sys.stderr.write(f"[TensorBoard Inspector] Traceback:\n{traceback.format_exc()}\n")
        sys.stderr.flush()
        
        # Try to write error result
        try:
            error_result = {
                "enabled": False,
                "error": str(e),
                "traceback": traceback.format_exc()
            }
            with open(output_file, 'w') as f:
                json.dump(error_result, f, indent=2)
        except:
            pass

