#!/usr/bin/env python3

#  Copyright (c) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""
SSH Key and Config Synchronization using NFS shared storage.

Replaces the torchrun-based ssh_sync.py to avoid O(N) TCPStore initialization
overhead at scale. Each rank writes its public key and network info to a shared
NFS directory, then all ranks read the aggregated data once a barrier file
signals completeness.

Directory layout (per workload):
  $SHARE_PATH/ssh_exchange/$WORKLOAD_ID/
    keys/rank_<N>.pub
    info/rank_<N>.json   -> {"ip": "...", "port": "..."}
    barrier              -> created by rank 0 after all files are present
"""

import argparse
import json
import logging
import os
import shutil
import socket
import struct
import sys
import time
import fcntl
from pathlib import Path

SSH_DIR = "/root/.ssh"
KEY_FILE = os.path.join(SSH_DIR, "id_rsa.pub")
AUTHORIZED_KEYS = os.path.join(SSH_DIR, "authorized_keys")
SSH_CONFIG = os.path.join(SSH_DIR, "config")
BACKUP_SUFFIX = ".backup"

POLL_INTERVAL = 0.5
BARRIER_TIMEOUT = 600

logging.basicConfig(
    level=logging.INFO,
    format="[%(asctime)s] %(levelname)s %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
log = logging.getLogger(__name__)


def parse_args():
    parser = argparse.ArgumentParser(description="SSH Sync via NFS shared storage")
    parser.add_argument("--interface", type=str, default="eth0")
    parser.add_argument("--timeout", type=int, default=BARRIER_TIMEOUT,
                        help="Seconds to wait for all ranks before giving up")
    args = parser.parse_args()

    args.rank = int(os.environ["RANK"])
    args.world_size = int(os.environ["WORLD_SIZE"])
    args.ssh_port = os.environ.get("SSH_PORT", "22")
    args.share_path = os.environ["SHARE_PATH"]
    args.workload_id = os.environ["WORKLOAD_ID"]

    args.exchange_dir = os.path.join(
        args.share_path, "ssh_exchange", args.workload_id
    )
    return args


def get_ip_by_interface(interface: str) -> str:
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
        ifreq = struct.pack(
            "16sH14s", interface.encode("utf-8"), socket.AF_INET, b"\x00" * 14
        )
        ret = fcntl.ioctl(s.fileno(), 0x8915, ifreq)
        ip = struct.unpack("16sH2x4s8x", ret)[2]
        return socket.inet_ntoa(ip)


def backup_file(filepath: str):
    if os.path.exists(filepath):
        shutil.copy2(filepath, filepath + BACKUP_SUFFIX)


def publish(args):
    """Write this rank's public key and network info to NFS."""
    keys_dir = os.path.join(args.exchange_dir, "keys")
    info_dir = os.path.join(args.exchange_dir, "info")
    os.makedirs(keys_dir, exist_ok=True)
    os.makedirs(info_dir, exist_ok=True)

    pub_key = Path(KEY_FILE).read_text().strip()
    key_path = os.path.join(keys_dir, f"rank_{args.rank}.pub")
    Path(key_path).write_text(pub_key + "\n")
    log.info("Rank %d wrote public key to %s", args.rank, key_path)

    my_ip = get_ip_by_interface(args.interface)
    info_path = os.path.join(info_dir, f"rank_{args.rank}.json")
    Path(info_path).write_text(json.dumps({"ip": my_ip, "port": args.ssh_port}))
    log.info("Rank %d wrote node info to %s (ip=%s port=%s)",
             args.rank, info_path, my_ip, args.ssh_port)


def wait_and_create_barrier(args):
    """Rank 0: poll until all rank files exist, then create barrier."""
    keys_dir = os.path.join(args.exchange_dir, "keys")
    deadline = time.monotonic() + args.timeout

    while time.monotonic() < deadline:
        existing = list(Path(keys_dir).glob("rank_*.pub"))
        if len(existing) >= args.world_size:
            barrier_path = os.path.join(args.exchange_dir, "barrier")
            Path(barrier_path).write_text(str(args.world_size))
            log.info("Rank 0 created barrier (%d/%d ranks ready)",
                     len(existing), args.world_size)
            return
        time.sleep(POLL_INTERVAL)

    raise TimeoutError(
        f"Rank 0 timed out waiting for {args.world_size} ranks "
        f"(got {len(list(Path(keys_dir).glob('rank_*.pub')))})"
    )


def wait_for_barrier(args):
    """Non-rank-0: poll until barrier file appears."""
    barrier_path = os.path.join(args.exchange_dir, "barrier")
    deadline = time.monotonic() + args.timeout

    while time.monotonic() < deadline:
        if os.path.exists(barrier_path):
            log.info("Rank %d detected barrier", args.rank)
            return
        time.sleep(POLL_INTERVAL)

    raise TimeoutError(f"Rank {args.rank} timed out waiting for barrier")


def assemble(args):
    """Read all keys and info from NFS, write local SSH config."""
    keys_dir = os.path.join(args.exchange_dir, "keys")
    info_dir = os.path.join(args.exchange_dir, "info")

    public_keys = []
    for kf in sorted(Path(keys_dir).glob("rank_*.pub")):
        public_keys.append(kf.read_text().strip())

    node_info = []
    for nf in sorted(Path(info_dir).glob("rank_*.json")):
        data = json.loads(nf.read_text())
        node_info.append((data["ip"], data["port"]))

    backup_file(AUTHORIZED_KEYS)
    unique_keys = list(set(public_keys))
    with open(AUTHORIZED_KEYS, "w") as f:
        for key in unique_keys:
            f.write(key + "\n")
    os.chmod(AUTHORIZED_KEYS, 0o600)
    log.info("Rank %d wrote %d keys to %s", args.rank, len(unique_keys), AUTHORIZED_KEYS)

    backup_file(SSH_CONFIG)
    with open(SSH_CONFIG, "w") as f:
        for ip, port in node_info:
            f.write(f"\nHost {ip}\n"
                    f"  HostName {ip}\n"
                    f"  User root\n"
                    f"  Port {port}\n"
                    f"  StrictHostKeyChecking no\n"
                    f"  UserKnownHostsFile=/dev/null\n"
                    f"  LogLevel QUIET\n")
    os.chmod(SSH_CONFIG, 0o600)
    log.info("Rank %d wrote SSH config for %d nodes", args.rank, len(node_info))


def signal_done(args):
    """Signal that this rank has finished writing SSH config."""
    done_dir = os.path.join(args.exchange_dir, "done")
    os.makedirs(done_dir, exist_ok=True)
    Path(os.path.join(done_dir, f"rank_{args.rank}")).write_text("1")
    log.info("Rank %d signaled assemble done", args.rank)


def wait_all_done(args):
    """Rank 0 waits until all ranks have finished writing SSH config.

    This prevents rank 0 from proceeding (e.g. running ansible) before
    worker nodes have written the master's public key to their
    authorized_keys.
    """
    done_dir = os.path.join(args.exchange_dir, "done")
    deadline = time.monotonic() + args.timeout

    while time.monotonic() < deadline:
        existing = list(Path(done_dir).glob("rank_*"))
        if len(existing) >= args.world_size:
            log.info("Rank 0 confirmed all %d ranks finished assemble",
                     len(existing))
            return
        time.sleep(POLL_INTERVAL)

    existing = list(Path(done_dir).glob("rank_*"))
    raise TimeoutError(
        f"Rank 0 timed out waiting for all ranks to finish assemble "
        f"(got {len(existing)}/{args.world_size})"
    )


def main():
    args = parse_args()
    log.info("Starting NFS-based SSH sync: rank=%d world_size=%d workload=%s",
             args.rank, args.world_size, args.workload_id)

    publish(args)

    if args.rank == 0:
        wait_and_create_barrier(args)
    else:
        wait_for_barrier(args)

    assemble(args)
    signal_done(args)

    if args.rank == 0:
        wait_all_done(args)

    log.info("SSH synchronization completed on rank %d", args.rank)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        log.critical("SSH sync failed: %s", e, exc_info=True)
        sys.exit(1)
