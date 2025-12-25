"""
Software Information Collector - Collect package versions and build information
"""
import os
import importlib.metadata
from typing import Dict, Any, List, Optional

from .logger import debug_log, warning_log


class SoftwareInfoCollector:
    """Software Information Collector"""
    
    # Core packages to track versions
    CORE_PACKAGES = [
        # Deep Learning Frameworks
        "torch",
        "tensorflow",
        "jax",
        "jaxlib",
        
        # Training Frameworks
        "transformers",
        "deepspeed",
        "accelerate",
        "pytorch-lightning",
        "lightning",
        
        # AMD/ROCm specific
        "torch-rocm",
        "triton",
        
        # Optimization & Extensions
        "flash-attn",
        "flash_attn",
        "apex",
        "xformers",
        "bitsandbytes",
        
        # PEFT & LoRA
        "peft",
        "loralib",
        
        # Experiment Tracking
        "wandb",
        "tensorboard",
        "mlflow",
        
        # Data & Utils
        "numpy",
        "scipy",
        "pandas",
        "datasets",
        "tokenizers",
        "safetensors",
        "einops",
        
        # Distributed Training
        "mpi4py",
        "horovod",
        "ray",
    ]
    
    # Build info environment variable mappings
    BUILD_ENV_MAPPING = {
        "build_url": ["BUILD_URL", "CI_BUILD_URL", "JENKINS_BUILD_URL", "GITHUB_RUN_URL"],
        "dockerfile_url": ["DOCKERFILE_URL", "DOCKER_FILE_URL"],
        "image_tag": ["IMAGE_TAG", "DOCKER_IMAGE", "CONTAINER_IMAGE", "DOCKER_TAG"],
        "build_date": ["BUILD_DATE", "BUILD_TIMESTAMP", "BUILD_TIME"],
        "git_commit": ["GIT_COMMIT", "GIT_SHA", "CI_COMMIT_SHA", "GITHUB_SHA"],
        "git_branch": ["GIT_BRANCH", "CI_COMMIT_BRANCH", "GITHUB_REF_NAME"],
        "git_repo": ["GIT_REPO", "GIT_REPOSITORY", "GITHUB_REPOSITORY"],
        "ci_pipeline_id": ["CI_PIPELINE_ID", "GITHUB_RUN_ID", "BUILD_NUMBER"],
    }
    
    def __init__(self, additional_packages: Optional[List[str]] = None):
        """
        Initialize collector with optional additional packages to track
        
        Args:
            additional_packages: Additional package names to track versions for
        """
        self.packages_to_track = self.CORE_PACKAGES.copy()
        if additional_packages:
            self.packages_to_track.extend(additional_packages)
        
        self._cached_versions = None
        self._cached_build_info = None
    
    def collect_software_info(self) -> Dict[str, Any]:
        """
        Collect complete software information
        
        Returns:
            Dict containing software and build information
        """
        debug_log("[Software] Collecting software information...")
        
        software_info = {
            "packages": self.get_package_versions(),
        }
        
        # Add ROCm version if available (from environment or package)
        rocm_version = self._get_rocm_package_version()
        if rocm_version:
            software_info["rocm_version"] = rocm_version
        
        debug_log(f"[Software] Collected {len(software_info['packages'])} package versions")
        return software_info
    
    def collect_build_info(self) -> Optional[Dict[str, Any]]:
        """
        Collect build information from environment variables
        
        Returns:
            Dict containing build information, or None if no info available
        """
        debug_log("[Software] Collecting build information...")
        
        if self._cached_build_info is not None:
            return self._cached_build_info
        
        build_info = {}
        
        for field, env_vars in self.BUILD_ENV_MAPPING.items():
            for env_var in env_vars:
                value = os.getenv(env_var)
                if value:
                    build_info[field] = value
                    debug_log(f"[Software] Build info {field} from {env_var}: {value}")
                    break
        
        if build_info:
            self._cached_build_info = build_info
            return build_info
        
        debug_log("[Software] No build information found in environment")
        return None
    
    def get_package_versions(self) -> Dict[str, str]:
        """
        Get versions of tracked packages
        
        Returns:
            Dict mapping package name to version string
        """
        if self._cached_versions is not None:
            return self._cached_versions
        
        versions = {}
        
        for pkg in self.packages_to_track:
            version = self._get_package_version(pkg)
            if version:
                # Normalize package name (replace hyphens with underscores)
                normalized_name = pkg.replace("-", "_")
                versions[normalized_name] = version
        
        self._cached_versions = versions
        debug_log(f"[Software] Found {len(versions)} installed packages")
        return versions
    
    def _get_package_version(self, package_name: str) -> Optional[str]:
        """
        Get version of a single package
        
        Args:
            package_name: Name of the package
            
        Returns:
            Version string or None if not installed
        """
        # Method 1: importlib.metadata (preferred)
        try:
            return importlib.metadata.version(package_name)
        except importlib.metadata.PackageNotFoundError:
            pass
        
        # Method 2: Try with underscore variant
        try:
            underscore_name = package_name.replace("-", "_")
            if underscore_name != package_name:
                return importlib.metadata.version(underscore_name)
        except importlib.metadata.PackageNotFoundError:
            pass
        
        # Method 3: Try with hyphen variant
        try:
            hyphen_name = package_name.replace("_", "-")
            if hyphen_name != package_name:
                return importlib.metadata.version(hyphen_name)
        except importlib.metadata.PackageNotFoundError:
            pass
        
        # Method 4: Direct import for special packages
        special_packages = {
            "torch": lambda: __import__("torch").__version__,
            "tensorflow": lambda: __import__("tensorflow").__version__,
            "jax": lambda: __import__("jax").__version__,
            "numpy": lambda: __import__("numpy").__version__,
            "transformers": lambda: __import__("transformers").__version__,
            "deepspeed": lambda: __import__("deepspeed").__version__,
        }
        
        if package_name in special_packages:
            try:
                return special_packages[package_name]()
            except (ImportError, AttributeError):
                pass
        
        return None
    
    def _get_rocm_package_version(self) -> Optional[str]:
        """Get ROCm version from package or environment"""
        # Check environment first
        if os.getenv("ROCM_VERSION"):
            return os.getenv("ROCM_VERSION")
        
        # Check PyTorch HIP version
        try:
            import torch
            if hasattr(torch.version, 'hip') and torch.version.hip:
                # Extract major.minor from hip version
                hip_parts = torch.version.hip.split('.')
                if len(hip_parts) >= 2:
                    return f"{hip_parts[0]}.{hip_parts[1].split('-')[0]}"
        except ImportError:
            pass
        except Exception:
            pass
        
        return None
    
    def get_framework_versions(self) -> Dict[str, str]:
        """
        Get versions of main deep learning frameworks only
        
        Returns:
            Dict with framework versions
        """
        framework_packages = [
            "torch", "tensorflow", "jax", "transformers",
            "deepspeed", "accelerate", "pytorch-lightning", "lightning"
        ]
        
        versions = {}
        for pkg in framework_packages:
            version = self._get_package_version(pkg)
            if version:
                normalized_name = pkg.replace("-", "_")
                versions[normalized_name] = version
        
        return versions
    
    def get_full_pip_freeze(self) -> Dict[str, str]:
        """
        Get all installed package versions (equivalent to pip freeze)
        
        Returns:
            Dict with all package versions
        """
        debug_log("[Software] Getting full pip freeze...")
        
        try:
            all_packages = {}
            for dist in importlib.metadata.distributions():
                name = dist.metadata.get('Name', '')
                version = dist.metadata.get('Version', '')
                if name and version:
                    all_packages[name.lower().replace("-", "_")] = version
            
            debug_log(f"[Software] Found {len(all_packages)} total packages")
            return all_packages
        except Exception as e:
            warning_log(f"[Software] Failed to get pip freeze: {e}")
            return {}


# Convenience functions for quick access
def get_package_versions() -> Dict[str, str]:
    """Quick function to get package versions"""
    collector = SoftwareInfoCollector()
    return collector.get_package_versions()


def get_build_info() -> Optional[Dict[str, Any]]:
    """Quick function to get build information"""
    collector = SoftwareInfoCollector()
    return collector.collect_build_info()

