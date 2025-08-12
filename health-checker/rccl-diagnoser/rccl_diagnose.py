#!/usr/bin/env python3

#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

import subprocess
import sys
import argparse
import random
import time
from typing import List, Tuple
from queue import Queue
import threading
import hashlib
from concurrent.futures import ThreadPoolExecutor

# ================= configuration =================
MPIEXEC = "/opt/openmpi-4.1.8/bin/mpirun"
RCCL_TEST = "/root/rccl-tests/build/all_reduce_perf"
NUM_GPUS_PER_NODE = 8
TEST_SIZE = "1G"

LD_LIBRARY_PATH = "/opt/rocm/lib:/usr/lib/x86_64-linux-gnu/openmpi/lib"
RCCL_SOCKET_IFNAME = "ens51f0"
RCCL_IB_HCA = "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
NCCL_IB_GID_INDEX = "3"

DEBUG_MODE = False
healthy_node_queue: Queue[str] = Queue()
# for log output
print_lock = threading.Lock()

total_nodes = 0
total_unhealthy_nodes = 0
stats_lock = threading.Lock()

# ===========================================

def log(msg: str):
    with print_lock:
        print(msg)

def get_log_filename(nodes: List[str]) -> str:
    node_str = ",".join(sorted(nodes))
    hash_obj = hashlib.sha256(node_str.encode('utf-8'))
    hash_hex = hash_obj.hexdigest()[:16]
    return f"/tmp/rccl_test_{hash_hex}.log"

def get_sort_ip(hosts_file):
    ip_list = []
    with open(hosts_file, "r") as file:
        for line in file:
            if line.strip() and line.strip()[0].isdigit():
                ip_list.append(line.strip())

    ip_int_list = [list(map(int, ip.split('.'))) for ip in ip_list]
    ip_int_list.sort()
    sorted_ip_list = [",".join(map(str, ip)) for ip in ip_int_list]
    return sorted_ip_list

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

def run_rccl_test(nodes: List[str]) -> float:
    """
    do rccl/all_reduce_perf test on specified nodes
    return: busbw (GB/s)
    """
    if len(nodes) < 2:
        log(f"[WARN] Not enough nodes ({nodes}) for RCCL test.")
        return 0.0

    np = len(nodes) * NUM_GPUS_PER_NODE
    host_str = ",".join([f"{node}:{NUM_GPUS_PER_NODE}" for node in nodes])

    cmd = [
        MPIEXEC, "-np", str(np), "-N", str(NUM_GPUS_PER_NODE),
        "--allow-run-as-root",
        "--mca", "routed", "direct",
        "--mca", "oob_tcp_if_include", RCCL_SOCKET_IFNAME,
        "--mca", "btl_tcp_if_include", RCCL_SOCKET_IFNAME,
        "--host", host_str,
        "-x", f"LD_LIBRARY_PATH={LD_LIBRARY_PATH}",
        "-x", f"RCCL_SOCKET_IFNAME={RCCL_SOCKET_IFNAME}",
        "-x", f"RCCL_IB_HCA={RCCL_IB_HCA}",
        "-x", f"NCCL_IB_GID_INDEX={NCCL_IB_GID_INDEX}",
    ]
    if DEBUG_MODE:
        cmd.extend(["-x", "NCCL_DEBUG=INFO"])
    cmd.append(RCCL_TEST)
    cmd.extend(["-b", "64M", "-e", TEST_SIZE, "-f", "2", "-g", "1"])

    log_file = get_log_filename(nodes)
    log(f"# Timestamp: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    log(f"# Log: {log_file}")
    log(f"# Command: {' '.join(cmd)}")

    try:
        with open(log_file, "w") as f:
            result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                    text=True, timeout=600)  # 10 minutes timeout
            print(result.stdout)
            f.write(result.stdout)

        target_size = str(parse_size(TEST_SIZE))
        lines = result.stdout.splitlines()
        for line in lines:
            if target_size in line:
                parts = line.strip().split()
                if len(parts) >= 8:
                    try:
                        busbw = float(parts[7])
                        log(f"[INFO] After test on {nodes}, busbw = {busbw:.2f} GB/s")
                        return busbw
                    except ValueError:
                        continue
        log(f"[FAIL] Failed to parse busbw from output for {nodes}")
        return 0.0
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

def diagnose_single_with_healthy(suspect_node: str, timeout: float = 1800.0) -> Tuple[str, bool]:
    """
    Single suspicious node and healthy node combination test
    Retrieve a healthy node from the global health node pool and return it after testing is completed
    """

    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            healthy_node = healthy_node_queue.get_nowait()
            log(f"[COMBINE] Testing {suspect_node} + {healthy_node} ...")
            busbw = run_rccl_test([suspect_node, healthy_node])
            is_faulty = busbw < THRESHOLD_GBPS
            log(f"[RESULT] {suspect_node}+{healthy_node} -> {busbw:.2f} GB/s -> {'FAULTY' if is_faulty else 'OK'}")
            healthy_node_queue.put(healthy_node)
            return suspect_node, is_faulty
        except Exception:
            with stats_lock:
                if total_unhealthy_nodes >= total_nodes:
                    break
            time.sleep(1)
            continue
    log(f"[TIMEOUT] failed to get healthy node for {suspect_node}")
    return suspect_node, True

def recursive_diagnose(nodes: List[str]) -> List[str]:
    """
    Recursively diagnose nodes and return the finally confirmed faulty nodes (those still < 300 when combined with healthy nodes).
    """
    global total_unhealthy_nodes
    busbw = run_rccl_test(nodes)
    log(f"[RESULT] {nodes} -> {busbw:.2f} GB/s")

    if busbw >= THRESHOLD_GBPS:
        log(f"[PASS] Group {nodes} is healthy. Adding to global healthy pool.")
        for node in nodes:
            healthy_node_queue.put(node)
        return []

    if len(nodes) <= 2:
        with stats_lock:
            total_unhealthy_nodes += len(nodes)
            if total_unhealthy_nodes >= total_nodes:
                return nodes

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

def main():
    parser = argparse.ArgumentParser(description="RCCL Fault Diagnoser")
    # threshold of bandwidth (GB/s)
    parser.add_argument("--threshold", type=float, default=300.0, help="Threshold in GB/s")
    # Maximum concurrent testing tasks (to avoid system overload)
    parser.add_argument("--max-concurrent", type=int, default=8, help="Max concurrent")
    # enable debug
    parser.add_argument("--debug", action="store_true", help="Enable NCCL_DEBUG=INFO")
    parser.add_argument("--socket-ifname", type=str, default="ens51f0",
                        help="Network interface for RCCL_SOCKET_IFNAME (default: ens51f0)")
    parser.add_argument("--ib-hca", type=str, default="bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7",
                    help="InfiniBand HCAs for RCCL_IB_HCA (default: bnxt_re[0-7])")
    args = parser.parse_args()

    nodes = get_sort_ip("/root/hosts")
    if len(nodes) < 2:
        print("Error: At least 2 nodes are required.")
        sys.exit(1)

    global THRESHOLD_GBPS, MAX_CONCURRENT_TESTS, DEBUG_MODE,  RCCL_SOCKET_IFNAME, RCCL_IB_HCA
    THRESHOLD_GBPS = args.threshold
    MAX_CONCURRENT_TESTS = args.max_concurrent
    DEBUG_MODE = args.debug
    RCCL_SOCKET_IFNAME = args.socket_ifname
    RCCL_IB_HCA = args.ib_hca

    log(f"🔍 Starting diagnosis on {nodes}, threshold = {THRESHOLD_GBPS} GB/s")
    log("⚙️ Starting recursive diagnosis...")
    global healthy_node_queue
    healthy_node_queue = Queue()
    global total_nodes, total_unhealthy_nodes
    total_nodes = len(nodes)
    total_unhealthy_nodes = 0

    bad_nodes = recursive_diagnose(nodes)
    if bad_nodes:
        log(f"[ERROR] unhealthy nodes: {bad_nodes}")
        sys.exit(1)
    else:
        log("[SUCCESS] all passed")
        sys.exit(0)

if __name__ == "__main__":
    main()