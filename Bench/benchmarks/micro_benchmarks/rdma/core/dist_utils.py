import os
import socket
import torch
import torch.distributed as dist
from datetime import timedelta
from .config import config
from .logger import get_logger
from .ib import gather_ib_info

logger = get_logger("dist_utils")

# Global storage for all rank information
_rank_info = None
_ib_info = None

def init_distributed():
    """Initialize torch.distributed and collect hostname/device for all ranks"""
    global _rank_info

    if dist.is_initialized():
        logger.info("torch.distributed is already initialized")
        return

    logger.info(
        f"Initializing distributed: backend={config.backend}, "
        f"init_method={config.init_method}, timeout={config.timeout}s"
    )

    dist.init_process_group(
        backend=config.backend,
        init_method=config.init_method,
        timeout=timedelta(seconds=config.timeout),
    )
    rank = dist.get_rank()
    hostname = socket.gethostname()
    logger.info(
        f"[{hostname}][Rank {rank}]Gathering rank info"
    )
    # Initialize collecting hostname and devices
    world_size = dist.get_world_size()
     # Set device
    if torch.cuda.is_available():
        if "LOCAL_RANK" in os.environ:
            local_rank = int(os.environ["LOCAL_RANK"])
        else:
            local_rank = rank % torch.cuda.device_count()
        torch.cuda.set_device(local_rank)
    info = {
    "rank": rank,  # global rank
    "local_rank": local_rank,  # GPU rank within this node
    "hostname": hostname,
    "device": local_rank if torch.cuda.is_available() else None
    }

    gathered = [None for _ in range(world_size)]
    dist.all_gather_object(gathered, info)
    dist.barrier()
    
    _rank_info = gathered
    _ib_info = gather_ib_info()
    logger.info(f"Collected hostname/device info for all ranks")

def get_rank_info():
    return _rank_info

def get_ib_info():
    return _ib_info

def get_hostname(r: int):
    """Return the hostname for rank r"""
    global _rank_info
    if _rank_info is None:
        raise RuntimeError("Distributed not initialized or rank info not gathered")
    return _rank_info[r]["hostname"]


def get_devices(r: int):
    """Return the GPU device list for rank r"""
    global _rank_info
    if _rank_info is None:
        raise RuntimeError("Distributed not initialized or rank info not gathered")
    return _rank_info[r]["devices"]


def cleanup_dist():
    """Destroy process group"""
    global _rank_info
    if dist.is_initialized():
        dist.destroy_process_group()
        logger.info("torch.distributed destroyed")
    _rank_info = None


def get_rank():
    """Return current rank, return 0 if not initialized"""
    if dist.is_initialized():
        return dist.get_rank()
    return 0


def get_world_size():
    """Return world_size, return 1 if not initialized"""
    if dist.is_initialized():
        return dist.get_world_size()
    return 1


def barrier():
    """Global barrier, does not block when not initialized"""
    if dist.is_initialized():
        dist.barrier()
