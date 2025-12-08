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

