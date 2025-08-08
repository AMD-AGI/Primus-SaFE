#!/usr/bin/env python3

#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""
Robust SSH key and config synchronization using torch.distributed.
Only rank 0 gathers data and writes files.
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
        format="[%(asctime)s] %(levelname)s [%(funcName)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )


def parse_args():
    parser = argparse.ArgumentParser(description="Robust SSH Sync Tool")
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
        type=int,
        default=0,
        help="If non-zero, skip sync and just barrier",
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

    return args


def get_ip_by_interface(interface: str) -> str:
    """Get IP address by interface name on Linux using ioctl."""
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            # SIOCGIFADDR: get interface address
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


def backup_file(filepath: str):
    """Create backup if file exists."""
    if os.path.exists(filepath):
        backup_path = filepath + BACKUP_SUFFIX
        shutil.copy2(filepath, backup_path)
        logging.info(f"Backed up {filepath} -> {backup_path}")


def write_authorized_keys(rank: int, public_keys: list):
    """Only rank 0 writes the authorized_keys file."""
    if rank != 0:
        return

    try:
        backup_file(AUTHORIZED_KEYS)
        with open(AUTHORIZED_KEYS, "w") as f:
            for key in public_keys:
                f.write(key.strip() + "\n")
        os.chmod(AUTHORIZED_KEYS, 0o600)
        logging.info(f"Wrote {len(public_keys)} keys to {AUTHORIZED_KEYS}")
    except Exception as e:
        logging.error(f"Failed to write {AUTHORIZED_KEYS}: {e}")
        raise


def write_ssh_config(rank: int, node_info_list: list):
    """Only rank 0 writes the SSH config file."""
    if rank != 0:
        return

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
        logging.info(f"Wrote {len(node_info_list)} hosts to {SSH_CONFIG}")
    except Exception as e:
        logging.error(f"Failed to write {SSH_CONFIG}: {e}")
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
    """Main logic: gather SSH keys and IP:Port info, only rank 0 writes files."""
    init_distributed(args)

    rank = dist.get_rank()
    world_size = dist.get_world_size()

    # === Step 1: Gather SSH public keys ===
    try:
        my_key = read_file_safe(KEY_FILE)
    except Exception:
        logging.error("SSH key read failed. Exiting.")
        dist.destroy_process_group()
        sys.exit(1)

    # Gather all keys to rank 0
    gathered_keys = [None] * world_size if rank == 0 else None
    dist.gather_object(my_key, gathered_keys, dst=0)
    if rank == 0:
        logging.info(f"Gathered {len(gathered_keys)} SSH public keys.")
        write_authorized_keys(rank, gathered_keys)

    # === Step 2: Gather IP and SSH port ===
    try:
        my_ip = get_ip_by_interface(args.interface)
        my_port = os.getenv("SSH_PORT", "22")
    except Exception:
        logging.error("Failed to get IP or port. Exiting.")
        dist.destroy_process_group()
        sys.exit(1)

    my_node_info = (my_ip, my_port)
    gathered_nodes = [None] * world_size if rank == 0 else None
    dist.gather_object(my_node_info, gathered_nodes, dst=0)
    if rank == 0:
        logging.info(f"Gathered {len(gathered_nodes)} node IP:Port info.")
        write_ssh_config(rank, gathered_nodes)

    # Ensure all nodes finish
    dist.barrier()
    if rank == 0:
        logging.info("✅ SSH synchronization completed successfully.")


def sync_no_data(args):
    """Just initialize and barrier."""
    init_distributed(args)
    logging.info("Barrier sync only (no data sync).")
    dist.barrier()
    if dist.get_rank() == 0:
        logging.info("✅ Barrier synchronization completed.")


def main():
    setup_logging()
    args = parse_args()

    logging.info(f"Starting on rank {args.rank}")

    try:
        if args.no_data_sync == 0:
            sync_ssh_data(args)
        else:
            sync_no_data(args)
    except Exception as e:
        logging.critical(f"Fatal error on rank {args.rank}: {e}", exc_info=True)
        sys.exit(1)
    finally:
        if dist.is_initialized():
            dist.destroy_process_group()


if __name__ == "__main__":
    main()