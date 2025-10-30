import os
import logging
from dataclasses import dataclass


@dataclass
class Config:
    # Distributed settings
    backend: str = os.getenv("RCCL_BACKEND", "nccl")  # or gloo
    init_method: str = os.getenv("RCCL_INIT_METHOD", "env://")
    timeout: int = int(os.getenv("RCCL_TIMEOUT", "60"))  # seconds

    # Logger settings
    log_level: int = int(os.getenv("RCCL_LOG_LEVEL", logging.INFO))
    log_dir: str = os.getenv("RCCL_LOG_DIR", "./logs")
    log_name: str = os.getenv("RCCL_LOG_NAME", "rccl_diag")

    # Output settings
    output_dir: str = os.getenv("PULSKIT_OUTPUT_DIR", "./rsults")

    # Debug
    debug: bool = os.getenv("RCCL_DEBUG", "0") == "1"


# Global configuration instance
config = Config()
