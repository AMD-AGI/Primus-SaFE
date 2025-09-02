#!/usr/bin/env python3

#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""
SSH key and config synchronization using torch.distributed.
MODIFIED: All ranks gather data and attempt to write files.
This is generally not recommended due to redundancy/conflicts,
but implemented as requested.
"""

import argparse
import os
import sys
import socket
import logging
import shutil
import fcntl
import struct
from datetime import timedelta
import torch.distributed as dist

# ==================== 配置 ====================
DEFAULT_INTERFACE = "ens51f0"  # for getting local ip
SSH_DIR = "/root/.ssh"
KEY_FILE = os.path.join(SSH_DIR, "id_rsa.pub")
AUTHORIZED_KEYS = os.path.join(SSH_DIR, "authorized_keys")
SSH_CONFIG = os.path.join(SSH_DIR, "config")
BACKUP_SUFFIX = ".backup"

LOG_LEVEL = logging.INFO
# =============================================


def setup_logging():
    logging.basicConfig(
        level=LOG_LEVEL,
        format="[%(asctime)s] %(levelname)s [%(funcName)s] [Rank %(rank)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )


def parse_args():
    parser = argparse.ArgumentParser(description="SSH Sync Tool - All Ranks Write")
    parser.add_argument(
        "--distributed-backend",
        default="gloo",
        choices=["nccl", "gloo", "ccl"],
        help="Distributed backend (use 'gloo' for CPU)",
    )
    parser.add_argument(
        "--local-rank",
        type=int,
        default=0,
        help="Local rank (set by torchrun)",
    )
    parser.add_argument(
        "--distributed-timeout-minutes",
        type=int,
        default=30,
        help="Timeout for distributed operations",
    )
    parser.add_argument(
        "--no-data-sync",
        action='store_true',
        help="If set, skip sync and just barrier",
    )
    parser.add_argument(
        "--interface",
        type=str,
        default=DEFAULT_INTERFACE,
        help=f"Network interface to get IP (default: {DEFAULT_INTERFACE})",
    )
    args = parser.parse_args()

    args.rank = int(os.getenv("RANK", "0"))
    args.world_size = int(os.getenv("WORLD_SIZE", "1"))
    if args.world_size < 1:
        raise ValueError("WORLD_SIZE must be >= 1")

    # Inject rank into logging format for this process
    old_factory = logging.getLogRecordFactory()
    def record_factory(*args, **kwargs):
        record = old_factory(*args, **kwargs)
        record.rank = args[0].split('Rank ')[-1].split(']')[0] if 'Rank ' in args[0] else args[0]
        return record
    logging.setLogRecordFactory(record_factory)

    return args


def get_ip_by_interface(interface: str) -> str:
    """Get IP address by interface name on Linux using ioctl."""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            ifreq = struct.pack('16sH14s', interface.encode('utf-8'), socket.AF_INET, b'\x00'*14)
            try:
                ret = fcntl.ioctl(s.fileno(), 0x8915, ifreq)  # SIOCGIFADDR
            except OSError as e:
                raise ValueError(f"Interface '{interface}' may not exist or have IP: {e}")
            ip = struct.unpack('16sH2x4s8x', ret)[2]
            return socket.inet_ntoa(ip)
    except Exception as e:
        logging.error(f"Failed to get IP via interface {interface}: {e}")
        raise

def read_file_safe(filepath: str) -> str:
    """Read file with error handling."""
    try:
        with open(filepath, "r") as f:
            content = f.read().strip()
        if not content:
            raise ValueError(f"{filepath} is empty")
        return content
    except Exception as e:
        logging.error(f"Failed to read {filepath}: {e}")
        raise

def check_ssh_dir_permissions(ssh_dir: str):
    """Check permissions of .ssh directory."""
    try:
        stat_info = os.stat(ssh_dir)
        mode = stat_info.st_mode
        permissions = oct(mode)[-3:]
        logging.info(f"Permissions for {ssh_dir}: {permissions}")
        # Note: 700 is strictest, 755 is sometimes acceptable
        if permissions not in ['700', '755']:
            logging.warning(f"Permissions for {ssh_dir} are {permissions}, recommended is 700 or 755.")
    except Exception as e:
        logging.error(f"Failed to check permissions for {ssh_dir}: {e}")


def backup_file(filepath: str):
    """Create backup if file exists."""
    if os.path.exists(filepath):
        backup_path = filepath + BACKUP_SUFFIX
        shutil.copy2(filepath, backup_path)
        logging.info(f"Backed up {filepath} -> {backup_path}")


def write_authorized_keys(rank: int, public_keys: list):
    """MODIFIED: All ranks attempt to write the authorized_keys file."""

    logging.info(f"Rank {rank} is attempting to write authorized_keys with {len(public_keys)} keys.")
    try:
        backup_file(AUTHORIZED_KEYS)
        # Acquire an exclusive lock before writing
        with open(AUTHORIZED_KEYS, "w") as f:
            lock_fd = f.fileno()
            fcntl.flock(lock_fd, fcntl.LOCK_EX)
            logging.debug(f"Rank {rank} acquired exclusive lock on {AUTHORIZED_KEYS}")
            for key in public_keys:
                f.write(key.strip() + "\n")
            f.flush() # Ensure data is written before releasing lock
            os.fsync(lock_fd)
            fcntl.flock(lock_fd, fcntl.LOCK_UN)
            logging.debug(f"Rank {rank} released lock on {AUTHORIZED_KEYS}")
        os.chmod(AUTHORIZED_KEYS, 0o600)
        logging.info(f"Rank {rank} successfully wrote {len(public_keys)} keys to {AUTHORIZED_KEYS}")
    except Exception as e:
        logging.error(f"Rank {rank} failed to write {AUTHORIZED_KEYS}: {e}")
        # Depending on requirements, you might want to raise or just log
        # raise # Re-raise if you want the process to fail on write error


def write_ssh_config(rank: int, node_info_list: list):
    """MODIFIED: All ranks attempt to write the SSH config file."""

    logging.info(f"Rank {rank} is attempting to write SSH config for {len(node_info_list)} nodes.")
    template = """
Host {ip}
  HostName {ip}
  User root
  Port {port}
  StrictHostKeyChecking no
  UserKnownHostsFile=/dev/null
  LogLevel QUIET
"""

    try:
        backup_file(SSH_CONFIG)
        # Acquire an exclusive lock before writing
        with open(SSH_CONFIG, "w") as f:
            lock_fd = f.fileno()
            fcntl.flock(lock_fd, fcntl.LOCK_EX)
            logging.debug(f"Rank {rank} acquired exclusive lock on {SSH_CONFIG}")
            for ip, port in node_info_list:
                f.write(template.format(ip=ip.strip(), port=port.strip()))
            f.flush()
            os.fsync(lock_fd)
            fcntl.flock(lock_fd, fcntl.LOCK_UN)
            logging.debug(f"Rank {rank} released lock on {SSH_CONFIG}")
        os.chmod(SSH_CONFIG, 0o600)
        logging.info(f"Rank {rank} successfully wrote {len(node_info_list)} hosts to {SSH_CONFIG}")
    except Exception as e:
        logging.error(f"Rank {rank} failed to write {SSH_CONFIG}: {e}")
        # Depending on requirements, you might want to raise or just log
        # raise


def init_distributed(args):
    """Initialize distributed process group."""
    logging.info(f"Initializing distributed: rank={args.rank}, world_size={args.world_size}")
    if dist.is_initialized():
        logging.warning("Distributed already initialized.")
        return

    dist.init_process_group(
        backend=args.distributed_backend,
        init_method="env://",
        world_size=args.world_size,
        rank=args.rank,
        timeout=timedelta(minutes=args.distributed_timeout_minutes),
    )
    logging.info("Distributed initialized.")


def sync_ssh_data(args):
    """Main logic: gather SSH keys and IP:Port info, ALL ranks write files."""
    init_distributed(args)

    rank = dist.get_rank()
    world_size = dist.get_world_size()
    logging.info(f"Node process started. Rank: {rank}, World Size: {world_size}")

    check_ssh_dir_permissions(SSH_DIR)

    # === Step 1: Gather SSH public keys ===
    my_key = None
    error_flag = False

    try:
        my_key = read_file_safe(KEY_FILE)
        logging.info(f"Rank {rank} read local public key (preview): {my_key[:50]}...")
    except Exception as e:
        logging.error(f"Rank {rank} failed to read SSH key: {e}")
        error_flag = True

    # Gather all keys: use None for failed ranks
    gathered_keys = [None] * world_size
    try:
        dist.all_gather_object(gathered_keys, my_key if not error_flag else f"ERROR_RANK_{rank}")
    except Exception as e:
        logging.error(f"Rank {rank} failed in all_gather_object for keys: {e}")
        # Still try to proceed
        pass

    # Check if any rank failed
    if any("ERROR_RANK_" in (k if isinstance(k, str) else "") for k in gathered_keys if k):
        logging.critical(f"Rank {rank} detected error in key gathering. Aborting.")
        # Don't write file if any key is missing
    else:
        write_authorized_keys(rank, gathered_keys)

    # === Step 2: Gather IP and SSH port ===
    try:
        my_ip = get_ip_by_interface(args.interface)
        my_port = os.getenv("SSH_PORT", "22")
        my_node_info = (my_ip, my_port)
        logging.info(f"Rank {rank} determined local IP: {my_ip}, Port: {my_port}")
    except Exception as e:
        logging.error(f"Rank {rank} failed to get IP or port: {e}")
        my_node_info = ("ERROR_IP", "ERROR_PORT")

    gathered_nodes = [None] * world_size
    try:
        dist.all_gather_object(gathered_nodes, my_node_info)
    except Exception as e:
        logging.error(f"Rank {rank} failed in all_gather_object for node info: {e}")
        pass

    if any(ip == "ERROR_IP" for ip, port in gathered_nodes):
        logging.critical(f"Rank {rank} detected error in node info gathering.")
    else:
        write_ssh_config(rank, gathered_nodes)

    logging.info(f"Rank {rank} waiting at barrier...")
    try:
        dist.barrier()
        logging.info(f"Rank {rank} passed barrier.")
    except Exception as e:
        logging.warning(f"Rank {rank} failed at barrier: {e}")

    logging.info(f"✅ SSH synchronization process completed on Rank {rank}.")

def sync_no_data(args):
    """Just initialize and barrier."""
    init_distributed(args)
    logging.info("Barrier sync only (no data sync).")

    try:
        dist.barrier()
        logging.info(f"✅ Barrier synchronization process completed on Rank {dist.get_rank()}.")
    except Exception as e:
        logging.warning(f"Rank {dist.get_rank()} failed at barrier: {e}")

def main():
    args = parse_args()
    setup_logging()
    logging.info(f"Starting script execution.")

    success = True
    try:
        if not args.no_data_sync:
            sync_ssh_data(args)
        else:
            sync_no_data(args)
    except Exception as e:
        logging.critical(f"Rank {dist.get_rank() if dist.is_initialized() else 'unknown'} got uncaught exception: {e}", exc_info=True)
        success = False
    finally:
        # ✅ all rank should to be destroy
        if dist.is_initialized():
            try:
                try:
                    dist.barrier(timeout=timedelta(seconds=30))
                except Exception:
                    pass
                dist.destroy_process_group()
                logging.info("Distributed process group destroyed.")
            except Exception as e:
                logging.warning(f"Error destroying process group: {e}")

    sys.exit(0 if success else 1)

if __name__ == "__main__":
    main()
