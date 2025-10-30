#!/usr/bin/env python3

#  Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

import subprocess
import sys
import os
import argparse
import time
from typing import List, Tuple
from queue import Queue, Empty
import threading
import hashlib
import ipaddress
from concurrent.futures import ThreadPoolExecutor
import datetime

# ================= configuration =================
MPIEXEC = "/opt/mpich/bin/mpirun"
RCCL_ALL_REDUCE_PERF = "/opt/rccl-tests/build/all_reduce_perf"
RCCL_ALL_TO_ALL_PERF = "/opt/rccl-tests/build/alltoall_perf"
RCCL_DEBUG="DEBUG"
NUM_GPUS_PER_NODE = 8
MAX_BYTES = "1G"

LD_LIBRARY_PATH = "/opt/rocm/lib:/opt/mpich/lib:/usr/local/lib"
RCCL_SOCKET_IFNAME = "ens51f0"
RCCL_IB_HCA = "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
NCCL_IB_GID_INDEX = 3
RCCL_TEST_TYPE = 0
RCCL_TEST_NAME = ""
SSH_PORT = 22

total_nodes = 0
total_failed_nodes = 0
healthy_node_queue: Queue[str] = Queue()
# for log output
print_lock = threading.Lock()
stat_lock = threading.Lock()
# ===========================================

def log(msg: str):
    with print_lock:
        current_time = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        print(f"[{current_time}] {msg}", flush=True)

def get_log_filename(nodes: List[str]) -> str:
    node_str = ",".join(sorted(nodes))
    hash_obj = hashlib.sha256(node_str.encode('utf-8'))
    hash_hex = hash_obj.hexdigest()[:16]
    return f"/tmp/rccl_test_{hash_hex}.log"

def threshold(node_count: int) -> float:
    G_PER_NODE = 8
    if RCCL_TEST_TYPE == 0:
        return 350.0*node_count*G_PER_NODE/(2*node_count*G_PER_NODE-1) *0.85
    try:
        bnic = float(os.environ['BNIC'])
        bxgmi = float(os.environ['BXGMI'])
    except (KeyError, ValueError):
        bnic = 48.0
        bxgmi = 315.0
    # Calculate traffic fractions
    remote_frac = (node_count - 1) / node_count
    local_frac = (G_PER_NODE - 1) / (G_PER_NODE * node_count)
    # Compute effective bandwidth
    beff = 1 / (remote_frac / bnic + local_frac / bxgmi)
    beff *= 0.7
    return beff

def get_hosts(hosts_file) -> List[str]:
    entries = []
    with open(hosts_file, "r") as file:
        for line in file:
            item = line.strip()
            if not item or item.startswith('#'):
                continue
            entries.append(item)
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

def parse_algbw(text, target_size, tolerance=10000):
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
                algbw = float(parts[10])
                return algbw
        except ValueError:
            continue
    return 0.0

def check_connectivity(nodes: List[str], timeout: int = 300) -> bool:
    if len(nodes) < 2:
        return True

    nodes_str = ",".join(nodes)
    np = len(nodes)
    ppn = 1

    cmd = [
        MPIEXEC, "-n", str(np), "-ppn", str(ppn),
        "-launcher", "ssh",
        "-hosts", nodes_str,
        "/bin/echo", "OK"
    ]

    env_vars = os.environ.copy()
    env_vars["MPIEXEC_RSH"] = f"ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p {SSH_PORT}"
    env_vars["MPIEXEC_ALLOW_ROOT"] = "1"
    env_vars["LD_LIBRARY_PATH"] = LD_LIBRARY_PATH

    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            result = subprocess.run(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                timeout=60,
                env=env_vars
            )

            # Check whether all nodes have returned "OK".
            ok_count = result.stdout.count("OK")
            if result.returncode == 0 and ok_count == len(nodes):
                log(f"[CONNECTIVITY] All {len(nodes)} nodes are reachable")
                return True
            else:
                log(f"[CONNECTIVITY] Command output: {result.stdout}, code: {result.returncode}")
                log(f"[CONNECTIVITY] Connectivity test failed ({ok_count}/{len(nodes)} nodes responded), retrying in 10 seconds...")
                time.sleep(10)
        except subprocess.TimeoutExpired:
            log(f"[CONNECTIVITY] Connectivity test timeout, retrying in 10 seconds...")
            time.sleep(10)
        except Exception as e:
            log(f"[CONNECTIVITY] Connectivity test exception: {e}, retrying in 10 seconds...")
            time.sleep(10)

    log(f"[CONNECTIVITY] Failed to establish connectivity within {timeout} seconds")
    return False

def run_rccl_test(nodes: List[str]) -> float:
    """
    do rccl/all_reduce_perf or rccl/alltoall_perf test on specified nodes
    return: algbw (GB/s)
    """
    if len(nodes) < 2:
        log(f"[WARN] Not enough nodes ({nodes}) for RCCL test.")
        return 0.0

    if not check_connectivity(nodes):
        log(f"[FAIL] Connectivity check failed for nodes {nodes}")
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
    env_vars["NCCL_DEBUG"] = RCCL_DEBUG
    env_vars["HSA_NO_SCRATCH_RECLAIM"] = "1"
    env_vars["HSA_FORCE_FINE_GRAIN_PCIE"] = "1"
    env_vars["NCCL_CHECKS_DISABLE"] = "1"
    env_vars["NCCL_ALGO"] = "RING"
    env_vars["RCCL_MSCCL_ENABLE"] = "0"
    env_vars["NCCL_IB_PCI_RELAXED_ORDERING"] = "1"
    env_vars["NCCL_SHM_DISABLE"] = "1"
    env_vars["NCCL_CROSS_NIC"] = "0"
    env_vars["NCCL_NET_GDR_LEVEL"] = "2"
    env_vars["NCCL_NET_GDR_READ"] = "1"
    if RCCL_TEST_TYPE == 0:
        RCCL_TEST = RCCL_ALL_REDUCE_PERF
    elif RCCL_TEST_TYPE == 1:
        RCCL_TEST = RCCL_ALL_TO_ALL_PERF
        if len(nodes) < 16:
            env_vars["NCCL_PXN_DISABLE"] = os.getenv('NCCL_PXN_DISABLE', '1')
            env_vars["NCCL_P2P_NET_CHUNKSIZE"] = os.getenv('NCCL_P2P_NET_CHUNKSIZE', '524288')
    else:
        raise ValueError("Invalid RCCL_TEST_TYPE")
    cmd.append(RCCL_TEST)
    cmd.extend(["-b", "16M", "-e", MAX_BYTES, "-f", "2", "-g", "1"])

    log_file = get_log_filename(nodes)
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
                timeout=300,
                env=env_vars
            )
            print(result.stdout)
            f.write(result.stdout)

        target_size = parse_size(MAX_BYTES)
        algbw = parse_algbw(result.stdout, target_size)
        if algbw == 0.0:
            log(f"[FAIL] Failed to parse algbw from output for {nodes}")
        else:
            log(f"[INFO] After {RCCL_TEST_NAME} on {nodes}, count={len(nodes)}, algbw = {algbw:.2f} GB/s")
        return algbw
    except subprocess.TimeoutExpired:
        log(f"[Exception] RCCL test timed out for {nodes}")
        return 0.0
    except Exception as e:
        log(f"[Exception] Test failed for {nodes}: {e}")
        return 0.0

def split_list(lst: List[str]) -> Tuple[List[str], List[str]]:
    lst = lst.copy()
    mid = len(lst) // 2
    return lst[:mid], lst[mid:]

def diagnose_single_with_healthy(suspect_node: str, timeout: float = 600.0) -> Tuple[str, bool]:
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
            log(f"[INFO] {RCCL_TEST_NAME} {suspect_node}+{healthy_node} -> {algbw:.2f} GB/s, threshold:{limit:.2f} GB/s-> {'FAULTY' if is_faulty else 'OK'}")
            healthy_node_queue.put(healthy_node)
            return suspect_node, is_faulty
        except Empty:
            with stat_lock:
                if total_failed_nodes >= total_nodes:
                    break
            time.sleep(1)
            continue
        except Exception as e:
            log(f"[WARN] Exception during test for {suspect_node}: {e}")
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
    log(f"[INFO] {RCCL_TEST_NAME} {nodes} -> {algbw:.2f} GB/s, threshold: {limit:.2f} GB/s")

    if algbw >= limit:
        log(f"[PASS] Group {nodes} is healthy. Adding to global healthy pool.")
        for node in nodes:
            healthy_node_queue.put(node)
        return []

    if len(nodes) <= 2:
        with stat_lock:
            global total_failed_nodes
            total_failed_nodes += len(nodes)
            if total_failed_nodes >= total_nodes and healthy_node_queue.empty():
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
    parser = argparse.ArgumentParser(description="RCCL Fault Preflight")
    # Maximum concurrent testing tasks (to avoid system overload)
    parser.add_argument("--max_concurrent", type=int, default=8, help="Max concurrent")
    # enable debug
    parser.add_argument("--rccl_debug", type=str, default="DEBUG", help="NCCL_DEBUG")
    parser.add_argument("--socket_ifname", type=str, default="ens51f0",
                        help="Network interface for RCCL_SOCKET_IFNAME (default: ens51f0)")
    parser.add_argument("--ib_hca", type=str, default="bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7",
                        help="InfiniBand HCAs for RCCL_IB_HCA (default: bnxt_re[0-7])")
    parser.add_argument("--ssh_port", type=int, default="22",
                        help="port for SSH to connect to (default: 22)")
    parser.add_argument("--nodes_file", type=str, default="/root/hosts",
                        help="node list file")
    parser.add_argument("--ib_gid_index", type=int, default=3, help="NCCL_IB_GID_INDEX")
    parser.add_argument("--rccl_test_type", type=int, default=0, choices=[0, 1], help="0: all_reduce_perf, 1: alltoall_perf")
    args = parser.parse_args()

    global MAX_CONCURRENT_TESTS, RCCL_DEBUG, RCCL_SOCKET_IFNAME, RCCL_IB_HCA, SSH_PORT, NCCL_IB_GID_INDEX, RCCL_TEST_TYPE, RCCL_TEST_NAME, MAX_BYTES
    MAX_CONCURRENT_TESTS = args.max_concurrent
    RCCL_DEBUG = args.rccl_debug
    RCCL_SOCKET_IFNAME = args.socket_ifname
    RCCL_IB_HCA = args.ib_hca
    SSH_PORT = args.ssh_port
    NCCL_IB_GID_INDEX = args.ib_gid_index
    RCCL_TEST_TYPE = args.rccl_test_type
    if RCCL_TEST_TYPE == 0:
        RCCL_TEST_NAME = "all_reduce_perf"
    else:
        RCCL_TEST_NAME = "alltoall_perf"

    nodes = get_hosts(args.nodes_file)
    if len(nodes) >= 64:
        MAX_BYTES="16G"
    else:
        MAX_BYTES="8G"
    return nodes

def main():
    nodes = parse_args()
    if len(nodes) < 2:
        print("Error: At least 2 nodes are required.")
        sys.exit(0)

    log(f"🔍 Starting diagnosis on {nodes}, test={RCCL_TEST_NAME}")
    log("⚙️ Starting recursive diagnosis...")
    global healthy_node_queue, total_nodes
    total_nodes = len(nodes)
    healthy_node_queue = Queue()

    bad_nodes = recursive_diagnose(nodes)
    if bad_nodes:
        log(f"[RESULT] unhealthy nodes: {bad_nodes}, obtained through {RCCL_TEST_NAME}")
        sys.exit(1)
    else:
        log(f"[RESULT] ✅ all passed, obtained through {RCCL_TEST_NAME}")
        sys.exit(0)

if __name__ == "__main__":
    main()
