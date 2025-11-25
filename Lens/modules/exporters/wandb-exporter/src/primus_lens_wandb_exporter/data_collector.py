"""
Data Collector - 采集框架检测需要的原始数据
"""
import os
import sys
import time
from typing import Dict, List, Any, Optional


class DataCollector:
    """数据采集器 - 采集原始证据数据"""
    
    def __init__(self):
        self.collector_version = "1.0.0"
        self.data_schema_version = "1.0"
    
    def collect_detection_data(self, wandb_run) -> Dict[str, Any]:
        """
        采集框架检测数据
        
        Args:
            wandb_run: WandB run 对象
        
        Returns:
            Dict: 完整的检测数据
        """
        # 采集原始证据
        evidence = self._collect_raw_evidence(wandb_run)
        
        # 生成 hints
        hints = self._get_framework_hints(evidence)
        
        # 构造完整的上报数据
        detection_data = {
            "source": "wandb",
            "type": "framework_detection_raw",
            "version": "1.0",
            "workload_uid": os.environ.get("WORKLOAD_UID", ""),
            "pod_uid": os.environ.get("POD_UID", ""),
            "pod_name": os.environ.get("POD_NAME", ""),
            "namespace": os.environ.get("POD_NAMESPACE", ""),
            "evidence": evidence,
            "hints": hints,
            "timestamp": time.time(),
        }
        
        return detection_data
    
    def _collect_raw_evidence(self, wandb_run) -> Dict[str, Any]:
        """
        采集原始证据数据
        
        Args:
            wandb_run: WandB run 对象
        
        Returns:
            Dict: 原始证据数据
        """
        evidence = {
            # 1. WandB 相关信息
            "wandb": self._extract_wandb_info(wandb_run),
            
            # 2. 环境变量（框架识别相关）
            "environment": self._extract_environment_vars(),
            
            # 3. PyTorch 信息（如果可用）
            "pytorch": self._extract_pytorch_info(),
            
            # 4. 系统信息
            "system": {
                "python_version": sys.version,
                "python_executable": sys.executable,
                "platform": sys.platform,
            },
        }
        
        return evidence
    
    def _extract_wandb_info(self, wandb_run) -> Dict[str, Any]:
        """提取 WandB 信息"""
        if wandb_run is None:
            return {}
        
        try:
            config = self._safe_get_config(wandb_run)
            tags = wandb_run.tags if hasattr(wandb_run, 'tags') else []
            
            return {
                "project": wandb_run.project if hasattr(wandb_run, 'project') else None,
                "name": wandb_run.name if hasattr(wandb_run, 'name') else None,
                "id": wandb_run.id if hasattr(wandb_run, 'id') else None,
                "config": config,
                "tags": tags,
            }
        except Exception as e:
            print(f"[Primus Lens Data Collector] Failed to extract wandb info: {e}")
            return {}
    
    def _safe_get_config(self, wandb_run) -> Dict[str, Any]:
        """安全获取 wandb config"""
        try:
            if wandb_run and hasattr(wandb_run, 'config'):
                # 尝试转换为 dict
                if hasattr(wandb_run.config, '_as_dict'):
                    return wandb_run.config._as_dict()
                elif hasattr(wandb_run.config, 'as_dict'):
                    return wandb_run.config.as_dict()
                elif isinstance(wandb_run.config, dict):
                    return wandb_run.config
                else:
                    # 尝试直接访问属性
                    return {k: v for k, v in wandb_run.config.__dict__.items() 
                            if not k.startswith('_')}
        except Exception as e:
            print(f"[Primus Lens Data Collector] Failed to get config: {e}")
        
        return {}
    
    def _extract_environment_vars(self) -> Dict[str, str]:
        """提取框架相关的环境变量"""
        env_vars = {
            # 通用框架标识
            "FRAMEWORK": os.environ.get("FRAMEWORK"),
            "TRAINING_FRAMEWORK": os.environ.get("TRAINING_FRAMEWORK"),
            
            # Primus 特定
            "PRIMUS_CONFIG": os.environ.get("PRIMUS_CONFIG"),
            "PRIMUS_VERSION": os.environ.get("PRIMUS_VERSION"),
            "PRIMUS_BACKEND": os.environ.get("PRIMUS_BACKEND"),
            
            # DeepSpeed 特定
            "DEEPSPEED_CONFIG": os.environ.get("DEEPSPEED_CONFIG"),
            "DEEPSPEED_VERSION": os.environ.get("DEEPSPEED_VERSION"),
            "DS_CONFIG": os.environ.get("DS_CONFIG"),
            
            # Megatron 特定
            "MEGATRON_CONFIG": os.environ.get("MEGATRON_CONFIG"),
            "MEGATRON_LM_PATH": os.environ.get("MEGATRON_LM_PATH"),
            
            # JAX 特定
            "JAX_BACKEND": os.environ.get("JAX_BACKEND"),
            "JAX_PLATFORMS": os.environ.get("JAX_PLATFORMS"),
            
            # PyTorch Lightning
            "PL_TRAINER_GPUS": os.environ.get("PL_TRAINER_GPUS"),
            
            # Hugging Face Transformers
            "TRANSFORMERS_CACHE": os.environ.get("TRANSFORMERS_CACHE"),
            
            # 分布式训练相关
            "WORLD_SIZE": os.environ.get("WORLD_SIZE"),
            "RANK": os.environ.get("RANK"),
            "LOCAL_RANK": os.environ.get("LOCAL_RANK"),
            "MASTER_ADDR": os.environ.get("MASTER_ADDR"),
            "MASTER_PORT": os.environ.get("MASTER_PORT"),
            
            # Kubernetes 相关
            "WORKLOAD_UID": os.environ.get("WORKLOAD_UID"),
            "POD_UID": os.environ.get("POD_UID"),
            "POD_NAME": os.environ.get("POD_NAME"),
            "POD_NAMESPACE": os.environ.get("POD_NAMESPACE"),
        }
        
        # 过滤掉 None 值
        return {k: v for k, v in env_vars.items() if v is not None}
    
    def _extract_pytorch_info(self) -> Optional[Dict[str, Any]]:
        """提取 PyTorch 相关信息"""
        try:
            import torch
            
            info = {
                "available": True,
                "version": torch.__version__,
                "cuda_available": torch.cuda.is_available(),
            }
            
            if torch.cuda.is_available():
                info["cuda_version"] = torch.version.cuda
            
            # 检测已导入的框架模块
            imported_modules = sys.modules.keys()
            info["detected_modules"] = {
                "deepspeed": "deepspeed" in imported_modules,
                "megatron": any("megatron" in mod for mod in imported_modules),
                "transformers": "transformers" in imported_modules,
                "lightning": "pytorch_lightning" in imported_modules or "lightning" in imported_modules,
            }
            
            return info
            
        except ImportError:
            return {"available": False}
        except Exception as e:
            print(f"[Primus Lens Data Collector] Failed to extract PyTorch info: {e}")
            return {"available": False, "error": str(e)}
    
    def _get_framework_hints(self, evidence: Dict[str, Any]) -> Dict[str, Any]:
        """
        生成轻量级预判断线索
        
        Args:
            evidence: 原始证据数据
        
        Returns:
            Dict: hints 数据
        """
        hints = {
            "possible_frameworks": [],
            "confidence": "low",  # low/medium/high
            "primary_indicators": [],
            "timestamp": time.time(),
        }
        
        env = evidence.get("environment", {})
        wandb_config = evidence.get("wandb", {}).get("config", {})
        pytorch_info = evidence.get("pytorch", {})
        
        # === 收集线索 ===
        
        # 1. 从环境变量收集（强指标）
        self._collect_env_hints(env, hints)
        
        # 2. 从 wandb config 收集（中等指标）
        self._collect_config_hints(wandb_config, hints)
        
        # 3. 从 PyTorch 模块收集（弱指标）
        self._collect_pytorch_hints(pytorch_info, hints)
        
        # 4. 从 wandb project name 收集（最弱指标）
        self._collect_project_hints(evidence.get("wandb", {}), hints)
        
        # === 评估置信度 ===
        hints["confidence"] = self._evaluate_confidence(hints["primary_indicators"])
        
        # 去重
        hints["possible_frameworks"] = list(set(hints["possible_frameworks"]))
        
        return hints
    
    def _collect_env_hints(self, env: Dict[str, str], hints: Dict[str, Any]):
        """从环境变量收集 hints"""
        # Primus
        if env.get("PRIMUS_CONFIG") or env.get("PRIMUS_VERSION"):
            hints["possible_frameworks"].append("primus")
            hints["primary_indicators"].append("PRIMUS env vars")
        
        # DeepSpeed
        if env.get("DEEPSPEED_CONFIG") or env.get("DS_CONFIG") or env.get("DEEPSPEED_VERSION"):
            hints["possible_frameworks"].append("deepspeed")
            hints["primary_indicators"].append("DEEPSPEED env vars")
        
        # Megatron
        if env.get("MEGATRON_CONFIG") or env.get("MEGATRON_LM_PATH"):
            hints["possible_frameworks"].append("megatron")
            hints["primary_indicators"].append("MEGATRON env vars")
        
        # JAX
        if env.get("JAX_BACKEND"):
            hints["possible_frameworks"].append("jax")
            hints["primary_indicators"].append("JAX env vars")
        
        # 通用 FRAMEWORK 环境变量
        if env.get("FRAMEWORK"):
            fw = env["FRAMEWORK"].lower()
            if fw not in hints["possible_frameworks"]:
                hints["possible_frameworks"].append(fw)
            hints["primary_indicators"].append(f"FRAMEWORK={fw}")
    
    def _collect_config_hints(self, wandb_config: Dict[str, Any], hints: Dict[str, Any]):
        """从 WandB config 收集 hints"""
        # 检查 config.framework 字段
        if "framework" in wandb_config:
            fw = str(wandb_config["framework"]).lower()
            if fw not in hints["possible_frameworks"]:
                hints["possible_frameworks"].append(fw)
            hints["primary_indicators"].append("wandb_config.framework")
        
        # 检查 config.trainer 字段
        if "trainer" in wandb_config:
            trainer = str(wandb_config["trainer"]).lower()
            if "deepspeed" in trainer and "deepspeed" not in hints["possible_frameworks"]:
                hints["possible_frameworks"].append("deepspeed")
                hints["primary_indicators"].append("wandb_config.trainer")
    
    def _collect_pytorch_hints(self, pytorch_info: Dict[str, Any], hints: Dict[str, Any]):
        """从 PyTorch 模块收集 hints"""
        if not pytorch_info.get("available"):
            return
        
        modules = pytorch_info.get("detected_modules", {})
        
        if modules.get("deepspeed"):
            if "deepspeed" not in hints["possible_frameworks"]:
                hints["possible_frameworks"].append("deepspeed")
            hints["primary_indicators"].append("pytorch.modules.deepspeed")
        
        if modules.get("megatron"):
            if "megatron" not in hints["possible_frameworks"]:
                hints["possible_frameworks"].append("megatron")
            hints["primary_indicators"].append("pytorch.modules.megatron")
    
    def _collect_project_hints(self, wandb_info: Dict[str, Any], hints: Dict[str, Any]):
        """从 WandB project name 收集 hints"""
        project = wandb_info.get("project", "")
        if not project:
            return
        
        project_lower = project.lower()
        frameworks = ["primus", "deepspeed", "megatron", "jax"]
        
        for framework in frameworks:
            if framework in project_lower and framework not in hints["possible_frameworks"]:
                hints["possible_frameworks"].append(framework)
                hints["primary_indicators"].append(f"project_name={project}")
    
    def _evaluate_confidence(self, indicators: List[str]) -> str:
        """评估置信度"""
        strong_indicators = sum(1 for ind in indicators 
                               if "env vars" in ind or "FRAMEWORK=" in ind)
        medium_indicators = sum(1 for ind in indicators 
                               if "wandb_config" in ind)
        
        if strong_indicators >= 2:
            return "high"
        elif strong_indicators >= 1 or medium_indicators >= 2:
            return "medium"
        else:
            return "low"

