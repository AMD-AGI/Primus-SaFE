# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
WandB Hook Module - Automatically intercept and enhance wandb functionality
No user code changes required, automatically activated after pip install
"""
import os
import sys
import atexit
import time
from typing import Any, Dict, Optional, List

# Import logger module
from .logger import debug_log, error_log, warning_log

# Import API reporter and data collector modules
try:
    from .api_reporter import get_global_reporter, shutdown_reporter
    from .data_collector import DataCollector
    _api_enabled = True
except ImportError:
    _api_enabled = False
    warning_log("[Primus Lens WandB] API reporter not available, using file-based reporting only")


class WandbInterceptor:
    """WandB Interceptor - Intercept key methods of wandb"""
    
    def __init__(self):
        self.original_init = None
        self.original_log = None
        self.original_finish = None
        self.is_patched = False
        self.metrics_buffer = []
        
        # API reporting related
        self.api_reporter = None
        self.data_collector = None
        self.wandb_run = None  # Save run object
        self.run_id = None
        
        # Whether API reporting is enabled
        self.api_reporting_enabled = (
            _api_enabled and 
            os.environ.get("PRIMUS_LENS_WANDB_API_REPORTING", "true").lower() == "true"
        )
        
        if self.api_reporting_enabled:
            self.api_reporter = get_global_reporter()
            self.data_collector = DataCollector()
            debug_log("[Primus Lens WandB] API reporting enabled")
        
    def _get_rank_info(self) -> Dict[str, int]:
        """Get rank information for distributed training"""
        if "SLURM_NODEID" in os.environ:
            node_rank = int(os.environ["SLURM_NODEID"])
        elif "PET_NODE_RANK" in os.environ:
            node_rank = int(os.environ["PET_NODE_RANK"])
        elif "NODE_RANK" in os.environ:
            node_rank = int(os.environ["NODE_RANK"])
        else:
            node_rank = 0
        
        return {
            "RANK": int(os.environ.get("RANK", -1)),
            "LOCAL_RANK": int(os.environ.get("LOCAL_RANK", -1)),
            "NODE_RANK": node_rank,
            "WORLD_SIZE": int(os.environ.get("WORLD_SIZE", -1)),
        }
    
    def _setup_metrics_output(self):
        """Setup metrics output path"""
        output_path = os.environ.get("PRIMUS_LENS_WANDB_OUTPUT_PATH", None)
        if output_path:
            rank_info = self._get_rank_info()
            node_rank = rank_info["NODE_RANK"]
            local_rank = rank_info["LOCAL_RANK"]
            
            # Create output directory
            full_path = os.path.join(output_path, f"node_{node_rank}", f"rank_{local_rank}")
            os.makedirs(full_path, exist_ok=True)
            
            return full_path
        return None
    
    def _save_metrics(self, data: Dict[str, Any], step: Optional[int] = None):
        """Save metrics to local file"""
        debug_log(f"[Primus Lens WandB] DEBUG: _save_metrics called with step={step}")
        output_path = self._setup_metrics_output()
        debug_log(f"[Primus Lens WandB] DEBUG: output_path={output_path}")
        if not output_path:
            debug_log(f"[Primus Lens WandB] DEBUG: output_path is None, returning")
            return
        
        try:
            import json
            import time
            
            metric_entry = {
                "timestamp": time.time(),
                "step": step,
                "data": data,
            }
            
            # Append to jsonl file
            metrics_file = os.path.join(output_path, "wandb_metrics.jsonl")
            debug_log(f"[Primus Lens WandB] DEBUG: Writing to {metrics_file}")
            with open(metrics_file, "a") as f:
                f.write(json.dumps(metric_entry) + "\n")
            debug_log(f"[Primus Lens WandB] DEBUG: Successfully wrote metrics")
                
        except Exception as e:
            error_log(f"[Primus Lens WandB] Failed to save metrics: {e}")
            import traceback
            traceback.print_exc()
    
    def _report_framework_detection(self, wandb_run):
        """Asynchronously report framework detection data (non-blocking)"""
        debug_log(f"[Primus Lens WandB] _report_framework_detection() called")
        debug_log(f"[Primus Lens WandB] WandB run name: {wandb_run.name if wandb_run else 'None'}")
        
        try:
            # Collect detection data
            debug_log(f"[Primus Lens WandB] Collecting detection data...")
            detection_data = self.data_collector.collect_detection_data(wandb_run)
            debug_log(f"[Primus Lens WandB] Detection data collected, keys: {list(detection_data.keys())}")
            
            # Validate required fields
            if not detection_data.get("workload_uid"):
                warning_log("[Primus Lens WandB] Warning: WORKLOAD_UID not set, detection may not be associated with workload")
            
            # Asynchronous reporting
            debug_log(f"[Primus Lens WandB] Calling api_reporter.report_detection()...")
            self.api_reporter.report_detection(detection_data)
            debug_log(f"[Primus Lens WandB] api_reporter.report_detection() completed")
            
            debug_log(f"[Primus Lens WandB] Framework detection data queued for reporting")
            if detection_data.get("hints", {}).get("possible_frameworks"):
                debug_log(f"  Detected frameworks: {detection_data['hints']['possible_frameworks']}")
                debug_log(f"  Confidence: {detection_data['hints']['confidence']}")
        
        except Exception as e:
            error_log(f"[Primus Lens WandB] Failed to report framework detection: {e}")
            import traceback
            traceback.print_exc()
    
    def _report_metrics(self, data: Dict[str, Any], step: Optional[int] = None):
        """Asynchronously report metrics data (non-blocking)"""
        debug_log(f"[Primus Lens WandB] _report_metrics() called, step={step}")
        debug_log(f"[Primus Lens WandB] Input data keys: {list(data.keys())}")
        
        try:
            # Construct metrics data
            current_time = time.time()
            
            # Convert data to metrics list
            metrics = []
            for key, value in data.items():
                # Skip non-numeric types
                if not isinstance(value, (int, float)):
                    debug_log(f"[Primus Lens WandB] Skipping non-numeric metric: {key} (type: {type(value).__name__})")
                    continue
                
                metric = {
                    "name": key,
                    "value": float(value),
                    "step": step if step is not None else 0,
                    "timestamp": current_time,
                    "tags": {},
                }
                metrics.append(metric)
            
            if not metrics:
                debug_log(f"[Primus Lens WandB] No numeric metrics to report")
                return
            
            debug_log(f"[Primus Lens WandB] Prepared {len(metrics)} metric(s) for reporting")
            
            # Construct request data
            metrics_data = {
                "source": "wandb",
                "workload_uid": os.environ.get("WORKLOAD_UID", ""),
                "pod_uid": os.environ.get("POD_UID", ""),  # Fix: use correct environment variable POD_UID
                "pod_name": os.environ.get("POD_NAME", ""),  # Add: missing pod_name field
                "run_id": self.run_id or "",
                "metrics": metrics,
                "timestamp": current_time,
            }
            
            debug_log(f"[Primus Lens WandB] Metrics data prepared, workload_uid={metrics_data['workload_uid']}, pod_name={metrics_data['pod_name']}, run_id={metrics_data['run_id']}")
            
            # Asynchronous reporting
            debug_log(f"[Primus Lens WandB] Calling api_reporter.report_metrics()...")
            self.api_reporter.report_metrics(metrics_data)
            debug_log(f"[Primus Lens WandB] api_reporter.report_metrics() completed")
        
        except Exception as e:
            # Print error to debug log
            debug_log(f"[Primus Lens WandB] Error in _report_metrics: {e}")
            import traceback
            debug_log(f"[Primus Lens WandB] Traceback: {traceback.format_exc()}")
    
    def patch_wandb(self):
        """Patch key methods of wandb"""
        if self.is_patched:
            return
        
        try:
            import wandb
        except ImportError:
            # wandb not installed, no need to patch
            return
        
        # Save original methods
        self.original_init = wandb.init
        self.original_log = wandb.log
        
        # Create interception methods
        def intercepted_init(*args, **kwargs):
            """Intercept wandb.init"""
            debug_log("[Primus Lens WandB] ============================================")
            debug_log("[Primus Lens WandB] intercepted_init() called")
            debug_log(f"[Primus Lens WandB] Args: {args}")
            debug_log(f"[Primus Lens WandB] Kwargs keys: {list(kwargs.keys())}")
            
            rank_info = self._get_rank_info()
            debug_log(f"[Primus Lens WandB] Rank info: {rank_info}")
            
            # Setup output path
            output_path = self._setup_metrics_output()
            if output_path:
                debug_log(f"[Primus Lens WandB] Metrics will be saved to: {output_path}")
            else:
                debug_log(f"[Primus Lens WandB] No output path configured (PRIMUS_LENS_WANDB_OUTPUT_PATH not set)")
            
            # Call original init
            debug_log(f"[Primus Lens WandB] Calling original wandb.init()...")
            result = self.original_init(*args, **kwargs)
            debug_log(f"[Primus Lens WandB] Original wandb.init() completed")
            
            # Save run object
            self.wandb_run = result
            if result:
                self.run_id = result.id if hasattr(result, 'id') else None
                debug_log(f"[Primus Lens WandB] WandB run initialized successfully")
                debug_log(f"[Primus Lens WandB]   Name: {result.name}")
                debug_log(f"[Primus Lens WandB]   Project: {result.project}")
                debug_log(f"[Primus Lens WandB]   Run ID: {self.run_id}")
                debug_log(f"[Primus Lens WandB]   API reporting enabled: {self.api_reporting_enabled}")
                
                # Asynchronously report framework detection data
                if self.api_reporting_enabled:
                    debug_log(f"[Primus Lens WandB] Starting framework detection reporting...")
                    self._report_framework_detection(result)
                    debug_log(f"[Primus Lens WandB] Framework detection reporting completed")
            else:
                warning_log("[Primus Lens WandB] wandb.init() returned None")
            
            # Important: wandb.init() overrides wandb.log, need to re-intercept
            # Also update original_log to the run object's log method
            import wandb
            if result and hasattr(result, 'log'):
                self.original_log = result.log  # Use run object's log method
                debug_log(f"[Primus Lens WandB] Using run.log method")
            else:
                self.original_log = wandb.log  # Fallback to module-level log
                debug_log(f"[Primus Lens WandB] Using wandb.log method")
            wandb.log = intercepted_log
            debug_log(f"[Primus Lens WandB] wandb.log re-patched")
            debug_log("[Primus Lens WandB] ============================================")
            
            return result
        
        def intercepted_log(data: Dict[str, Any], step: Optional[int] = None, *args, **kwargs):
            """Intercept wandb.log"""
            debug_log(f"[Primus Lens WandB] --------------------------------------------")
            debug_log(f"[Primus Lens WandB] intercepted_log() called")
            debug_log(f"[Primus Lens WandB] Step: {step}")
            debug_log(f"[Primus Lens WandB] Data keys: {list(data.keys())}")
            debug_log(f"[Primus Lens WandB] Data values sample: {dict(list(data.items())[:5])}")
            
            # Copy data to avoid modifying original data
            enhanced_data = data.copy()
            
            # Add Primus Lens marker
            enhanced_data["_primus_lens_enabled"] = True
            
            # Add additional system metrics based on configuration
            enhance_metrics = os.environ.get("PRIMUS_LENS_WANDB_ENHANCE_METRICS", "false").lower() == "true"
            debug_log(f"[Primus Lens WandB] Enhance metrics: {enhance_metrics}")
            
            if enhance_metrics:
                try:
                    import psutil
                    enhanced_data["_primus_sys_cpu_percent"] = psutil.cpu_percent()
                    enhanced_data["_primus_sys_memory_percent"] = psutil.virtual_memory().percent
                    debug_log(f"[Primus Lens WandB] Added system metrics (CPU, Memory)")
                    
                    # GPU metrics (if available)
                    try:
                        import pynvml
                        pynvml.nvmlInit()
                        device_count = pynvml.nvmlDeviceGetCount()
                        for i in range(device_count):
                            handle = pynvml.nvmlDeviceGetHandleByIndex(i)
                            util = pynvml.nvmlDeviceGetUtilizationRates(handle)
                            mem = pynvml.nvmlDeviceGetMemoryInfo(handle)
                            enhanced_data[f"_primus_gpu_{i}_util"] = util.gpu
                            enhanced_data[f"_primus_gpu_{i}_mem_used_mb"] = mem.used / 1024 / 1024
                        debug_log(f"[Primus Lens WandB] Added GPU metrics for {device_count} device(s)")
                    except:
                        pass  # GPU metrics collection failed, ignore
                        
                except ImportError:
                    debug_log(f"[Primus Lens WandB] psutil not available, skipping system metrics")
            
            # Save metrics to local file
            save_local = os.environ.get("PRIMUS_LENS_WANDB_SAVE_LOCAL", "true").lower() == "true"
            debug_log(f"[Primus Lens WandB] Save local: {save_local}")
            
            if save_local:
                debug_log(f"[Primus Lens WandB] Saving metrics to local file...")
                self._save_metrics(enhanced_data, step)
                debug_log(f"[Primus Lens WandB] Local save completed")
            
            # Asynchronously report metrics to API
            debug_log(f"[Primus Lens WandB] API reporting enabled: {self.api_reporting_enabled}")
            
            if self.api_reporting_enabled:
                debug_log(f"[Primus Lens WandB] Starting metrics API reporting...")
                self._report_metrics(data, step)
                debug_log(f"[Primus Lens WandB] Metrics API reporting completed")
            
            # Call original log method
            debug_log(f"[Primus Lens WandB] Calling original wandb.log()...")
            result = self.original_log(enhanced_data, step=step, *args, **kwargs)
            debug_log(f"[Primus Lens WandB] Original wandb.log() completed")
            debug_log(f"[Primus Lens WandB] --------------------------------------------")
            
            return result
        
        # Apply patch
        wandb.init = intercepted_init
        wandb.log = intercepted_log
        
        # Add marker for verification
        wandb._primus_lens_patched = True
        
        self.is_patched = True
        debug_log("[Primus Lens WandB] WandB successfully patched!")
    
    def install(self):
        """Install interceptor"""
        # Check if enabled
        if os.environ.get("PRIMUS_LENS_WANDB_HOOK", "true").lower() != "true":
            return
        
        debug_log("[Primus Lens WandB] Installing WandB hook...")
        
        # Patch wandb
        self.patch_wandb()


# Global interceptor instance
_global_interceptor = None


def install_wandb_hook():
    """Install WandB Hook - automatically called when package is imported"""
    global _global_interceptor
    
    if _global_interceptor is None:
        _global_interceptor = WandbInterceptor()
        _global_interceptor.install()
        
        # Register cleanup function on exit
        def cleanup_on_exit():
            """Clean up resources on program exit"""
            debug_log("[Primus Lens WandB] Cleaning up...")
            if _api_enabled:
                shutdown_reporter()
        
        atexit.register(cleanup_on_exit)


# Automatically install when module is imported (if triggered via .pth file)
if __name__ != "__main__":
    # Use import hook mechanism, delay patching until wandb is imported
    import sys
    from importlib.abc import MetaPathFinder, Loader
    from importlib.machinery import ModuleSpec
    
    class WandbImportHook(MetaPathFinder):
        """WandB import hook - automatically patch when wandb is imported"""
        
        def find_spec(self, fullname, path, target=None):
            if fullname == "wandb":
                # Find wandb's spec
                for finder in sys.meta_path:
                    if finder is self:
                        continue
                    spec = finder.find_spec(fullname, path, target)
                    if spec:
                        # After finding, patch immediately after import
                        original_loader = spec.loader
                        
                        class PatchingLoader(Loader):
                            def exec_module(self, module):
                                # Execute original loading
                                original_loader.exec_module(module)
                                # Patch immediately
                                install_wandb_hook()
                        
                        spec.loader = PatchingLoader()
                        # Remove itself to avoid repeated triggers
                        try:
                            sys.meta_path.remove(self)
                        except ValueError:
                            pass
                        return spec
            return None
    
    # Register import hook
    sys.meta_path.insert(0, WandbImportHook())

