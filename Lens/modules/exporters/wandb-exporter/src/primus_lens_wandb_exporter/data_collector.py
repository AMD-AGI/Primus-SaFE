"""
Data Collector - 采集框架检测需要的原始数据
"""
import os
import sys
import time
from typing import Dict, List, Any, Optional

# 导入日志模块
from .logger import debug_log, error_log, warning_log


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
        debug_log(f"[Primus Lens Data Collector] collect_detection_data() called")
        debug_log(f"[Primus Lens Data Collector] WandB run: {wandb_run.name if wandb_run and hasattr(wandb_run, 'name') else 'None'}")
        
        # 采集原始证据
        debug_log(f"[Primus Lens Data Collector] Collecting raw evidence...")
        evidence = self._collect_raw_evidence(wandb_run)
        debug_log(f"[Primus Lens Data Collector] Evidence collected, keys: {list(evidence.keys())}")
        
        # 生成 hints
        debug_log(f"[Primus Lens Data Collector] Generating framework hints...")
        hints = self._get_framework_hints(evidence)
        debug_log(f"[Primus Lens Data Collector] Hints generated: possible_frameworks={hints.get('possible_frameworks')}, confidence={hints.get('confidence')}")
        
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
        
        debug_log(f"[Primus Lens Data Collector] Detection data prepared, workload_uid={detection_data['workload_uid']}")
        
        return detection_data
    
    def _collect_raw_evidence(self, wandb_run) -> Dict[str, Any]:
        """
        采集原始证据数据
        
        Args:
            wandb_run: WandB run 对象
        
        Returns:
            Dict: 原始证据数据
        """
        debug_log(f"[Primus Lens Data Collector] _collect_raw_evidence() started")
        
        # 1. WandB 相关信息
        debug_log(f"[Primus Lens Data Collector] Extracting WandB info...")
        wandb_info = self._extract_wandb_info(wandb_run)
        debug_log(f"[Primus Lens Data Collector] WandB info extracted: project={wandb_info.get('project')}, run_id={wandb_info.get('id')}")
        
        # 2. 环境变量（框架识别相关）
        debug_log(f"[Primus Lens Data Collector] Extracting environment variables...")
        env_vars = self._extract_environment_vars()
        debug_log(f"[Primus Lens Data Collector] Found {len(env_vars)} relevant environment variables")
        
        # 3. PyTorch 信息（如果可用）
        debug_log(f"[Primus Lens Data Collector] Extracting PyTorch info...")
        pytorch_info = self._extract_pytorch_info()
        debug_log(f"[Primus Lens Data Collector] PyTorch available: {pytorch_info.get('available', False)}")
        
        # 4. Wrapper 框架检测（通过 import）
        debug_log(f"[Primus Lens Data Collector] Detecting wrapper frameworks by import...")
        wrapper_frameworks = self._detect_wrapper_by_import()
        debug_log(f"[Primus Lens Data Collector] Detected {len(wrapper_frameworks)} wrapper framework(s): {list(wrapper_frameworks.keys())}")
        
        # 5. Base 框架检测（通过 import）
        debug_log(f"[Primus Lens Data Collector] Detecting base frameworks by import...")
        base_frameworks = self._detect_base_by_import()
        debug_log(f"[Primus Lens Data Collector] Detected {len(base_frameworks)} base framework(s): {list(base_frameworks.keys())}")
        
        evidence = {
            "wandb": wandb_info,
            "environment": env_vars,
            "pytorch": pytorch_info,
            "wrapper_frameworks": wrapper_frameworks,  # 新增：通过 import 检测的 wrapper 框架
            "base_frameworks": base_frameworks,        # 新增：通过 import 检测的 base 框架
            # 6. 系统信息
            "system": {
                "python_version": sys.version,
                "python_executable": sys.executable,
                "platform": sys.platform,
            },
        }
        
        debug_log(f"[Primus Lens Data Collector] _collect_raw_evidence() completed")
        
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
            warning_log(f"[Primus Lens Data Collector] Failed to extract wandb info: {e}")
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
            warning_log(f"[Primus Lens Data Collector] Failed to get config: {e}")
        
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
    
    def _detect_wrapper_by_import(self) -> Dict[str, Any]:
        """
        通过 import 检测 Wrapper 框架
        
        支持的 Wrapper 框架：
        - Primus: 企业级训练框架
        - PyTorch Lightning: PyTorch 高级封装
        - Hugging Face Trainer: Transformers 训练封装
        
        Returns:
            Dict: 检测到的 wrapper 框架信息
        """
        detected_wrappers = {}
        
        # 1. 检测 Primus
        try:
            import primus
            primus_info = {
                "detected": True,
                "version": getattr(primus, '__version__', 'unknown'),
                "initialized": False,
                "base_framework": None
            }
            
            # 检查是否初始化
            try:
                from primus.core.utils.global_vars import is_initialized, get_primus_config
                if is_initialized():
                    primus_info["initialized"] = True
                    config = get_primus_config()
                    
                    # 尝试获取底层框架信息
                    try:
                        pre_trainer_cfg = config.get_module_config("pre_trainer")
                        primus_info["base_framework"] = pre_trainer_cfg.framework
                    except:
                        pass
            except:
                pass
            
            detected_wrappers["primus"] = primus_info
            debug_log(f"[Primus Lens Data Collector] Detected Primus: version={primus_info['version']}, initialized={primus_info['initialized']}")
        except ImportError:
            pass
        
        # 2. 检测 PyTorch Lightning
        try:
            import pytorch_lightning as pl
            lightning_info = {
                "detected": True,
                "version": getattr(pl, '__version__', 'unknown'),
                "module_name": "pytorch_lightning",
                "trainer_available": hasattr(pl, 'Trainer')
            }
            detected_wrappers["lightning"] = lightning_info
            debug_log(f"[Primus Lens Data Collector] Detected Lightning: version={lightning_info['version']}")
        except ImportError:
            # 尝试新版本的 lightning
            try:
                import lightning as L
                lightning_info = {
                    "detected": True,
                    "version": getattr(L, '__version__', 'unknown'),
                    "module_name": "lightning",
                    "trainer_available": hasattr(L, 'Trainer')
                }
                detected_wrappers["lightning"] = lightning_info
                debug_log(f"[Primus Lens Data Collector] Detected Lightning (new): version={lightning_info['version']}")
            except ImportError:
                pass
        
        # 3. 检测 Hugging Face Trainer
        try:
            from transformers import Trainer, TrainingArguments
            import transformers
            trainer_info = {
                "detected": True,
                "version": getattr(transformers, '__version__', 'unknown'),
                "has_trainer": True,
                "has_training_arguments": True
            }
            detected_wrappers["transformers_trainer"] = trainer_info
            debug_log(f"[Primus Lens Data Collector] Detected Transformers Trainer: version={trainer_info['version']}")
        except ImportError:
            pass
        
        return detected_wrappers
    
    def _detect_base_by_import(self) -> Dict[str, Any]:
        """
        通过 import 检测 Base 框架
        
        支持的 Base 框架：
        - Megatron-LM: NVIDIA 大规模语言模型训练框架
        - DeepSpeed: Microsoft 分布式训练优化框架
        - JAX: Google 高性能机器学习框架
        - Transformers: Hugging Face 模型库
        
        Returns:
            Dict: 检测到的 base 框架信息
        """
        detected_bases = {}
        
        # 1. 检测 Megatron-LM
        try:
            import megatron
            megatron_info = {
                "detected": True,
                "version": getattr(megatron, '__version__', 'unknown'),
                "initialized": False
            }
            
            # 检查是否已初始化
            try:
                from megatron.training import get_args
                args = get_args()
                megatron_info["initialized"] = True
            except:
                pass
            
            detected_bases["megatron"] = megatron_info
            debug_log(f"[Primus Lens Data Collector] Detected Megatron: initialized={megatron_info['initialized']}")
        except ImportError:
            pass
        
        # 2. 检测 DeepSpeed
        try:
            import deepspeed
            deepspeed_info = {
                "detected": True,
                "version": getattr(deepspeed, '__version__', 'unknown'),
                "initialized": False
            }
            
            # 检查是否已初始化
            if hasattr(deepspeed, 'is_initialized'):
                try:
                    deepspeed_info["initialized"] = deepspeed.is_initialized()
                except:
                    pass
            
            detected_bases["deepspeed"] = deepspeed_info
            debug_log(f"[Primus Lens Data Collector] Detected DeepSpeed: version={deepspeed_info['version']}")
        except ImportError:
            pass
        
        # 3. 检测 JAX
        try:
            import jax
            jax_info = {
                "detected": True,
                "version": getattr(jax, '__version__', 'unknown'),
                "backend": None,
                "devices": 0
            }
            
            # 获取 JAX 配置信息
            try:
                jax_info["backend"] = jax.default_backend()
                jax_info["devices"] = len(jax.devices())
            except:
                pass
            
            detected_bases["jax"] = jax_info
            debug_log(f"[Primus Lens Data Collector] Detected JAX: version={jax_info['version']}, backend={jax_info['backend']}")
        except ImportError:
            pass
        
        # 4. 检测 Transformers（作为 base 框架）
        try:
            import transformers
            # 只有在没有检测到 Trainer 作为 wrapper 时才作为 base
            transformers_info = {
                "detected": True,
                "version": getattr(transformers, '__version__', 'unknown')
            }
            detected_bases["transformers"] = transformers_info
            debug_log(f"[Primus Lens Data Collector] Detected Transformers: version={transformers_info['version']}")
        except ImportError:
            pass
        
        return detected_bases
    
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
            warning_log(f"[Primus Lens Data Collector] Failed to extract PyTorch info: {e}")
            return {"available": False, "error": str(e)}
    
    def _get_framework_hints(self, evidence: Dict[str, Any]) -> Dict[str, Any]:
        """
        生成轻量级预判断线索（支持两层框架检测）
        
        Framework Detection Layers:
        - wrapper_frameworks: 外层包装框架（如 Primus）
        - base_frameworks: 底层基础框架（如 Megatron、JAX、DeepSpeed）
        
        Args:
            evidence: 原始证据数据
        
        Returns:
            Dict: hints 数据，包含分层的框架信息
        """
        debug_log(f"[Primus Lens Data Collector] _get_framework_hints() started")
        
        hints = {
            "wrapper_frameworks": [],      # 外层包装框架（如 Primus）
            "base_frameworks": [],          # 底层基础框架（如 Megatron、JAX）
            "possible_frameworks": [],      # 保留兼容性：所有检测到的框架
            "confidence": "low",            # low/medium/high
            "primary_indicators": [],
            "framework_layers": {},         # 框架层级关系映射
            "timestamp": time.time(),
        }
        
        env = evidence.get("environment", {})
        wandb_config = evidence.get("wandb", {}).get("config", {})
        pytorch_info = evidence.get("pytorch", {})
        wrapper_by_import = evidence.get("wrapper_frameworks", {})
        base_by_import = evidence.get("base_frameworks", {})
        
        # === 收集线索 ===
        
        # 0. 从 import 检测收集（最强指标）
        debug_log(f"[Primus Lens Data Collector] Collecting hints from import detection...")
        self._collect_import_hints(wrapper_by_import, base_by_import, hints)
        debug_log(f"[Primus Lens Data Collector] Import hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 1. 从环境变量收集（强指标）
        debug_log(f"[Primus Lens Data Collector] Collecting hints from environment variables...")
        self._collect_env_hints(env, hints)
        debug_log(f"[Primus Lens Data Collector] Env hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 2. 从 wandb config 收集（中等指标）
        debug_log(f"[Primus Lens Data Collector] Collecting hints from WandB config...")
        self._collect_config_hints(wandb_config, hints)
        debug_log(f"[Primus Lens Data Collector] Config hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 3. 从 PyTorch 模块收集（弱指标）
        debug_log(f"[Primus Lens Data Collector] Collecting hints from PyTorch modules...")
        self._collect_pytorch_hints(pytorch_info, hints)
        debug_log(f"[Primus Lens Data Collector] PyTorch hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 4. 从 wandb project name 收集（最弱指标）
        debug_log(f"[Primus Lens Data Collector] Collecting hints from project name...")
        self._collect_project_hints(evidence.get("wandb", {}), hints)
        debug_log(f"[Primus Lens Data Collector] Project hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # === 评估置信度 ===
        hints["confidence"] = self._evaluate_confidence(hints["primary_indicators"])
        debug_log(f"[Primus Lens Data Collector] Confidence evaluated: {hints['confidence']}")
        
        # 去重
        hints["wrapper_frameworks"] = list(set(hints["wrapper_frameworks"]))
        hints["base_frameworks"] = list(set(hints["base_frameworks"]))
        
        # 构建 possible_frameworks（保持向后兼容）
        hints["possible_frameworks"] = hints["wrapper_frameworks"] + hints["base_frameworks"]
        
        # 构建框架层级关系
        self._build_framework_layers(hints)
        
        debug_log(f"[Primus Lens Data Collector] _get_framework_hints() completed")
        debug_log(f"[Primus Lens Data Collector] Final hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']} (confidence: {hints['confidence']})")
        
        return hints
    
    def _collect_import_hints(self, wrapper_by_import: Dict[str, Any], 
                              base_by_import: Dict[str, Any], hints: Dict[str, Any]):
        """
        从 import 检测收集 hints（最强指标）
        
        Transformers 作为兜底策略：
        - transformers 和 transformers_trainer 太基础，很多项目都会安装
        - 只有在没有检测到其他更具体的框架时，才将它们作为框架
        
        Args:
            wrapper_by_import: 通过 import 检测到的 wrapper 框架
            base_by_import: 通过 import 检测到的 base 框架
            hints: hints 字典
        """
        # 先收集非 transformers 相关的框架
        non_transformers_wrappers = []
        non_transformers_bases = []
        
        # 处理 Wrapper 框架（排除 transformers_trainer）
        for framework_name, framework_info in wrapper_by_import.items():
            if framework_info.get("detected"):
                # 暂时跳过 transformers_trainer，最后处理
                if framework_name == "transformers_trainer":
                    continue
                
                # 添加到 wrapper_frameworks
                if framework_name not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append(framework_name)
                    non_transformers_wrappers.append(framework_name)
                hints["primary_indicators"].append(f"import.{framework_name}")
                
                # 如果是 Primus 且有 base_framework 信息，也记录下来
                if framework_name == "primus" and framework_info.get("base_framework"):
                    base_fw = framework_info["base_framework"].lower()
                    if base_fw not in hints["base_frameworks"]:
                        hints["base_frameworks"].append(base_fw)
                        non_transformers_bases.append(base_fw)
                    hints["primary_indicators"].append(f"primus.base_framework={base_fw}")
        
        # 处理 Base 框架（排除 transformers）
        for framework_name, framework_info in base_by_import.items():
            if framework_info.get("detected"):
                # 暂时跳过 transformers，最后处理
                if framework_name == "transformers":
                    continue
                
                if framework_name not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(framework_name)
                    non_transformers_bases.append(framework_name)
                hints["primary_indicators"].append(f"import.{framework_name}")
        
        # === 兜底策略：Transformers ===
        # 只有在没有检测到其他框架时，才添加 transformers 相关框架
        has_other_frameworks = len(non_transformers_wrappers) > 0 or len(non_transformers_bases) > 0
        
        if not has_other_frameworks:
            # 没有其他框架，使用 transformers 作为兜底
            
            # 添加 transformers_trainer（如果检测到）
            if "transformers_trainer" in wrapper_by_import and wrapper_by_import["transformers_trainer"].get("detected"):
                if "transformers_trainer" not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append("transformers_trainer")
                hints["primary_indicators"].append("import.transformers_trainer (fallback)")
                debug_log("[Primus Lens Data Collector] Using transformers_trainer as fallback wrapper framework")
            
            # 添加 transformers（如果检测到且不与 trainer 重复）
            if "transformers" in base_by_import and base_by_import["transformers"].get("detected"):
                # 如果已经添加了 transformers_trainer，就不再添加 transformers 作为 base
                if "transformers_trainer" not in hints["wrapper_frameworks"]:
                    if "transformers" not in hints["base_frameworks"]:
                        hints["base_frameworks"].append("transformers")
                    hints["primary_indicators"].append("import.transformers (fallback)")
                    debug_log("[Primus Lens Data Collector] Using transformers as fallback base framework")
        else:
            debug_log(f"[Primus Lens Data Collector] Skipping transformers (found other frameworks: wrappers={non_transformers_wrappers}, bases={non_transformers_bases})")
    
    def _collect_env_hints(self, env: Dict[str, str], hints: Dict[str, Any]):
        """从环境变量收集 hints（分层检测）"""
        # === Wrapper Frameworks（外层包装框架）===
        
        # Primus
        if env.get("PRIMUS_CONFIG") or env.get("PRIMUS_VERSION"):
            hints["wrapper_frameworks"].append("primus")
            hints["primary_indicators"].append("PRIMUS env vars")
            
            # 如果有 PRIMUS_BACKEND，记录底层框架信息
            backend = env.get("PRIMUS_BACKEND")
            if backend:
                backend_lower = backend.lower()
                if backend_lower not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(backend_lower)
                hints["primary_indicators"].append(f"PRIMUS_BACKEND={backend}")
        
        # === Base Frameworks（底层基础框架）===
        
        # DeepSpeed
        if env.get("DEEPSPEED_CONFIG") or env.get("DS_CONFIG") or env.get("DEEPSPEED_VERSION"):
            hints["base_frameworks"].append("deepspeed")
            hints["primary_indicators"].append("DEEPSPEED env vars")
        
        # Megatron
        if env.get("MEGATRON_CONFIG") or env.get("MEGATRON_LM_PATH"):
            hints["base_frameworks"].append("megatron")
            hints["primary_indicators"].append("MEGATRON env vars")
        
        # JAX
        if env.get("JAX_BACKEND") or env.get("JAX_PLATFORMS"):
            hints["base_frameworks"].append("jax")
            hints["primary_indicators"].append("JAX env vars")
        
        # === 通用 FRAMEWORK 环境变量 ===
        if env.get("FRAMEWORK") or env.get("TRAINING_FRAMEWORK"):
            fw = (env.get("FRAMEWORK") or env.get("TRAINING_FRAMEWORK")).lower()
            # 根据框架名称判断层级
            if fw in ["primus", "lightning", "pytorch_lightning"]:
                if fw not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append(fw)
            else:
                if fw not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(fw)
            hints["primary_indicators"].append(f"FRAMEWORK={fw}")
    
    def _collect_config_hints(self, wandb_config: Dict[str, Any], hints: Dict[str, Any]):
        """从 WandB config 收集 hints（分层检测）"""
        # 检查 config.framework 字段
        if "framework" in wandb_config:
            fw = str(wandb_config["framework"]).lower()
            # 根据框架类型分类
            if fw in ["primus", "lightning", "pytorch_lightning"]:
                if fw not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append(fw)
            else:
                if fw not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(fw)
            hints["primary_indicators"].append("wandb_config.framework")
        
        # 检查 config.base_framework 字段（Primus 特定）
        if "base_framework" in wandb_config:
            base_fw = str(wandb_config["base_framework"]).lower()
            if base_fw not in hints["base_frameworks"]:
                hints["base_frameworks"].append(base_fw)
            hints["primary_indicators"].append("wandb_config.base_framework")
        
        # 检查 config.trainer 字段
        if "trainer" in wandb_config:
            trainer = str(wandb_config["trainer"]).lower()
            if "deepspeed" in trainer and "deepspeed" not in hints["base_frameworks"]:
                hints["base_frameworks"].append("deepspeed")
                hints["primary_indicators"].append("wandb_config.trainer")
            elif "megatron" in trainer and "megatron" not in hints["base_frameworks"]:
                hints["base_frameworks"].append("megatron")
                hints["primary_indicators"].append("wandb_config.trainer")
    
    def _collect_pytorch_hints(self, pytorch_info: Dict[str, Any], hints: Dict[str, Any]):
        """从 PyTorch 模块收集 hints（分层检测）"""
        if not pytorch_info.get("available"):
            return
        
        modules = pytorch_info.get("detected_modules", {})
        
        # Wrapper frameworks
        if modules.get("lightning"):
            if "lightning" not in hints["wrapper_frameworks"]:
                hints["wrapper_frameworks"].append("lightning")
            hints["primary_indicators"].append("pytorch.modules.lightning")
        
        # Base frameworks
        if modules.get("deepspeed"):
            if "deepspeed" not in hints["base_frameworks"]:
                hints["base_frameworks"].append("deepspeed")
            hints["primary_indicators"].append("pytorch.modules.deepspeed")
        
        if modules.get("megatron"):
            if "megatron" not in hints["base_frameworks"]:
                hints["base_frameworks"].append("megatron")
            hints["primary_indicators"].append("pytorch.modules.megatron")
        
        if modules.get("transformers"):
            if "transformers" not in hints["base_frameworks"]:
                hints["base_frameworks"].append("transformers")
            hints["primary_indicators"].append("pytorch.modules.transformers")
    
    def _collect_project_hints(self, wandb_info: Dict[str, Any], hints: Dict[str, Any]):
        """从 WandB project name 收集 hints（分层检测）"""
        project = wandb_info.get("project", "")
        if not project:
            return
        
        project_lower = project.lower()
        
        # Wrapper frameworks
        wrapper_frameworks = ["primus", "lightning"]
        for framework in wrapper_frameworks:
            if framework in project_lower and framework not in hints["wrapper_frameworks"]:
                hints["wrapper_frameworks"].append(framework)
                hints["primary_indicators"].append(f"project_name={project}")
        
        # Base frameworks
        base_frameworks = ["deepspeed", "megatron", "jax", "transformers"]
        for framework in base_frameworks:
            if framework in project_lower and framework not in hints["base_frameworks"]:
                hints["base_frameworks"].append(framework)
                hints["primary_indicators"].append(f"project_name={project}")
    
    def _build_framework_layers(self, hints: Dict[str, Any]):
        """
        构建框架层级关系映射
        
        示例：
        {
            "primus": {
                "layer": "wrapper",
                "base_frameworks": ["megatron", "deepspeed"]
            },
            "megatron": {
                "layer": "base",
                "wrapper_frameworks": ["primus"]
            }
        }
        """
        layers = {}
        
        # 记录 wrapper frameworks
        for wrapper in hints["wrapper_frameworks"]:
            layers[wrapper] = {
                "layer": "wrapper",
                "base_frameworks": hints["base_frameworks"].copy()
            }
        
        # 记录 base frameworks
        for base in hints["base_frameworks"]:
            layers[base] = {
                "layer": "base",
                "wrapper_frameworks": hints["wrapper_frameworks"].copy()
            }
        
        hints["framework_layers"] = layers
    
    def _evaluate_confidence(self, indicators: List[str]) -> str:
        """
        评估置信度
        
        指标强度分级：
        - 最强：import 检测（实际模块已加载）
        - 强：环境变量、FRAMEWORK/BACKEND 变量
        - 中：wandb_config 字段
        - 弱：PyTorch 模块、项目名称
        """
        # 最强指标：import 检测
        import_indicators = sum(1 for ind in indicators if ind.startswith("import."))
        
        # 强指标：环境变量
        strong_indicators = sum(1 for ind in indicators 
                               if "env vars" in ind or "FRAMEWORK=" in ind or "BACKEND=" in ind)
        
        # 中等指标：wandb config
        medium_indicators = sum(1 for ind in indicators 
                               if "wandb_config" in ind)
        
        # 如果有 import 检测，直接高置信度
        if import_indicators >= 1:
            return "high"
        elif strong_indicators >= 2:
            return "high"
        elif strong_indicators >= 1 or medium_indicators >= 2:
            return "medium"
        else:
            return "low"

