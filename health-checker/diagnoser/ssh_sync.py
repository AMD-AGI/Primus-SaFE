#!/usr/bin/env python3

#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""
SSH Key and Config Synchronization using torch.distributed.

All ranks:
  1. Read their own SSH public key
  2. Gather all public keys and IP:Port info from all ranks
  3. Write the combined public keys to their local ~/.ssh/authorized_keys
  4. Write SSH config for all nodes

This enables all-to-all passwordless SSH between nodes.
"""

import argparse
import os
import sys
import socket
import logging
import shutil
import struct
from datetime import timedelta
import torch.distributed as dist


# ==================== Configuration ====================
DEFAULT_INTERFACE = "eth0"  # Network interface name used to get local IP
SSH_DIR = "/root/.ssh"
KEY_FILE = os.path.join(SSH_DIR, "id_rsa.pub")
AUTHORIZED_KEYS = os.path.join(SSH_DIR, "authorized_keys")
SSH_CONFIG = os.path.join(SSH_DIR, "config")
BACKUP_SUFFIX = ".backup"
LOG_LEVEL = logging.INFO
# ======================================================


def setup_logging():
    """Set up logging with rank information in the format."""
    logging.basicConfig(
        level=LOG_LEVEL,
        format="[%(asctime)s] %(levelname)s [%(funcName)s] [Rank %(rank)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )


def parse_args():
    """Parse command line arguments."""
    parser = argparse.ArgumentParser(description="SSH Sync Tool - All Ranks Write (No Locks)")
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

    # Inject rank into log record
    old_factory = logging.getLogRecordFactory()
    def record_factory(*args, **kwargs):
        record = old_factory(*args, **kwargs)
        record.rank = str(args.rank) if hasattr(args, 'rank') else 'unknown'
        return record
    logging.setLogRecordFactory(record_factory)

    return args


def get_ip_by_interface(interface: str) -> str:
    """Get local IP address by interface name (Linux only)"""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            ifreq = struct.pack('16sH14s', interface.encode('utf-8'), socket.AF_INET, b'\x00'*14)
            ret = fcntl.ioctl(s.fileno(), 0x8915, ifreq)  # SIOCGIFADDR
            ip = struct.unpack('16sH2x4s8x', ret)[2]
            return socket.inet_ntoa(ip)
    except Exception as e:
        logging.error(f"Failed to get IP via interface {interface}: {e}")
        raise


def read_file_safe(filepath: str) -> str:
    """Safely read a file."""
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
    """Check permissions of the .ssh directory."""
    try:
        stat_info = os.stat(ssh_dir)
        mode = stat_info.st_mode
        permissions = oct(mode)[-3:]
        logging.info(f"Permissions for {ssh_dir}: {permissions}")
        if permissions not in ['700', '755']:
            logging.warning(f"Permissions for {ssh_dir} are {permissions}, recommended is 700 or 755.")
    except Exception as e:
        logging.error(f"Failed to check permissions for {ssh_dir}: {e}")


def backup_file(filepath: str):
    """Backup file if it exists."""
    if os.path.exists(filepath):
        backup_path = filepath + BACKUP_SUFFIX
        shutil.copy2(filepath, backup_path)
        logging.info(f"Backed up {filepath} -> {backup_path}")


def write_authorized_keys(rank: int, public_keys: list):
    """All ranks write all public keys to their local authorized_keys"""
    unique_keys = list(set(key.strip() for key in public_keys if key.strip()))
    logging.info(f"Rank {rank} writing {len(unique_keys)} unique keys to {AUTHORIZED_KEYS}")

    try:
        backup_file(AUTHORIZED_KEYS)
        with open(AUTHORIZED_KEYS, "w") as f:
            for key in unique_keys:
                f.write(key + "\n")
        os.chmod(AUTHORIZED_KEYS, 0o600)
        logging.info(f"Rank {rank} successfully updated {AUTHORIZED_KEYS}")
    except Exception as e:
        logging.error(f"Rank {rank} failed to write {AUTHORIZED_KEYS}: {e}")
        raise


def write_ssh_config(rank: int, node_info_list: list):
    """All ranks generate SSH config containing all nodes"""
    logging.info(f"Rank {rank} writing SSH config for {len(node_info_list)} nodes")
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
        with open(SSH_CONFIG, "w") as f:
            for ip, port in node_info_list:
                f.write(template.format(ip=ip.strip(), port=port.strip()))
        os.chmod(SSH_CONFIG, 0o600)
        logging.info(f"Rank {rank} successfully updated {SSH_CONFIG}")
    except Exception as e:
        logging.error(f"Rank {rank} failed to write {SSH_CONFIG}: {e}")
        raise


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
    """Main synchronization logic"""
    init_distributed(args)
    rank = dist.get_rank()
    world_size = dist.get_world_size()
    logging.info(f"Node process started. Rank: {rank}, World Size: {world_size}")

    check_ssh_dir_permissions(SSH_DIR)

    # === Step 1: Gather SSH public keys from all nodes ===
    my_key = None
    try:
        my_key = read_file_safe(KEY_FILE)
        logging.info(f"Rank {rank} read local public key (preview): {my_key[:50]}...")
    except Exception as e:
        logging.error(f"Rank {rank} failed to read SSH key: {e}")
        my_key = f"ERROR_KEY_RANK_{rank}"

    gathered_keys = [None] * world_size
    try:
        dist.all_gather_object(gathered_keys, my_key)
    except Exception as e:
        logging.error(f"Rank {rank} failed in all_gather_object for keys: {e}")
        raise

    if any("ERROR_KEY_RANK_" in (k if isinstance(k, str) else "") for k in gathered_keys):
        logging.critical(f"Rank {rank} detected key read error. Aborting.")
        raise RuntimeError("SSH key collection failed")
    else:
        write_authorized_keys(rank, gathered_keys)

    # === Step 2: Gather IP and SSH port from all nodes ===
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
        raise

    if any(ip == "ERROR_IP" for ip, port in gathered_nodes):
        logging.critical(f"Rank {rank} detected IP collection error.")
        raise RuntimeError("IP collection failed")
    else:
        write_ssh_config(rank, gathered_nodes)

    # === Synchronize all nodes before exit ===
    logging.info(f"Rank {rank} waiting at barrier...")
    try:
        dist.barrier()
        logging.info(f"Rank {rank} passed barrier.")
    except Exception as e:
        logging.warning(f"Rank {rank} failed at barrier: {e}")

    logging.info(f"✅ SSH synchronization completed on Rank {rank}.")


def main():
    args = parse_args()
    setup_logging()
    logging.info(f"Starting SSH sync script.")

    success = True
    try:
        sync_ssh_data(args)
    except Exception as e:
        logging.critical(f"Rank {dist.get_rank() if dist.is_initialized() else 'unknown'} got uncaught exception: {e}", exc_info=True)
        success = False
    finally:
        if dist.is_initialized():
            try:
                dist.barrier(timeout=timedelta(seconds=30))
                dist.destroy_process_group()
                logging.info("Distributed process group destroyed.")
            except Exception as e:
                logging.warning(f"Error destroying process group: {e}")

    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()