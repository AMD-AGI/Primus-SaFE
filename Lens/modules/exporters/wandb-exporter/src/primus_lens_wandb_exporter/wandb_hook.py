"""
WandB Hook Module - 自动劫持和增强 wandb 功能
无需用户修改任何代码，通过 pip install 即可自动生效
"""
import os
import sys
import atexit
import time
from typing import Any, Dict, Optional, List

# 导入日志模块
from .logger import debug_log, error_log, warning_log

# 导入 API 上报和数据采集模块
try:
    from .api_reporter import get_global_reporter, shutdown_reporter
    from .data_collector import DataCollector
    _api_enabled = True
except ImportError:
    _api_enabled = False
    warning_log("[Primus Lens WandB] API reporter not available, using file-based reporting only")


class WandbInterceptor:
    """WandB 拦截器 - 劫持 wandb 的关键方法"""
    
    def __init__(self):
        self.original_init = None
        self.original_log = None
        self.original_finish = None
        self.is_patched = False
        self.metrics_buffer = []
        
        # API 上报相关
        self.api_reporter = None
        self.data_collector = None
        self.wandb_run = None  # 保存 run 对象
        self.run_id = None
        
        # 是否启用 API 上报
        self.api_reporting_enabled = (
            _api_enabled and 
            os.environ.get("PRIMUS_LENS_WANDB_API_REPORTING", "true").lower() == "true"
        )
        
        if self.api_reporting_enabled:
            self.api_reporter = get_global_reporter()
            self.data_collector = DataCollector()
            debug_log("[Primus Lens WandB] API reporting enabled")
        
    def _get_rank_info(self) -> Dict[str, int]:
        """获取分布式训练的 rank 信息"""
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
        """设置指标输出路径"""
        output_path = os.environ.get("PRIMUS_LENS_WANDB_OUTPUT_PATH", None)
        if output_path:
            rank_info = self._get_rank_info()
            node_rank = rank_info["NODE_RANK"]
            local_rank = rank_info["LOCAL_RANK"]
            
            # 创建输出目录
            full_path = os.path.join(output_path, f"node_{node_rank}", f"rank_{local_rank}")
            os.makedirs(full_path, exist_ok=True)
            
            return full_path
        return None
    
    def _save_metrics(self, data: Dict[str, Any], step: Optional[int] = None):
        """保存指标到本地文件"""
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
            
            # 追加到 jsonl 文件
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
        """异步上报框架检测数据（不阻塞）"""
        try:
            # 采集检测数据
            detection_data = self.data_collector.collect_detection_data(wandb_run)
            
            # 验证必需字段
            if not detection_data.get("workload_uid"):
                warning_log("[Primus Lens WandB] Warning: WORKLOAD_UID not set, detection may not be associated with workload")
            
            # 异步上报
            self.api_reporter.report_detection(detection_data)
            
            debug_log(f"[Primus Lens WandB] Framework detection data queued for reporting")
            if detection_data.get("hints", {}).get("possible_frameworks"):
                debug_log(f"  Detected frameworks: {detection_data['hints']['possible_frameworks']}")
                debug_log(f"  Confidence: {detection_data['hints']['confidence']}")
        
        except Exception as e:
            error_log(f"[Primus Lens WandB] Failed to report framework detection: {e}")
            import traceback
            traceback.print_exc()
    
    def _report_metrics(self, data: Dict[str, Any], step: Optional[int] = None):
        """异步上报指标数据（不阻塞）"""
        try:
            # 构造指标数据
            current_time = time.time()
            
            # 将 data 转换为 metrics 列表
            metrics = []
            for key, value in data.items():
                # 跳过非数值类型
                if not isinstance(value, (int, float)):
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
                return
            
            # 构造请求数据
            metrics_data = {
                "source": "wandb",
                "workload_uid": os.environ.get("WORKLOAD_UID", ""),
                "pod_uid": os.environ.get("POD_NAME", ""),
                "run_id": self.run_id or "",
                "metrics": metrics,
                "timestamp": current_time,
            }
            
            # 异步上报
            self.api_reporter.report_metrics(metrics_data)
        
        except Exception as e:
            # 不打印错误，避免日志过多
            pass
    
    def patch_wandb(self):
        """Patch wandb 的关键方法"""
        if self.is_patched:
            return
        
        try:
            import wandb
        except ImportError:
            # wandb 未安装，无需 patch
            return
        
        # 保存原始方法
        self.original_init = wandb.init
        self.original_log = wandb.log
        
        # 创建拦截方法
        def intercepted_init(*args, **kwargs):
            """拦截 wandb.init"""
            debug_log("[Primus Lens WandB] Intercepted wandb.init()")
            
            rank_info = self._get_rank_info()
            debug_log(f"[Primus Lens WandB] Rank info: {rank_info}")
            
            # 设置输出路径
            output_path = self._setup_metrics_output()
            if output_path:
                debug_log(f"[Primus Lens WandB] Metrics will be saved to: {output_path}")
            
            # 调用原始 init
            result = self.original_init(*args, **kwargs)
            
            # 保存 run 对象
            self.wandb_run = result
            if result:
                self.run_id = result.id if hasattr(result, 'id') else None
                debug_log(f"[Primus Lens WandB] WandB run initialized: {result.name}")
                debug_log(f"[Primus Lens WandB] Project: {result.project}")
                debug_log(f"[Primus Lens WandB] Run ID: {self.run_id}")
                
                # 异步上报框架检测数据
                if self.api_reporting_enabled:
                    self._report_framework_detection(result)
            
            # 重要：wandb.init() 会覆盖 wandb.log，需要重新劫持
            # 同时更新 original_log 为 run 对象的 log 方法
            import wandb
            if result and hasattr(result, 'log'):
                self.original_log = result.log  # 使用 run 对象的 log 方法
            else:
                self.original_log = wandb.log  # 回退到模块级别的 log
            wandb.log = intercepted_log
            
            return result
        
        def intercepted_log(data: Dict[str, Any], step: Optional[int] = None, *args, **kwargs):
            """拦截 wandb.log"""
            debug_log(f"[Primus Lens WandB] DEBUG: intercepted_log called, step={step}, data keys={list(data.keys())}")
            # 复制数据，避免修改原始数据
            enhanced_data = data.copy()
            
            # 添加 Primus Lens 标记
            enhanced_data["_primus_lens_enabled"] = True
            
            # 根据配置添加额外的系统指标
            if os.environ.get("PRIMUS_LENS_WANDB_ENHANCE_METRICS", "false").lower() == "true":
                try:
                    import psutil
                    enhanced_data["_primus_sys_cpu_percent"] = psutil.cpu_percent()
                    enhanced_data["_primus_sys_memory_percent"] = psutil.virtual_memory().percent
                    
                    # GPU 指标（如果可用）
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
                    except:
                        pass  # GPU 指标收集失败，忽略
                        
                except ImportError:
                    pass  # psutil 未安装，跳过系统指标
            
            # 保存指标到本地
            if os.environ.get("PRIMUS_LENS_WANDB_SAVE_LOCAL", "true").lower() == "true":
                self._save_metrics(enhanced_data, step)
            
            # 异步上报指标到 API
            if self.api_reporting_enabled:
                self._report_metrics(data, step)
            
            # 调用原始 log 方法
            return self.original_log(enhanced_data, step=step, *args, **kwargs)
        
        # 应用 patch
        wandb.init = intercepted_init
        wandb.log = intercepted_log
        
        # 添加标记，便于验证
        wandb._primus_lens_patched = True
        
        self.is_patched = True
        debug_log("[Primus Lens WandB] WandB successfully patched!")
    
    def install(self):
        """安装劫持器"""
        # 检查是否启用
        if os.environ.get("PRIMUS_LENS_WANDB_HOOK", "true").lower() != "true":
            return
        
        debug_log("[Primus Lens WandB] Installing WandB hook...")
        
        # Patch wandb
        self.patch_wandb()


# 全局拦截器实例
_global_interceptor = None


def install_wandb_hook():
    """安装 WandB Hook - 会在包导入时自动调用"""
    global _global_interceptor
    
    if _global_interceptor is None:
        _global_interceptor = WandbInterceptor()
        _global_interceptor.install()
        
        # 注册退出时的清理函数
        def cleanup_on_exit():
            """程序退出时清理资源"""
            debug_log("[Primus Lens WandB] Cleaning up...")
            if _api_enabled:
                shutdown_reporter()
        
        atexit.register(cleanup_on_exit)


# 在模块导入时自动安装（如果通过 .pth 文件触发）
if __name__ != "__main__":
    # 使用 import hook 机制，延迟到 wandb 导入时才 patch
    import sys
    from importlib.abc import MetaPathFinder, Loader
    from importlib.machinery import ModuleSpec
    
    class WandbImportHook(MetaPathFinder):
        """WandB 导入钩子 - 在 wandb 导入时自动 patch"""
        
        def find_spec(self, fullname, path, target=None):
            if fullname == "wandb":
                # 找到 wandb 的 spec
                for finder in sys.meta_path:
                    if finder is self:
                        continue
                    spec = finder.find_spec(fullname, path, target)
                    if spec:
                        # 找到后，在导入后立即 patch
                        original_loader = spec.loader
                        
                        class PatchingLoader(Loader):
                            def exec_module(self, module):
                                # 执行原始加载
                                original_loader.exec_module(module)
                                # 立即 patch
                                install_wandb_hook()
                        
                        spec.loader = PatchingLoader()
                        # 移除自己，避免重复触发
                        try:
                            sys.meta_path.remove(self)
                        except ValueError:
                            pass
                        return spec
            return None
    
    # 注册 import hook
    sys.meta_path.insert(0, WandbImportHook())

