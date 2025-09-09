#!/usr/bin/env python3

#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

import subprocess
import sys
import os
import argparse
import random
import time
from typing import List, Tuple
from queue import Queue, Empty
import threading
import hashlib
from concurrent.futures import ThreadPoolExecutor

# ================= configuration =================
MPIEXEC = "/opt/mpich/bin/mpirun"
RCCL_ALL_REDUCE_PERF = "/opt/rccl-tests/build/all_reduce_perf"
RCCL_ALL_TO_ALL_PERF = "/opt/rccl-tests/build/alltoall_perf"
NUM_GPUS_PER_NODE = 8
MAX_BYTES = "1G"

LD_LIBRARY_PATH = "/opt/rocm/lib:/opt/mpich/lib:/usr/local/lib"
RCCL_SOCKET_IFNAME = "ens51f0"
RCCL_IB_HCA = "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
NCCL_IB_GID_INDEX = 3
RCCL_TEST_TYPE = 0
SSH_PORT = 22

DEBUG_MODE = False
total_nodes = 0
healthy_node_queue: Queue[str] = Queue()
# for log output
print_lock = threading.Lock()
# ===========================================

def log(msg: str):
    with print_lock:
        print(msg)

def get_log_filename(nodes: List[str]) -> str:
    node_str = ",".join(sorted(nodes))
    hash_obj = hashlib.sha256(node_str.encode('utf-8'))
    hash_hex = hash_obj.hexdigest()[:16]
    return f"/tmp/rccl_test_{hash_hex}.log"

def threshold(node_count: int) -> float:
    if RCCL_TEST_TYPE == 0:
        return 300.0
    try:
        bnic = float(os.environ['BNIC'])
        bxgmi = float(os.environ['BXGMI'])
    except (KeyError, ValueError):
        bnic = 50.0
        bxgmi = 315.0
    G_PER_NODE = 8
    # Calculate traffic fractions
    remote_frac = (node_count - 1) / node_count
    local_frac = (G_PER_NODE - 1) / (G_PER_NODE * node_count)
    # Compute effective bandwidth
    beff = 1 / (remote_frac / bnic + local_frac / bxgmi)
    beff *= 0.7
    return beff

def get_randomized_hosts(hosts_file) -> List[str]:
    entries = []
    with open(hosts_file, "r") as file:
        for line in file:
            item = line.strip()
            if not item or item.startswith('#'):
                continue
            entries.append(item)

    random.shuffle(entries)
    return entries
def parse_size(size_str: str) -> int:
    size_str = size_str.strip().upper()
    units = {'K': 1024, 'M': 1024**2, 'G': 1024**3, 'T': 1024**4}

    if size_str[-1] in units:
        number_str = size_str[:-1]
        unit = units[size_str[-1]]
    else:
        number_str = size_str
        unit = 1
    try:
        number = float(number_str)
        return int(number * unit)
    except ValueError:
        raise ValueError(f"Invalid size string: {size_str}")

def parse_algbw_after_header(text, target_size, tolerance=1000):
    parsing_enabled = False

    for line_num, line in enumerate(text.strip().splitlines(), 1):
        line = line.strip()
        if line.startswith('#') and 'algbw' in line.lower() and 'busbw' in line.lower():
            if 'size' in line.lower() and 'count' in line.lower():
                parsing_enabled = True
            continue

        if not parsing_enabled:
            continue
        if not line or line.startswith('#'):
            continue

        parts = line.split()
        if len(parts) <= 11:
            continue
        try:
            size = int(parts[0])
            if abs(size - target_size) <= tolerance:
                if RCCL_TEST_TYPE == 0:
                    algbw = float(parts[11])
                else:
                    algbw = float(parts[10])
                return algbw
        except ValueError:
            continue
    return 0.0


def run_rccl_test(nodes: List[str]) -> float:
    """
    do rccl/all_reduce_perf or rccl/alltoall_perf test on specified nodes
    return: algbw (GB/s)
    """
    if len(nodes) < 2:
        log(f"[WARN] Not enough nodes ({nodes}) for RCCL test.")
        return 0.0

    nodes_str = ",".join([f"{node}" for node in nodes])
    np = len(nodes) * NUM_GPUS_PER_NODE
    dev0 = RCCL_IB_HCA.split(',')[0]

    cmd = [
        MPIEXEC, "-n", str(np), "-ppn", str(NUM_GPUS_PER_NODE),
        "-launcher", "ssh",
        "-hosts", nodes_str,
    ]
    env_vars = os.environ.copy()
    env_vars["MPIEXEC_ALLOW_ROOT"] = "1"
    env_vars["NCCL_IB_HCA"] = RCCL_IB_HCA
    env_vars["NCCL_SOCKET_IFNAME"] = RCCL_SOCKET_IFNAME
    env_vars["UCX_NET_DEVICES"] = dev0 + ":1"
    env_vars["NCCL_IB_GID_INDEX"] = str(NCCL_IB_GID_INDEX)
    env_vars["LD_LIBRARY_PATH"] = LD_LIBRARY_PATH
    env_vars["MPIEXEC_RSH"] = f"ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p {SSH_PORT}"
    if DEBUG_MODE:
        env_vars["NCCL_DEBUG"] = "INFO"
    if RCCL_TEST_TYPE == 0:
        RCCL_TEST = RCCL_ALL_REDUCE_PERF
    elif RCCL_TEST_TYPE == 1:
        RCCL_TEST = RCCL_ALL_TO_ALL_PERF
        env_vars["NCCL_PXN_DISABLE"] = "0"
        env_vars["NCCL_P2P_NET_CHUNKSIZE"] = "524288"
    else:
        raise ValueError("Invalid RCCL_TEST_TYPE")
    cmd.append(RCCL_TEST)
    cmd.extend(["-b", "16M", "-e", MAX_BYTES, "-f", "2", "-g", "1"])

    log_file = get_log_filename(nodes)
    log(f"# Timestamp: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    log(f"# Log: {log_file}")
    env_str_parts = []
    for k, v in env_vars.items():
        if k.startswith('MPI') or k.startswith('NCCL') or k.startswith('LD_') or k.startswith('UCX_') or  k.startswith('RCCL_'):
            env_str_parts.append(f'{k}="{v}"')
    env_str_for_manual_exec = " ".join(env_str_parts)
    cmd_str_for_manual_exec = " ".join(cmd)
    full_manual_cmd = f"{env_str_for_manual_exec} {cmd_str_for_manual_exec}"
    log(f"# Command (for manual execution): {full_manual_cmd}")

    try:
        with open(log_file, "w") as f:
            result = subprocess.run(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                timeout=600,
                env=env_vars
            )
            print(result.stdout)
            f.write(result.stdout)

        target_size = parse_size(MAX_BYTES)
        algbw = parse_algbw_after_header(result.stdout, target_size)
        if algbw == 0.0:
            log(f"[FAIL] Failed to parse algbw from output for {nodes}")
        else:
            log(f"[INFO] After test on {nodes}, algbw = {algbw:.2f} GB/s")
        return algbw
    except subprocess.TimeoutExpired:
        log(f"[Exception] RCCL test timed out for {nodes}")
        return 0.0
    except Exception as e:
        log(f"[Exception] Test failed for {nodes}: {e}")
        return 0.0

def split_list(lst: List[str]) -> Tuple[List[str], List[str]]:
    lst = lst.copy()
    random.shuffle(lst)
    mid = len(lst) // 2
    return lst[:mid], lst[mid:]

def diagnose_single_with_healthy(suspect_node: str, timeout: float = 900.0) -> Tuple[str, bool]:
    """
    Single suspicious node and healthy node combination test
    Retrieve a healthy node from the global health node pool and return it after testing is completed
    """
    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            healthy_node = healthy_node_queue.get_nowait()
            log(f"[COMBINE] Testing {suspect_node} + {healthy_node} ...")
            test_nodes=[suspect_node, healthy_node]
            algbw = run_rccl_test(test_nodes)
            limit = threshold(len(test_nodes))
            is_faulty = algbw < limit
            log(f"[RESULT] {suspect_node}+{healthy_node} -> {algbw:.2f} GB/s, threshold:{limit:.2f} GB/s-> {'FAULTY' if is_faulty else 'OK'}")
            healthy_node_queue.put(healthy_node)
            return suspect_node, is_faulty
        except Empty:
            time.sleep(1)
            continue
        except Exception as e:
            log(f"[ERROR] Exception during test for {suspect_node}: {e}")
            if 'healthy_node' in locals():
                healthy_node_queue.put(healthy_node)
            return suspect_node, True

    log(f"[TIMEOUT] failed to get healthy node for {suspect_node}, using fallback method")
    return suspect_node, True

def recursive_diagnose(nodes: List[str]) -> List[str]:
    """
    Recursively diagnose nodes and return the finally confirmed faulty nodes (those still < threshold when combined with healthy nodes).
    """
    algbw = run_rccl_test(nodes)
    limit = threshold(len(nodes))
    log(f"[RESULT] {nodes} -> {algbw:.2f} GB/s, threshold: {limit:.2f} GB/s")

    if algbw >= limit:
        log(f"[PASS] Group {nodes} is healthy. Adding to global healthy pool.")
        for node in nodes:
            healthy_node_queue.put(node)
        return []

    if len(nodes) <= 2:
        if healthy_node_queue.empty() and len(nodes) == total_nodes:
            log(f"[WARNING] All nodes appear to be faulty or no healthy nodes available for comparison")
            return nodes.copy()

        log(f"[FINAL CHECK] Testing {nodes} individually with healthy nodes.")
        bad_nodes = []
        # Parallel testing (up to MAX_CONCURRENT_TESTS)
        with ThreadPoolExecutor(max_workers=min(MAX_CONCURRENT_TESTS, len(nodes))) as executor:
            futures = [executor.submit(diagnose_single_with_healthy, node) for node in nodes]
            for future in futures:
                try:
                    node, is_faulty = future.result()
                    if is_faulty:
                        bad_nodes.append(node)
                        log(f"[FAIL] {node} confirmed faulty.")
                    else:
                        healthy_node_queue.put(node)
                        log(f"[PASS] {node} passed with healthy node.")
                except Exception as e:
                    node = "unknown"
                    log(f"[Exception] during test for {node}: {e}")
                    bad_nodes.append(node)
        return bad_nodes

    group_a, group_b = split_list(nodes)
    confirmed_bad = []
    log(f"[SPLIT] {nodes} -> A: {group_a}, B: {group_b}")

    with ThreadPoolExecutor(max_workers=2) as executor:
        future_a = executor.submit(recursive_diagnose, group_a)
        future_b = executor.submit(recursive_diagnose, group_b)
        if future_a:
            bad_a = future_a.result()
            confirmed_bad.extend(bad_a)
        if future_b:
            bad_b = future_b.result()
            confirmed_bad.extend(bad_b)
    return list(set(confirmed_bad))

def parse_args() -> List[str]:
    parser = argparse.ArgumentParser(description="RCCL Fault Diagnoser")
    # Maximum concurrent testing tasks (to avoid system overload)
    parser.add_argument("--max-concurrent", type=int, default=8, help="Max concurrent")
    # enable debug
    parser.add_argument("--debug", action="store_true", help="Enable NCCL_DEBUG=INFO")
    parser.add_argument("--socket-ifname", type=str, default="ens51f0",
                        help="Network interface for RCCL_SOCKET_IFNAME (default: ens51f0)")
    parser.add_argument("--ib-hca", type=str, default="bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7",
                        help="InfiniBand HCAs for RCCL_IB_HCA (default: bnxt_re[0-7])")
    parser.add_argument("--ssh-port", type=int, default="22",
                        help="port for SSH to connect to (default: 22)")
    parser.add_argument("--nodes-file", type=str, default="/root/hosts",
                        help="node list file")
    parser.add_argument("--max-bytes", type=str, default="1G",
                        help="maxbytes for rccl-test")
    parser.add_argument("--ib-gid-index", type=int, default=3, help="NCCL_IB_GID_INDEX")
    parser.add_argument("--rccl-test-type", type=int, default=0, choices=[0, 1], help="0: all_reduce_perf, 1: alltoall_perf")
    args = parser.parse_args()

    global MAX_CONCURRENT_TESTS, DEBUG_MODE, RCCL_SOCKET_IFNAME, RCCL_IB_HCA, SSH_PORT, NCCL_IB_GID_INDEX, RCCL_TEST_TYPE, MAX_BYTES
    MAX_CONCURRENT_TESTS = args.max_concurrent
    DEBUG_MODE = args.debug
    RCCL_SOCKET_IFNAME = args.socket_ifname
    RCCL_IB_HCA = args.ib_hca
    SSH_PORT = args.ssh_port
    NCCL_IB_GID_INDEX = args.ib_gid_index
    RCCL_TEST_TYPE = args.rccl_test_type
    MAX_BYTES = args.max_bytes

    nodes = get_randomized_hosts(args.nodes_file)
    return nodes

def main():
    nodes = parse_args()
    if len(nodes) < 2:
        print("Error: At least 2 nodes are required.")
        sys.exit(1)

    log(f"🔍 Starting diagnosis on {nodes}, rccl_test_type={RCCL_TEST_TYPE}")
    log("⚙️ Starting recursive diagnosis...")
    global healthy_node_queue, total_nodes
    total_nodes = len(nodes)
    healthy_node_queue = Queue()

    bad_nodes = recursive_diagnose(nodes)
    if bad_nodes:
        if RCCL_TEST_TYPE == 0:
            log(f"[ERROR] unhealthy nodes: {bad_nodes}, obtained through all_reduce_perf")
        else:
            log(f"[ERROR] unhealthy nodes: {bad_nodes}, obtained through alltoall_perf")
        sys.exit(1)
    else:
        if RCCL_TEST_TYPE == 0:
            log(f"[SUCCESS] ✅ all passed, obtained through all_reduce_perf")
        else:
            log(f"[SUCCESS] ✅ all passed, obtained through alltoall_perf")
        sys.exit(0)

if __name__ == "__main__":
    main()
