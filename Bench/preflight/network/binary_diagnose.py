#!/usr/bin/env python3

#  Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

import argparse
import datetime
import hashlib
import os
import re
import subprocess
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from queue import Queue, Empty
from typing import List, Tuple, Dict

# ================= Configuration =================
# System paths
MPIEXEC = "/opt/mpich/bin/mpirun"
RCCL_TESTS = {
    0: "/opt/rccl-tests/build/all_reduce_perf",
    1: "/opt/rccl-tests/build/alltoall_perf"
}

# Default settings
NUM_GPUS_PER_NODE = 8
LD_LIBRARY_PATH = "/opt/rocm/lib:/opt/mpich/lib:/usr/local/lib"

# Runtime variables (will be updated by parse_args)
RCCL_DEBUG = "DEBUG"
RCCL_SOCKET_IFNAME = "ens51f0"
RCCL_IB_HCA = "bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7"
NCCL_IB_GID_INDEX = 3
RCCL_TEST_TYPE = 0
SSH_PORT = 22
ENABLE_AINIC = False
MAX_BYTES = "1G"
MAX_CONCURRENT_TESTS = 8

# Global state
total_nodes = 0
total_failed_nodes = 0
healthy_node_queue: Queue[str] = Queue()
print_lock = threading.Lock()
stat_lock = threading.Lock()
# ===========================================

def log(msg: str) -> None:
    """Thread-safe logging with timestamp."""
    with print_lock:
        timestamp = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        print(f"[{timestamp}] {msg}", flush=True)

def get_log_filename(nodes: List[str]) -> str:
    node_str = ",".join(sorted(nodes))
    hash_obj = hashlib.sha256(node_str.encode('utf-8'))
    hash_hex = hash_obj.hexdigest()[:16]
    return f"/tmp/rccl_test_{hash_hex}.log"

def threshold(node_count: int) -> float:
    """Calculate bandwidth threshold for given node count."""
    G_PER_NODE = 8
    
    if RCCL_TEST_TYPE == 0:  # all_reduce
        return 350.0 * node_count * G_PER_NODE / (2 * node_count * G_PER_NODE - 1) * 0.85
    
    # alltoall test
    bnic = float(os.environ.get('BNIC', '48.0'))
    bxgmi = float(os.environ.get('BXGMI', '315.0'))
    
    remote_frac = (node_count - 1) / node_count
    local_frac = (G_PER_NODE - 1) / (G_PER_NODE * node_count)
    
    return 0.7 / (remote_frac / bnic + local_frac / bxgmi)

def get_hosts(hosts_file: str) -> List[str]:
    """Read hosts from file, skipping comments and empty lines."""
    with open(hosts_file, "r") as f:
        return [line.strip() for line in f 
                if line.strip() and not line.strip().startswith('#')]

def parse_size(size_str: str) -> int:
    """
    Parse size string (e.g., '8G', '1024M', '512') to bytes.
    Supports K/KB, M/MB, G/GB, T/TB units (case-insensitive).
    """
    if not size_str:
        raise ValueError("Empty size string")
    
    size_str = size_str.strip().upper()
    
    # Define unit multipliers
    units = {
        'K': 1024, 'KB': 1024,
        'M': 1024**2, 'MB': 1024**2,
        'G': 1024**3, 'GB': 1024**3,
        'T': 1024**4, 'TB': 1024**4,
        'B': 1  # Support explicit byte suffix
    }
    
    # Extract number and unit parts
    match = re.match(r'^([\d.]+)\s*([KMGTB]{1,2})?$', size_str)
    if not match:
        raise ValueError(f"Invalid size format: '{size_str}'. Expected format: number[unit], e.g., '8G', '1024M'")
    
    number_str, unit_str = match.groups()
    
    try:
        number = float(number_str)
        if number < 0:
            raise ValueError(f"Size cannot be negative: {size_str}")
    except ValueError:
        raise ValueError(f"Invalid number in size string: '{number_str}'")
    
    # Get multiplier (default to 1 if no unit specified)
    multiplier = units.get(unit_str, 1) if unit_str else 1
    
    return int(number * multiplier)

def format_size(size_bytes: int, precision: int = 0) -> str:
    """
    Format byte size to human-readable string with appropriate unit.
    Args:
        size_bytes: Size in bytes
        precision: Number of decimal places (default 0 for integer output)
    Returns:
        Formatted string like '8G', '1024M', etc.
    """
    if size_bytes < 0:
        raise ValueError("Size cannot be negative")
    
    # Define thresholds for each unit (use 1024 as base)
    units = [
        (1024**4, 'T'),
        (1024**3, 'G'),
        (1024**2, 'M'),
        (1024, 'K'),
        (1, 'B')
    ]
    
    for threshold, unit in units:
        if size_bytes >= threshold:
            value = size_bytes / threshold
            if precision == 0:
                # For integer output, only show decimal if needed
                if value == int(value):
                    return f"{int(value)}{unit}"
                else:
                    # Round to nearest integer
                    return f"{int(round(value))}{unit}"
            else:
                return f"{value:.{precision}f}{unit}"
    
    return "0B"

def parse_algbw(text: str, target_size: int, tolerance: int = 10000) -> float:
    """Parse algbw value from RCCL test output for a specific size."""
    lines = text.strip().splitlines()
    header_found = False
    
    for line in lines:
        line = line.strip()
        
        # Find header to enable parsing
        if not header_found:
            if line.startswith('#') and all(k in line.lower() for k in ['algbw', 'busbw', 'size', 'count']):
                header_found = True
            continue
        
        # Skip comments and empty lines
        if not line or line.startswith('#'):
            continue
        
        # Parse data line
        parts = line.split()
        if len(parts) > 10:  # Need at least 11 parts to access parts[10] (in-place algbw)
            try:
                size = int(parts[0])
                if abs(size - target_size) <= tolerance:
                    return float(parts[10])  # In-place algbw column
            except (ValueError, IndexError):
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

def build_env_vars() -> Dict[str, str]:
    """Build environment variables for RCCL test."""
    env = os.environ.copy()
    
    # Common environment variables for all modes
    env.update({
        "MPIEXEC_ALLOW_ROOT": "1",
        "NCCL_SOCKET_IFNAME": RCCL_SOCKET_IFNAME,
        "NCCL_IB_GID_INDEX": str(NCCL_IB_GID_INDEX),
        "NCCL_IB_HCA": RCCL_IB_HCA,
        "NCCL_IB_DISABLE": "0",
        "NCCL_IB_PCI_RELAXED_ORDERING": "1",
        "NCCL_SHM_DISABLE": "1",
        "NCCL_CHECKS_DISABLE": "1",
        "NCCL_CROSS_NIC": "0",
        "RCCL_MSCCL_ENABLE": "0",
        "NCCL_DEBUG": RCCL_DEBUG,
        "NCCL_NET_GDR_LEVEL": "2",
        "NCCL_NET_GDR_READ": "1",
        "MPIEXEC_RSH": f"ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p {SSH_PORT}"
    })
    
    if ENABLE_AINIC:
        # AINIC mode: use special library paths and AINIC-specific settings
        # NOTE: Do NOT set NCCL_PXN_DISABLE or NCCL_P2P_NET_CHUNKSIZE in AINIC mode
        env.update({
            "LD_LIBRARY_PATH": f"/opt/amd-anp/build:/opt/amd-anp/build/lib:/opt/rccl/build/release:{LD_LIBRARY_PATH}",
            "NCCL_DMABUF_ENABLE": "0",
            "NCCL_GDR_FLUSH_DISABLE": "1",
            "NCCL_MAX_P2P_CHANNELS": "56",
            "NET_OPTIONAL_RECV_COMPLETION": "1",
            "NCCL_IB_USE_INLINE": "1",
            "RCCL_GDR_FLUSH_GPU_MEM_NO_RELAXED_ORDERING": "0",
            "NCCL_IB_TC": "104",
            "NCCL_IB_FIFO_TC": "192",
            "UCX_NET_DEVICES": RCCL_SOCKET_IFNAME
        })
        # Remove any conflicting PXN-related environment variables
        for key in ['NCCL_PXN_DISABLE', 'NCCL_P2P_NET_CHUNKSIZE']:
            env.pop(key, None)
    else:
        # Standard mode: use default library paths
        env.update({
            "LD_LIBRARY_PATH": LD_LIBRARY_PATH,
            "UCX_NET_DEVICES": RCCL_IB_HCA.split(',')[0] + ":1"
        })
    
    return env

def run_rccl_test(nodes: List[str]) -> float:
    """Run RCCL performance test on specified nodes."""
    if len(nodes) < 2:
        log(f"[WARN] Not enough nodes ({nodes}) for RCCL test.")
        return 0.0

    if not check_connectivity(nodes):
        log(f"[FAIL] Connectivity check failed for nodes {nodes}")
        return 0.0

    # Get test binary
    if RCCL_TEST_TYPE not in RCCL_TESTS:
        raise ValueError(f"Invalid RCCL_TEST_TYPE: {RCCL_TEST_TYPE}")
    rccl_test = RCCL_TESTS[RCCL_TEST_TYPE]
    
    # Build environment variables
    env_vars = build_env_vars()
    
    # Add test-specific optimizations for alltoall tests
    if RCCL_TEST_TYPE == 1:
        if ENABLE_AINIC:
            # AINIC mode: No PXN-related settings, they conflict with AINIC
            # AINIC uses its own optimized data path
            pass
        elif len(nodes) < 16:
            # Non-AINIC mode for small clusters: check if PXN optimization is needed
            pxn_disable = os.getenv('NCCL_PXN_DISABLE', '1')
            env_vars["NCCL_PXN_DISABLE"] = pxn_disable
            
            # Only set P2P_NET_CHUNKSIZE if PXN is enabled (NCCL_PXN_DISABLE != '1')
            # When PXN is disabled, P2P_NET_CHUNKSIZE is not needed
            if pxn_disable != '1':
                env_vars["NCCL_P2P_NET_CHUNKSIZE"] = os.getenv('NCCL_P2P_NET_CHUNKSIZE', '524288')
  
    # Build command
    nodes_str = ",".join(nodes)
    np = len(nodes) * NUM_GPUS_PER_NODE
    cmd = [
        MPIEXEC, "-n", str(np), "-ppn", str(NUM_GPUS_PER_NODE),
        "-launcher", "ssh", "-hosts", nodes_str,
        rccl_test, "-b", "32M", "-e", MAX_BYTES, "-f", "2", "-g", "1"
    ]

    log_file = get_log_filename(nodes)
    log(f"# Log: {log_file}")
    
    # Build manual execution command for debugging
    relevant_prefixes = ('MPI', 'NCCL_', 'LD_', 'UCX_', 'RCCL_', 'ANP_', 'HSA_')
    env_str = " ".join(f'{k}="{v}"' for k, v in env_vars.items() 
                       if any(k.startswith(p) for p in relevant_prefixes))
    cmd_str = " ".join(cmd)
    log(f"# Command (for manual execution): {env_str} {cmd_str}")

    try:
        with open(log_file, "w") as f:
            # Use Popen for real-time output
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                env=env_vars
            )
            output_lines = []
            start_time = time.time()
            timeout_seconds = 300
            
            while True:
                # Check if process has finished
                retcode = process.poll()
                
                # Read available output
                line = ""
                if process.stdout:
                    line = process.stdout.readline()
                    if line:
                        print(line, end='', flush=True)
                        f.write(line)
                        f.flush()  # Ensure data is written to file
                        output_lines.append(line)
                
                # Check timeout
                if time.time() - start_time > timeout_seconds:
                    process.kill()
                    raise subprocess.TimeoutExpired(cmd, timeout_seconds)
                
                # If process finished and no more output, break
                if retcode is not None and not line:
                    # Read any remaining output after process ends
                    remaining = process.stdout.read()
                    if remaining:
                        print(remaining, end='', flush=True)
                        f.write(remaining)
                        f.flush()
                        output_lines.append(remaining)
                    break
            
            result_stdout = ''.join(output_lines)

        target_size = parse_size(MAX_BYTES)
        algbw = parse_algbw(result_stdout, target_size)
        if algbw == 0.0:
            log(f"[FAIL] Failed to parse algbw from output for {nodes}")
        else:
            test_name = "all_reduce_perf" if RCCL_TEST_TYPE == 0 else "alltoall_perf"
            log(f"[INFO] After {test_name} on {nodes}, count={len(nodes)}, algbw = {algbw:.2f} GB/s")
        return algbw
    except subprocess.TimeoutExpired:
        log(f"[Exception] RCCL test timed out for {nodes}")
        return 0.0
    except Exception as e:
        log(f"[Exception] Test failed for {nodes}: {e}")
        return 0.0


def diagnose_single_with_healthy(suspect_node: str, timeout: float = 600.0) -> Tuple[str, bool]:
    """
    Single suspicious node and healthy node combination test
    Retrieve a healthy node from the global health node pool and return it after testing is completed
    """
    start_time = time.time()
    healthy_node = None
    
    while time.time() - start_time < timeout:
        try:
            healthy_node = healthy_node_queue.get_nowait()
            log(f"[COMBINE] Testing {suspect_node} + {healthy_node} ...")
            test_nodes=[suspect_node, healthy_node]
            algbw = run_rccl_test(test_nodes)
            limit = threshold(len(test_nodes))
            is_faulty = algbw < limit
            test_name = "all_reduce_perf" if RCCL_TEST_TYPE == 0 else "alltoall_perf"
            log(f"[INFO] {test_name} {suspect_node}+{healthy_node} -> {algbw:.2f} GB/s, threshold:{limit:.2f} GB/s-> {'FAULTY' if is_faulty else 'OK'}")
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
            if healthy_node is not None:
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
    test_name = "all_reduce_perf" if RCCL_TEST_TYPE == 0 else "alltoall_perf"
    log(f"[INFO] {test_name} {nodes} -> {algbw:.2f} GB/s, threshold: {limit:.2f} GB/s")

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

    # Split nodes into two groups
    mid = len(nodes) // 2
    group_a, group_b = nodes[:mid], nodes[mid:]
    confirmed_bad = []
    log(f"[SPLIT] {nodes} -> A: {group_a}, B: {group_b}")

    with ThreadPoolExecutor(max_workers=2) as executor:
        future_a = executor.submit(recursive_diagnose, group_a)
        future_b = executor.submit(recursive_diagnose, group_b)
        bad_a = future_a.result()
        bad_b = future_b.result()
        confirmed_bad.extend(bad_a)
        confirmed_bad.extend(bad_b)
    return list(set(confirmed_bad))

def parse_args() -> List[str]:
    """Parse command line arguments and update global configuration."""
    parser = argparse.ArgumentParser(description="RCCL Fault Preflight")
    parser.add_argument("--max_concurrent", type=int, default=8, help="Max concurrent tests")
    parser.add_argument("--rccl_debug", type=str, default="DEBUG", help="NCCL_DEBUG level")
    parser.add_argument("--socket_ifname", type=str, default="ens51f0", help="Network interface")
    parser.add_argument("--ib_hca", type=str, 
                       default="bnxt_re0,bnxt_re1,bnxt_re2,bnxt_re3,bnxt_re4,bnxt_re5,bnxt_re6,bnxt_re7",
                       help="InfiniBand HCAs")
    parser.add_argument("--ssh_port", type=int, default=22, help="SSH port")
    parser.add_argument("--nodes_file", type=str, default="/root/hosts", help="Node list file")
    parser.add_argument("--ib_gid_index", type=int, default=None, help="NCCL_IB_GID_INDEX")
    parser.add_argument("--rccl_test_type", type=int, default=0, choices=[0, 1], 
                       help="0: all_reduce_perf, 1: alltoall_perf")
    parser.add_argument("--enable_ainic", type=str, default="false", 
                       help="Enable AINIC mode (disables PXN, uses ANP libraries)")
    
    args = parser.parse_args()
    
    # Update global configuration
    global MAX_CONCURRENT_TESTS, RCCL_DEBUG, RCCL_SOCKET_IFNAME, RCCL_IB_HCA
    global SSH_PORT, NCCL_IB_GID_INDEX, RCCL_TEST_TYPE, MAX_BYTES, ENABLE_AINIC
    
    MAX_CONCURRENT_TESTS = args.max_concurrent
    RCCL_DEBUG = args.rccl_debug
    RCCL_SOCKET_IFNAME = args.socket_ifname
    RCCL_IB_HCA = args.ib_hca
    SSH_PORT = args.ssh_port
    RCCL_TEST_TYPE = args.rccl_test_type
    # Check both command line and environment for AINIC enablement
    ENABLE_AINIC = (args.enable_ainic.lower() == 'true' or 
                   os.environ.get('ENABLE_AINIC', '').lower() == 'true')
    
    if args.ib_gid_index is not None:
        NCCL_IB_GID_INDEX = args.ib_gid_index
    
    # Get nodes and set MAX_BYTES based on cluster size
    nodes = get_hosts(args.nodes_file)
    
    # Scale MAX_BYTES based on cluster size for more efficient testing
    node_count = len(nodes)
    if node_count >= 64:
        MAX_BYTES = "16G"  # Large clusters (64+ nodes): 16G
    elif node_count >= 8:
        MAX_BYTES = "8G"   # Medium clusters (8-63 nodes): 8G
    elif node_count > 4:
        MAX_BYTES = "4G"   # Medium clusters (5-63 nodes): 8G
    elif node_count > 2:
        MAX_BYTES = "2G"   # Small clusters (3-4 nodes): 4G
    else:
        MAX_BYTES = "1G"   # Tiny clusters (1-2 nodes): 2G

    
    return nodes

def main():
    """Main entry point for the diagnostic tool."""
    nodes = parse_args()
    
    if len(nodes) < 2:
        print("Error: At least 2 nodes are required.")
        sys.exit(1)
    
    # Get test name for logging
    test_name = "all_reduce_perf" if RCCL_TEST_TYPE == 0 else "alltoall_perf"
    
    # Log configuration details
    log(f"🔍 Starting diagnosis on {len(nodes)} nodes: {nodes}, test={test_name}")
    if ENABLE_AINIC:
        log("📌 AINIC mode enabled: PXN disabled, using ANP libraries")
    else:
        log("📌 Standard mode: using default RCCL configuration")
    log(f"📊 Test parameters: MAX_BYTES={MAX_BYTES} (adaptive for {len(nodes)} nodes), Interface={RCCL_SOCKET_IFNAME}")
    log("⚙️ Starting recursive diagnosis...")
    
    # Initialize global state
    global healthy_node_queue, total_nodes
    total_nodes = len(nodes)
    healthy_node_queue = Queue()
    
    # Run diagnosis
    bad_nodes = recursive_diagnose(nodes)
    
    # Report results
    if bad_nodes:
        log(f"[RESULT] unhealthy nodes: {bad_nodes}, obtained through {test_name}")
        sys.exit(1)
    else:
        log(f"[RESULT] ✅ all passed, obtained through {test_name}")
        sys.exit(0)

if __name__ == "__main__":
    main()