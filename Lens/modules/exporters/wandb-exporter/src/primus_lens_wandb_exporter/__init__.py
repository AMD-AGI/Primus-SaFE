"""
Primus Lens WandB Exporter
Automatically intercepts wandb reporting without requiring user code changes
"""

__version__ = "0.2.0"
__author__ = "Primus Team"

# Automatically install hook (triggered via .pth file)
# This module will be automatically imported when Python starts

# Export collectors for external use
from .data_collector import DataCollector
from .hardware_info import HardwareInfoCollector
from .software_info import SoftwareInfoCollector, get_package_versions, get_build_info

__all__ = [
    "DataCollector",
    "HardwareInfoCollector", 
    "SoftwareInfoCollector",
    "get_package_versions",
    "get_build_info",
]

