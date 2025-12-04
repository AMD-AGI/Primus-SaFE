"""
Data Collector - Collect raw data for framework detection
"""
import os
import sys
import time
from typing import Dict, List, Any, Optional

# Import logger module
from .logger import debug_log, error_log, warning_log


class DataCollector:
    """Data Collector - Collect raw evidence data"""
    
    def __init__(self):
        self.collector_version = "1.0.0"
        self.data_schema_version = "1.0"
    
    def collect_detection_data(self, wandb_run) -> Dict[str, Any]:
        """
        Collect framework detection data
        
        Args:
            wandb_run: WandB run object
        
        Returns:
            Dict: Complete detection data
        """
        debug_log(f"[Primus Lens Data Collector] collect_detection_data() called")
        debug_log(f"[Primus Lens Data Collector] WandB run: {wandb_run.name if wandb_run and hasattr(wandb_run, 'name') else 'None'}")
        
        # Collect raw evidence
        debug_log(f"[Primus Lens Data Collector] Collecting raw evidence...")
        evidence = self._collect_raw_evidence(wandb_run)
        debug_log(f"[Primus Lens Data Collector] Evidence collected, keys: {list(evidence.keys())}")
        
        # Generate hints
        debug_log(f"[Primus Lens Data Collector] Generating framework hints...")
        hints = self._get_framework_hints(evidence)
        debug_log(f"[Primus Lens Data Collector] Hints generated: possible_frameworks={hints.get('possible_frameworks')}, confidence={hints.get('confidence')}")
        
        # Construct complete reporting data
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
        Collect raw evidence data
        
        Args:
            wandb_run: WandB run object
        
        Returns:
            Dict: Raw evidence data
        """
        debug_log(f"[Primus Lens Data Collector] _collect_raw_evidence() started")
        
        # 1. WandB related information
        debug_log(f"[Primus Lens Data Collector] Extracting WandB info...")
        wandb_info = self._extract_wandb_info(wandb_run)
        debug_log(f"[Primus Lens Data Collector] WandB info extracted: project={wandb_info.get('project')}, run_id={wandb_info.get('id')}")
        
        # 2. Environment variables (framework identification related)
        debug_log(f"[Primus Lens Data Collector] Extracting environment variables...")
        env_vars = self._extract_environment_vars()
        debug_log(f"[Primus Lens Data Collector] Found {len(env_vars)} relevant environment variables")
        
        # 3. PyTorch information (if available)
        debug_log(f"[Primus Lens Data Collector] Extracting PyTorch info...")
        pytorch_info = self._extract_pytorch_info()
        debug_log(f"[Primus Lens Data Collector] PyTorch available: {pytorch_info.get('available', False)}")
        
        # 4. Wrapper framework detection (via import)
        debug_log(f"[Primus Lens Data Collector] Detecting wrapper frameworks by import...")
        wrapper_frameworks = self._detect_wrapper_by_import()
        debug_log(f"[Primus Lens Data Collector] Detected {len(wrapper_frameworks)} wrapper framework(s): {list(wrapper_frameworks.keys())}")
        
        # 5. Base framework detection (via import)
        debug_log(f"[Primus Lens Data Collector] Detecting base frameworks by import...")
        base_frameworks = self._detect_base_by_import()
        debug_log(f"[Primus Lens Data Collector] Detected {len(base_frameworks)} base framework(s): {list(base_frameworks.keys())}")
        
        evidence = {
            "wandb": wandb_info,
            "environment": env_vars,
            "pytorch": pytorch_info,
            "wrapper_frameworks": wrapper_frameworks,  # New: wrapper frameworks detected via import
            "base_frameworks": base_frameworks,        # New: base frameworks detected via import
            # 6. System information
            "system": {
                "python_version": sys.version,
                "python_executable": sys.executable,
                "platform": sys.platform,
            },
        }
        
        debug_log(f"[Primus Lens Data Collector] _collect_raw_evidence() completed")
        
        return evidence
    
    def _extract_wandb_info(self, wandb_run) -> Dict[str, Any]:
        """Extract WandB information"""
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
        """Safely get wandb config"""
        try:
            if wandb_run and hasattr(wandb_run, 'config'):
                # Try to convert to dict
                if hasattr(wandb_run.config, '_as_dict'):
                    return wandb_run.config._as_dict()
                elif hasattr(wandb_run.config, 'as_dict'):
                    return wandb_run.config.as_dict()
                elif isinstance(wandb_run.config, dict):
                    return wandb_run.config
                else:
                    # Try to access attributes directly
                    return {k: v for k, v in wandb_run.config.__dict__.items() 
                            if not k.startswith('_')}
        except Exception as e:
            warning_log(f"[Primus Lens Data Collector] Failed to get config: {e}")
        
        return {}
    
    def _extract_environment_vars(self) -> Dict[str, str]:
        """Extract framework-related environment variables"""
        env_vars = {
            # Generic framework identifiers
            "FRAMEWORK": os.environ.get("FRAMEWORK"),
            "TRAINING_FRAMEWORK": os.environ.get("TRAINING_FRAMEWORK"),
            
            # Primus specific
            "PRIMUS_CONFIG": os.environ.get("PRIMUS_CONFIG"),
            "PRIMUS_VERSION": os.environ.get("PRIMUS_VERSION"),
            "PRIMUS_BACKEND": os.environ.get("PRIMUS_BACKEND"),
            
            # DeepSpeed specific
            "DEEPSPEED_CONFIG": os.environ.get("DEEPSPEED_CONFIG"),
            "DEEPSPEED_VERSION": os.environ.get("DEEPSPEED_VERSION"),
            "DS_CONFIG": os.environ.get("DS_CONFIG"),
            
            # Megatron specific
            "MEGATRON_CONFIG": os.environ.get("MEGATRON_CONFIG"),
            "MEGATRON_LM_PATH": os.environ.get("MEGATRON_LM_PATH"),
            
            # JAX specific
            "JAX_BACKEND": os.environ.get("JAX_BACKEND"),
            "JAX_PLATFORMS": os.environ.get("JAX_PLATFORMS"),
            
            # PyTorch Lightning
            "PL_TRAINER_GPUS": os.environ.get("PL_TRAINER_GPUS"),
            
            # Hugging Face Transformers
            "TRANSFORMERS_CACHE": os.environ.get("TRANSFORMERS_CACHE"),
            
            # Distributed training related
            "WORLD_SIZE": os.environ.get("WORLD_SIZE"),
            "RANK": os.environ.get("RANK"),
            "LOCAL_RANK": os.environ.get("LOCAL_RANK"),
            "MASTER_ADDR": os.environ.get("MASTER_ADDR"),
            "MASTER_PORT": os.environ.get("MASTER_PORT"),
            
            # Kubernetes related
            "WORKLOAD_UID": os.environ.get("WORKLOAD_UID"),
            "POD_UID": os.environ.get("POD_UID"),
            "POD_NAME": os.environ.get("POD_NAME"),
            "POD_NAMESPACE": os.environ.get("POD_NAMESPACE"),
        }
        
        # Filter out None values
        return {k: v for k, v in env_vars.items() if v is not None}
    
    def _detect_wrapper_by_import(self) -> Dict[str, Any]:
        """
        Detect Wrapper frameworks via import
        
        Supported Wrapper frameworks:
        - Primus: Enterprise-level training framework
        - PyTorch Lightning: High-level PyTorch wrapper
        - Hugging Face Trainer: Transformers training wrapper
        
        Returns:
            Dict: Detected wrapper framework information
        """
        detected_wrappers = {}
        
        # 1. Detect Primus
        try:
            import primus
            primus_info = {
                "detected": True,
                "version": getattr(primus, '__version__', 'unknown'),
                "initialized": False,
                "base_framework": None
            }
            
            # Check if initialized
            try:
                from primus.core.utils.global_vars import is_initialized, get_primus_config
                if is_initialized():
                    primus_info["initialized"] = True
                    config = get_primus_config()
                    
                    # Try to get underlying framework information
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
        
        # 2. Detect PyTorch Lightning
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
            # Try new version of lightning
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
        
        # 3. Detect Hugging Face Trainer
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
        Detect Base frameworks via import
        
        Supported Base frameworks:
        - Megatron-LM: NVIDIA large-scale language model training framework
        - DeepSpeed: Microsoft distributed training optimization framework
        - JAX: Google high-performance machine learning framework
        - Transformers: Hugging Face model library
        
        Returns:
            Dict: Detected base framework information
        """
        detected_bases = {}
        
        # 1. Detect Megatron-LM
        try:
            import megatron
            megatron_info = {
                "detected": True,
                "version": getattr(megatron, '__version__', 'unknown'),
                "initialized": False
            }
            
            # Check if initialized
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
        
        # 2. Detect DeepSpeed
        try:
            import deepspeed
            deepspeed_info = {
                "detected": True,
                "version": getattr(deepspeed, '__version__', 'unknown'),
                "initialized": False
            }
            
            # Check if initialized
            if hasattr(deepspeed, 'is_initialized'):
                try:
                    deepspeed_info["initialized"] = deepspeed.is_initialized()
                except:
                    pass
            
            detected_bases["deepspeed"] = deepspeed_info
            debug_log(f"[Primus Lens Data Collector] Detected DeepSpeed: version={deepspeed_info['version']}")
        except ImportError:
            pass
        
        # 3. Detect JAX
        try:
            import jax
            jax_info = {
                "detected": True,
                "version": getattr(jax, '__version__', 'unknown'),
                "backend": None,
                "devices": 0
            }
            
            # Get JAX configuration information
            try:
                jax_info["backend"] = jax.default_backend()
                jax_info["devices"] = len(jax.devices())
            except:
                pass
            
            detected_bases["jax"] = jax_info
            debug_log(f"[Primus Lens Data Collector] Detected JAX: version={jax_info['version']}, backend={jax_info['backend']}")
        except ImportError:
            pass
        
        # 4. Detect Transformers (as base framework)
        try:
            import transformers
            # Only consider as base if Trainer is not detected as wrapper
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
        """Extract PyTorch-related information"""
        try:
            import torch
            
            info = {
                "available": True,
                "version": torch.__version__,
                "cuda_available": torch.cuda.is_available(),
            }
            
            if torch.cuda.is_available():
                info["cuda_version"] = torch.version.cuda
            
            # Detect imported framework modules
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
        Generate lightweight prediction hints (supports two-layer framework detection)
        
        Framework Detection Layers:
        - wrapper_frameworks: Outer wrapper frameworks (e.g., Primus)
        - base_frameworks: Underlying base frameworks (e.g., Megatron, JAX, DeepSpeed)
        
        Args:
            evidence: Raw evidence data
        
        Returns:
            Dict: hints data containing layered framework information
        """
        debug_log(f"[Primus Lens Data Collector] _get_framework_hints() started")
        
        hints = {
            "wrapper_frameworks": [],      # Outer wrapper frameworks (e.g., Primus)
            "base_frameworks": [],          # Underlying base frameworks (e.g., Megatron, JAX)
            "possible_frameworks": [],      # Maintain compatibility: all detected frameworks
            "confidence": "low",            # low/medium/high
            "primary_indicators": [],
            "framework_layers": {},         # Framework hierarchy mapping
            "timestamp": time.time(),
        }
        
        env = evidence.get("environment", {})
        wandb_config = evidence.get("wandb", {}).get("config", {})
        pytorch_info = evidence.get("pytorch", {})
        wrapper_by_import = evidence.get("wrapper_frameworks", {})
        base_by_import = evidence.get("base_frameworks", {})
        
        # === Collect hints ===
        
        # 0. Collect from import detection (strongest indicator)
        debug_log(f"[Primus Lens Data Collector] Collecting hints from import detection...")
        self._collect_import_hints(wrapper_by_import, base_by_import, hints)
        debug_log(f"[Primus Lens Data Collector] Import hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 1. Collect from environment variables (strong indicator)
        debug_log(f"[Primus Lens Data Collector] Collecting hints from environment variables...")
        self._collect_env_hints(env, hints)
        debug_log(f"[Primus Lens Data Collector] Env hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 2. Collect from wandb config (medium indicator)
        debug_log(f"[Primus Lens Data Collector] Collecting hints from WandB config...")
        self._collect_config_hints(wandb_config, hints)
        debug_log(f"[Primus Lens Data Collector] Config hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 3. Collect from PyTorch modules (weak indicator)
        debug_log(f"[Primus Lens Data Collector] Collecting hints from PyTorch modules...")
        self._collect_pytorch_hints(pytorch_info, hints)
        debug_log(f"[Primus Lens Data Collector] PyTorch hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # 4. Collect from wandb project name (weakest indicator)
        debug_log(f"[Primus Lens Data Collector] Collecting hints from project name...")
        self._collect_project_hints(evidence.get("wandb", {}), hints)
        debug_log(f"[Primus Lens Data Collector] Project hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']}")
        
        # === Evaluate confidence ===
        hints["confidence"] = self._evaluate_confidence(hints["primary_indicators"])
        debug_log(f"[Primus Lens Data Collector] Confidence evaluated: {hints['confidence']}")
        
        # Deduplicate
        hints["wrapper_frameworks"] = list(set(hints["wrapper_frameworks"]))
        hints["base_frameworks"] = list(set(hints["base_frameworks"]))
        
        # Build possible_frameworks (maintain backward compatibility)
        hints["possible_frameworks"] = hints["wrapper_frameworks"] + hints["base_frameworks"]
        
        # Build framework hierarchy
        self._build_framework_layers(hints)
        
        debug_log(f"[Primus Lens Data Collector] _get_framework_hints() completed")
        debug_log(f"[Primus Lens Data Collector] Final hints: wrapper={hints['wrapper_frameworks']}, base={hints['base_frameworks']} (confidence: {hints['confidence']})")
        
        return hints
    
    def _collect_import_hints(self, wrapper_by_import: Dict[str, Any], 
                              base_by_import: Dict[str, Any], hints: Dict[str, Any]):
        """
        Collect hints from import detection (strongest indicator)
        
        Transformers as fallback strategy:
        - transformers and transformers_trainer are too basic, many projects will install them
        - Only use them as frameworks when no other more specific frameworks are detected
        
        Args:
            wrapper_by_import: wrapper frameworks detected via import
            base_by_import: base frameworks detected via import
            hints: hints dictionary
        """
        # First collect non-transformers related frameworks
        non_transformers_wrappers = []
        non_transformers_bases = []
        
        # Handle Wrapper frameworks (exclude transformers_trainer)
        for framework_name, framework_info in wrapper_by_import.items():
            if framework_info.get("detected"):
                # Skip transformers_trainer for now, handle it last
                if framework_name == "transformers_trainer":
                    continue
                
                # Add to wrapper_frameworks
                if framework_name not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append(framework_name)
                    non_transformers_wrappers.append(framework_name)
                hints["primary_indicators"].append(f"import.{framework_name}")
                
                # If Primus and has base_framework info, also record it
                if framework_name == "primus" and framework_info.get("base_framework"):
                    base_fw = framework_info["base_framework"].lower()
                    if base_fw not in hints["base_frameworks"]:
                        hints["base_frameworks"].append(base_fw)
                        non_transformers_bases.append(base_fw)
                    hints["primary_indicators"].append(f"primus.base_framework={base_fw}")
        
        # Handle Base frameworks (exclude transformers)
        for framework_name, framework_info in base_by_import.items():
            if framework_info.get("detected"):
                # Skip transformers for now, handle it last
                if framework_name == "transformers":
                    continue
                
                if framework_name not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(framework_name)
                    non_transformers_bases.append(framework_name)
                hints["primary_indicators"].append(f"import.{framework_name}")
        
        # === Fallback strategy: Transformers ===
        # Only add transformers-related frameworks when no other frameworks are detected
        has_other_frameworks = len(non_transformers_wrappers) > 0 or len(non_transformers_bases) > 0
        
        if not has_other_frameworks:
            # No other frameworks, use transformers as fallback
            
            # Add transformers_trainer (if detected)
            if "transformers_trainer" in wrapper_by_import and wrapper_by_import["transformers_trainer"].get("detected"):
                if "transformers_trainer" not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append("transformers_trainer")
                hints["primary_indicators"].append("import.transformers_trainer (fallback)")
                debug_log("[Primus Lens Data Collector] Using transformers_trainer as fallback wrapper framework")
            
            # Add transformers (if detected and not duplicating trainer)
            if "transformers" in base_by_import and base_by_import["transformers"].get("detected"):
                # If transformers_trainer is already added, don't add transformers as base
                if "transformers_trainer" not in hints["wrapper_frameworks"]:
                    if "transformers" not in hints["base_frameworks"]:
                        hints["base_frameworks"].append("transformers")
                    hints["primary_indicators"].append("import.transformers (fallback)")
                    debug_log("[Primus Lens Data Collector] Using transformers as fallback base framework")
        else:
            debug_log(f"[Primus Lens Data Collector] Skipping transformers (found other frameworks: wrappers={non_transformers_wrappers}, bases={non_transformers_bases})")
    
    def _collect_env_hints(self, env: Dict[str, str], hints: Dict[str, Any]):
        """Collect hints from environment variables (layered detection)"""
        # === Wrapper Frameworks (outer wrapper frameworks) ===
        
        # Primus
        if env.get("PRIMUS_CONFIG") or env.get("PRIMUS_VERSION"):
            hints["wrapper_frameworks"].append("primus")
            hints["primary_indicators"].append("PRIMUS env vars")
            
            # If PRIMUS_BACKEND exists, record underlying framework information
            backend = env.get("PRIMUS_BACKEND")
            if backend:
                backend_lower = backend.lower()
                if backend_lower not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(backend_lower)
                hints["primary_indicators"].append(f"PRIMUS_BACKEND={backend}")
        
        # === Base Frameworks (underlying base frameworks) ===
        
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
        
        # === Generic FRAMEWORK environment variable ===
        if env.get("FRAMEWORK") or env.get("TRAINING_FRAMEWORK"):
            fw = (env.get("FRAMEWORK") or env.get("TRAINING_FRAMEWORK")).lower()
            # Determine layer based on framework name
            if fw in ["primus", "lightning", "pytorch_lightning"]:
                if fw not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append(fw)
            else:
                if fw not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(fw)
            hints["primary_indicators"].append(f"FRAMEWORK={fw}")
    
    def _collect_config_hints(self, wandb_config: Dict[str, Any], hints: Dict[str, Any]):
        """Collect hints from WandB config (layered detection)"""
        # Check config.framework field
        if "framework" in wandb_config:
            fw = str(wandb_config["framework"]).lower()
            # Classify by framework type
            if fw in ["primus", "lightning", "pytorch_lightning"]:
                if fw not in hints["wrapper_frameworks"]:
                    hints["wrapper_frameworks"].append(fw)
            else:
                if fw not in hints["base_frameworks"]:
                    hints["base_frameworks"].append(fw)
            hints["primary_indicators"].append("wandb_config.framework")
        
        # Check config.base_framework field (Primus specific)
        if "base_framework" in wandb_config:
            base_fw = str(wandb_config["base_framework"]).lower()
            if base_fw not in hints["base_frameworks"]:
                hints["base_frameworks"].append(base_fw)
            hints["primary_indicators"].append("wandb_config.base_framework")
        
        # Check config.trainer field
        if "trainer" in wandb_config:
            trainer = str(wandb_config["trainer"]).lower()
            if "deepspeed" in trainer and "deepspeed" not in hints["base_frameworks"]:
                hints["base_frameworks"].append("deepspeed")
                hints["primary_indicators"].append("wandb_config.trainer")
            elif "megatron" in trainer and "megatron" not in hints["base_frameworks"]:
                hints["base_frameworks"].append("megatron")
                hints["primary_indicators"].append("wandb_config.trainer")
    
    def _collect_pytorch_hints(self, pytorch_info: Dict[str, Any], hints: Dict[str, Any]):
        """Collect hints from PyTorch modules (layered detection)"""
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
        """Collect hints from WandB project name (layered detection)"""
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
        Build framework hierarchy mapping
        
        Example:
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
        
        # Record wrapper frameworks
        for wrapper in hints["wrapper_frameworks"]:
            layers[wrapper] = {
                "layer": "wrapper",
                "base_frameworks": hints["base_frameworks"].copy()
            }
        
        # Record base frameworks
        for base in hints["base_frameworks"]:
            layers[base] = {
                "layer": "base",
                "wrapper_frameworks": hints["wrapper_frameworks"].copy()
            }
        
        hints["framework_layers"] = layers
    
    def _evaluate_confidence(self, indicators: List[str]) -> str:
        """
        Evaluate confidence level
        
        Indicator strength levels:
        - Strongest: import detection (actual module loaded)
        - Strong: environment variables, FRAMEWORK/BACKEND variables
        - Medium: wandb_config fields
        - Weak: PyTorch modules, project names
        """
        # Strongest indicator: import detection
        import_indicators = sum(1 for ind in indicators if ind.startswith("import."))
        
        # Strong indicators: environment variables
        strong_indicators = sum(1 for ind in indicators 
                               if "env vars" in ind or "FRAMEWORK=" in ind or "BACKEND=" in ind)
        
        # Medium indicators: wandb config
        medium_indicators = sum(1 for ind in indicators 
                               if "wandb_config" in ind)
        
        # If import detection exists, directly set high confidence
        if import_indicators >= 1:
            return "high"
        elif strong_indicators >= 2:
            return "high"
        elif strong_indicators >= 1 or medium_indicators >= 2:
            return "medium"
        else:
            return "low"

