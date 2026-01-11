# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""
Hardware Information Collector - Collect GPU architecture and hardware information
"""
import os
import subprocess
from typing import Dict, Any, Optional

from .logger import debug_log, warning_log


class HardwareInfoCollector:
    """Hardware Information Collector"""
    
    # GPU architecture mapping (product name -> arch code)
    GPU_ARCH_MAP = {
        "MI325": "mi325",
        "MI350": "mi350", 
        "MI300": "mi300",
        "MI300X": "mi300x",
        "MI300A": "mi300a",
        "MI250": "mi250",
        "MI250X": "mi250x",
        "MI210": "mi210",
        "MI100": "mi100",
        "MI50": "mi50",
        "MI60": "mi60",
        # NVIDIA GPUs
        "A100": "a100",
        "H100": "h100",
        "H200": "h200",
        "V100": "v100",
        "RTX 4090": "rtx4090",
        "RTX 3090": "rtx3090",
    }
    
    def __init__(self):
        self._cached_gpu_arch = None
        self._cached_rocm_version = None
        self._cached_gpu_info = None
    
    def collect_hardware_info(self) -> Dict[str, Any]:
        """
        Collect complete hardware information
        
        Returns:
            Dict containing hardware information
        """
        debug_log("[Hardware] Collecting hardware information...")
        
        hardware_info = {
            "gpu_arch": self.get_gpu_arch(),
            "gpu_count": self.get_gpu_count(),
            "gpu_memory_gb": self.get_gpu_memory(),
            "gpu_name": self.get_gpu_name(),
        }
        
        # Add ROCm version if available
        rocm_version = self.get_rocm_version()
        if rocm_version:
            hardware_info["rocm_version"] = rocm_version
        
        # Add CUDA version if available
        cuda_version = self.get_cuda_version()
        if cuda_version:
            hardware_info["cuda_version"] = cuda_version
        
        debug_log(f"[Hardware] Collected: {hardware_info}")
        return hardware_info
    
    def get_gpu_arch(self) -> Optional[str]:
        """
        Get GPU architecture
        
        Returns:
            GPU architecture string (e.g., 'mi325', 'mi350', 'a100')
        """
        if self._cached_gpu_arch is not None:
            return self._cached_gpu_arch
        
        # Method 1: Environment variables
        env_keys = ["GPU_ARCH", "AMD_GPU_ARCH", "NVIDIA_GPU_ARCH"]
        for key in env_keys:
            if os.getenv(key):
                self._cached_gpu_arch = os.getenv(key).lower()
                debug_log(f"[Hardware] GPU arch from env {key}: {self._cached_gpu_arch}")
                return self._cached_gpu_arch
        
        # Method 2: PyTorch device name
        try:
            import torch
            if torch.cuda.is_available():
                device_name = torch.cuda.get_device_name(0)
                arch = self._parse_gpu_arch(device_name)
                if arch:
                    self._cached_gpu_arch = arch
                    debug_log(f"[Hardware] GPU arch from PyTorch: {arch} (device: {device_name})")
                    return arch
        except ImportError:
            pass
        except Exception as e:
            warning_log(f"[Hardware] Failed to get GPU arch from PyTorch: {e}")
        
        # Method 3: rocm-smi
        arch = self._get_gpu_arch_from_rocm_smi()
        if arch:
            self._cached_gpu_arch = arch
            return arch
        
        # Method 4: nvidia-smi
        arch = self._get_gpu_arch_from_nvidia_smi()
        if arch:
            self._cached_gpu_arch = arch
            return arch
        
        return None
    
    def _parse_gpu_arch(self, device_name: str) -> Optional[str]:
        """Parse GPU architecture from device name"""
        if not device_name:
            return None
        
        device_name_upper = device_name.upper()
        for product, arch in self.GPU_ARCH_MAP.items():
            if product.upper() in device_name_upper:
                return arch
        
        return None
    
    def _get_gpu_arch_from_rocm_smi(self) -> Optional[str]:
        """Get GPU architecture from rocm-smi"""
        try:
            result = subprocess.run(
                ["rocm-smi", "--showproductname"],
                capture_output=True,
                text=True,
                timeout=10
            )
            if result.returncode == 0:
                for line in result.stdout.split('\n'):
                    arch = self._parse_gpu_arch(line)
                    if arch:
                        debug_log(f"[Hardware] GPU arch from rocm-smi: {arch}")
                        return arch
        except FileNotFoundError:
            debug_log("[Hardware] rocm-smi not found")
        except subprocess.TimeoutExpired:
            warning_log("[Hardware] rocm-smi timeout")
        except Exception as e:
            warning_log(f"[Hardware] rocm-smi error: {e}")
        
        return None
    
    def _get_gpu_arch_from_nvidia_smi(self) -> Optional[str]:
        """Get GPU architecture from nvidia-smi"""
        try:
            result = subprocess.run(
                ["nvidia-smi", "--query-gpu=name", "--format=csv,noheader"],
                capture_output=True,
                text=True,
                timeout=10
            )
            if result.returncode == 0:
                for line in result.stdout.strip().split('\n'):
                    arch = self._parse_gpu_arch(line)
                    if arch:
                        debug_log(f"[Hardware] GPU arch from nvidia-smi: {arch}")
                        return arch
        except FileNotFoundError:
            debug_log("[Hardware] nvidia-smi not found")
        except subprocess.TimeoutExpired:
            warning_log("[Hardware] nvidia-smi timeout")
        except Exception as e:
            warning_log(f"[Hardware] nvidia-smi error: {e}")
        
        return None
    
    def get_rocm_version(self) -> Optional[str]:
        """
        Get ROCm version
        
        Returns:
            ROCm version string (e.g., 'rocm7.1', 'rocm6.4')
        """
        if self._cached_rocm_version is not None:
            return self._cached_rocm_version
        
        # Method 1: Environment variable
        if os.getenv("ROCM_VERSION"):
            version = os.getenv("ROCM_VERSION")
            self._cached_rocm_version = f"rocm{version}"
            debug_log(f"[Hardware] ROCm version from env: {self._cached_rocm_version}")
            return self._cached_rocm_version
        
        # Method 2: PyTorch HIP version
        try:
            import torch
            if hasattr(torch.version, 'hip') and torch.version.hip:
                # torch.version.hip format: "6.2.41133-d2f89a95"
                hip_parts = torch.version.hip.split('.')
                if len(hip_parts) >= 2:
                    major_minor = f"{hip_parts[0]}.{hip_parts[1].split('-')[0]}"
                    self._cached_rocm_version = f"rocm{major_minor}"
                    debug_log(f"[Hardware] ROCm version from PyTorch HIP: {self._cached_rocm_version}")
                    return self._cached_rocm_version
        except ImportError:
            pass
        except Exception as e:
            warning_log(f"[Hardware] Failed to get ROCm version from PyTorch: {e}")
        
        # Method 3: Read version file
        version_files = [
            "/opt/rocm/.info/version",
            "/opt/rocm/version.txt",
            "/opt/rocm/.version",
        ]
        for version_file in version_files:
            if os.path.exists(version_file):
                try:
                    with open(version_file, 'r') as f:
                        version = f.read().strip().split('-')[0]
                        self._cached_rocm_version = f"rocm{version}"
                        debug_log(f"[Hardware] ROCm version from file: {self._cached_rocm_version}")
                        return self._cached_rocm_version
                except Exception as e:
                    warning_log(f"[Hardware] Failed to read {version_file}: {e}")
        
        # Method 4: rocminfo
        try:
            result = subprocess.run(
                ["rocminfo"],
                capture_output=True,
                text=True,
                timeout=10
            )
            if result.returncode == 0:
                for line in result.stdout.split('\n'):
                    if 'Runtime Version' in line or 'ROCm' in line:
                        # Try to extract version number
                        import re
                        match = re.search(r'(\d+\.\d+)', line)
                        if match:
                            version = match.group(1)
                            self._cached_rocm_version = f"rocm{version}"
                            debug_log(f"[Hardware] ROCm version from rocminfo: {self._cached_rocm_version}")
                            return self._cached_rocm_version
        except FileNotFoundError:
            debug_log("[Hardware] rocminfo not found")
        except Exception as e:
            warning_log(f"[Hardware] rocminfo error: {e}")
        
        return None
    
    def get_cuda_version(self) -> Optional[str]:
        """
        Get CUDA version
        
        Returns:
            CUDA version string (e.g., 'cuda12.1')
        """
        # Method 1: Environment variable
        if os.getenv("CUDA_VERSION"):
            return f"cuda{os.getenv('CUDA_VERSION')}"
        
        # Method 2: PyTorch CUDA version
        try:
            import torch
            if torch.cuda.is_available() and torch.version.cuda:
                return f"cuda{torch.version.cuda}"
        except ImportError:
            pass
        except Exception:
            pass
        
        return None
    
    def get_gpu_count(self) -> int:
        """Get number of GPUs"""
        try:
            import torch
            if torch.cuda.is_available():
                return torch.cuda.device_count()
        except ImportError:
            pass
        except Exception:
            pass
        
        # Fallback: try environment variable
        if os.getenv("WORLD_SIZE"):
            try:
                return int(os.getenv("WORLD_SIZE"))
            except ValueError:
                pass
        
        return 0
    
    def get_gpu_memory(self) -> Optional[float]:
        """Get total GPU memory in GB (for first GPU)"""
        try:
            import torch
            if torch.cuda.is_available():
                total_memory = torch.cuda.get_device_properties(0).total_memory
                return round(total_memory / (1024 ** 3), 1)
        except ImportError:
            pass
        except Exception:
            pass
        
        return None
    
    def get_gpu_name(self) -> Optional[str]:
        """Get GPU device name"""
        try:
            import torch
            if torch.cuda.is_available():
                return torch.cuda.get_device_name(0)
        except ImportError:
            pass
        except Exception:
            pass
        
        return None

