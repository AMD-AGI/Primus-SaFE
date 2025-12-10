#!/usr/bin/env python3

import os
import subprocess
import sys
import time
import re
import socket
import glob
import argparse
from datetime import datetime
from typing import List
import concurrent.futures
import threading

# for log output
print_lock = threading.Lock()
IB_WRITE_BW_CMD = '/opt/rdma-perftest/bin/ib_write_bw'
BASE_SSH_CMD = ['ssh', '-o', 'StrictHostKeyChecking=no', '-o', 'UserKnownHostsFile=/dev/null', '-o', 'ConnectTimeout=5']

def log(msg: str):
    with print_lock:
        if len(msg) == 0:
            print()
        else:
            current_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            print(f"[{current_time}] {msg}", flush=True)

def get_ip(interface):
    """
    Get local IP address from network interface

    Args:
        interface (str): Network interface name

    Returns:
        str: IP address or None if not found
    """
    try:
        result = subprocess.run(['ifconfig', interface], capture_output=True, text=True)
        match = re.search(r'inet (\d+\.\d+\.\d+\.\d+)', result.stdout)
        if match:
            return match.group(1)
    except Exception:
        pass
    return None

def kill_remote_listener(node, ssh_cmd):
    """
    Kill remote ib_write_bw process and wait for it to terminate

    Args:
        node (str): Remote node address
        ssh_cmd (list): SSH command list

    Returns:
        bool: True if processes terminated successfully, False otherwise
    """
    log(f"[{node}] Cleaning up remote ib_write_bw process...")

    # Kill all ib_write_bw processes (be more aggressive)
    cmd = ssh_cmd + [node, 'pkill -TERM ib_write_bw 2>/dev/null || true']
    subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    max_retry = 10
    timeout = 0.3
    # Wait for processes to terminate (max 3 seconds)
    for i in range(max_retry):
        cmd = ssh_cmd + [node, 'pgrep ib_write_bw > /dev/null']
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=1)
        if result.returncode != 0:
            log(f"[{node}] Remote ib_write_bw processes successfully terminated")
            return True
        if i == max_retry - 1:
            break
        time.sleep(timeout)

    log(f"[{node}] Warning: Remote ib_write_bw processes may still be running")
    return False

def kill_local_listener(server_process):
    # Clean up the failed server
    server_process.terminate()
    try:
        server_process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        server_process.kill()

def check_remote_server_ready(node, dev, ssh_cmd):
    """
    Check if server is ready

    Args:
        node (str): Remote node address
        dev (str): Device name
        ssh_cmd (list): SSH command list

    Returns:
        bool: True if server is ready, False otherwise
    """
    log(f"[{node}] Checking if server is ready...")
    max_retry = 10
    timeout = 0.3
    cmd = ssh_cmd + [node, 'netstat -anpl | grep ib_write_bw']
    for i in range(max_retry):
        # Check if port is listening
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=1)
        if result.returncode == 0:
            log(f"[{node}] Server on device {dev} is ready")
            return True
        if i == max_retry - 1:
            break
        time.sleep(timeout)

    log(f"[{node}] Server on device {dev} startup timeout")
    return False

def check_local_server_ready(dev):
    """
    Check if local server is ready

    Args:
        dev (str): Device name

    Returns:
        bool: True if server is ready, False otherwise
    """
    log(f"[LOCAL] Checking if server is ready...")
    max_retry = 10
    timeout = 0.3
    for i in range(max_retry):
        # Check if port is listening
        try:
            result = subprocess.run(['netstat', '-anpl'], capture_output=True, text=True, timeout=1)
            if 'ib_write_bw' in result.stdout:
                log(f"[LOCAL] Server on device {dev} is ready")
                return True
        except subprocess.TimeoutExpired:
            pass

        if i == max_retry - 1:
            break
        time.sleep(timeout)

    log(f"[LOCAL] Server on device {dev} startup timeout")
    return False

def get_mapped_hca(index, ib_hca_list):
    """
    Get the corresponding HCA device based on HCA mapping relationship

    Args:
        index (int): the index of input HCA device
        ib_hca_list (list): List of available HCA devices

    Returns:
        str: Mapped HCA device name
    """
    hca_mapping = {0: 0, 1: 2, 2: 4, 3: 6, 4: 7, 5: 5, 6: 3, 7: 1}

    # Only use mapping relationship when HCA count is 8
    if len(ib_hca_list) == 8:
        return ib_hca_list[hca_mapping[index]]

    # By default, return the input HCA (when array element count is not 8 or mapping not found)
    return ib_hca_list[index]

def test_node_pair(client, local_ip, server, client_ib_hca, server_ib_hca, ib_params, ssh_cmd, group_idx):
    """
    Test a single node pair (client -> server)

    Returns: success
    """
    if not server:
        return False

    log("-" * 50)
    log(f"[{datetime.now().strftime('%H:%M:%S')}] TESTING: {server} on {server_ib_hca}, group: {group_idx+1}")
    log("-" * 50)

    # === STEP 1: Start Server (Listener) ===
    log(f"[{server}] Starting server...")
    server_process = None
    try:
        if server == local_ip:
            # For local node, run command directly without SSH
            ib_write_cmd = [IB_WRITE_BW_CMD, '-d', server_ib_hca] + ib_params
            server_process = subprocess.Popen(ib_write_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            server_pid = server_process.pid
            log(f"[{server}] Local server started, PID: {server_pid}, cmd: {' '.join(ib_write_cmd)}")

            if not check_local_server_ready(server_ib_hca):
                log(f"[{server}] failed to start Local server")
                kill_local_listener(server_process)
                return False
        else:
            # For remote node, use SSH as before
            log(f"[{server}] Starting Remote server via SSH...")
            ib_write_cmd = ssh_cmd + [server, f"{IB_WRITE_BW_CMD} -d {server_ib_hca} {' '.join(ib_params)}"]
            server_process = subprocess.Popen(ib_write_cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            server_pid = server_process.pid
            log(f"[{server}] Remote server started, SSH PID: {server_pid}, cmd: {IB_WRITE_BW_CMD} -d {server_ib_hca} {' '.join(ib_params)}")

            # Wait for remote server started
            if not check_remote_server_ready(server, server_ib_hca, ssh_cmd):
                log(f"[{server}] failed to start Remote server ")
                # Clean up the failed server
                kill_remote_listener(server, ssh_cmd)
                return False

        # === STEP 2: Start Client ===
        try:
            if client == local_ip:
                # Local client execution
                client_cmd = [IB_WRITE_BW_CMD, '-d', client_ib_hca] + ib_params + [server]
            else:
                # Remote client execution via SSH
                ib_write_cmd = f"{IB_WRITE_BW_CMD} -d {client_ib_hca} {' '.join(ib_params)} {server}"
                client_cmd = ssh_cmd + [client, ib_write_cmd]
            log(f"[{server}] Starting Client from {client} to {server}, cmd: {client_cmd}")
            result = subprocess.run(client_cmd, capture_output=True, text=True, timeout=40)
            output = result.stdout + result.stderr
            client_ret = result.returncode
        except subprocess.TimeoutExpired as e:
            output = e.stdout.decode() + e.stderr.decode() if e.stdout and e.stderr else ""
            client_ret = 124  # Timeout error code
        except Exception as e:
            output = str(e)
            client_ret = 1

        if client_ret != 0:
            output_client_error(client_ret)

        # === STEP 3: Show Debug Output ===
        log(f"group: {group_idx+1} [{server}] CLIENT OUTPUT:")
        for line in output.split('\n'):
            if line.strip():
                log(f"    > {line}")
        log("")

        # === STEP 4: Cleanup Server ===
        if server == local_ip:
            if server_process:
                kill_local_listener(server_process)
        else:
            kill_remote_listener(server, ssh_cmd)

        # === STEP 5: Determine PASS/FAIL ===
        success = client_ret == 0 and "Gb/sec" in output
        if success:
            log(f"RESULT: PASS - {client_ib_hca} -> {server}:{server_ib_hca}, group: {group_idx+1}")
        else:
            log(f"RESULT: FAIL - {client_ib_hca} -> {server}:{server_ib_hca}, group: {group_idx+1} (exit code: {client_ret})")
            # Highlight common errors
            error_patterns = ["Couldn't connect", "No route", "Device not found", "Permission denied", "timeout"]
            for pattern in error_patterns:
                if pattern in output:
                    log(f"    [ERROR] {pattern}")

        return success
    except Exception:
        # Ensure cleanup even if an exception occurs
        if server_process:
            if server == local_ip:
                kill_local_listener(server_process)
            else:
                kill_remote_listener(server, ssh_cmd)
        return False


def test_node_group(local_ip, node_group, ib_hca_list, ib_params, ssh_cmd, group_idx):
    """
    Test all nodes in a group serially

    Returns:
        list: List of failed nodes
    """

    all_failed = True
    final_failed_nodes = set()

    # Generate a client candidate list: prioritize local IP, followed by other nodes
    client_candidates = []
    if local_ip in node_group:
        client_candidates.append(local_ip)
        client_candidates.extend([node for node in node_group if node != local_ip])
    else:
        client_candidates = node_group

    for i, client_candidate in enumerate(client_candidates):
        log(f"Trying {client_candidate} as client for group {group_idx + 1}")
        current_failed_nodes = set()
        current_success = False

        # Get mapped HCA for this device
        for index, client_ib_hca in enumerate(ib_hca_list):
            server_ib_hca = get_mapped_hca(index, ib_hca_list)
            for node in node_group:
                try:
                    if (client_candidate == node) or (node in current_failed_nodes):
                        continue
                    success = test_node_pair(client_candidate, local_ip, node,
                                             client_ib_hca, server_ib_hca, ib_params, ssh_cmd, group_idx)
                    if not success:
                        current_failed_nodes.add(node)
                    else:
                        current_success = True
                except Exception as exc:
                    log(f"[{node}] Generated an exception: {exc}")
                    current_failed_nodes.add(node)

        # If any test succeeds, then we have found a valid client.
        if current_success:
            log(f"Found valid client {client_candidate} for group {group_idx + 1}")
            final_failed_nodes = current_failed_nodes
            all_failed = False
            break
        elif i == len(node_group) - 1:
            all_failed = True
            break

    if all_failed:
        log(f"All nodes failed as client in group {group_idx + 1}, returning all nodes as failed")
        return list(node_group)
    else:
        log(f"Group {group_idx + 1} testing completed with {len(final_failed_nodes)} failed nodes")
        return list(final_failed_nodes)


def parse_args():
    """
    Parse command line arguments

    Returns:
        argparse.Namespace: Parsed arguments
    """
    parser = argparse.ArgumentParser(description="Run InfiniBand write bandwidth tests between nodes")
    parser.add_argument("--ib_hca",
                        help="Comma-separated list of InfiniBand Host Channel Adapters (HCAs) to test")
    parser.add_argument("--socket_ifname",
                        help="Network interface name used to determine local IP address")
    parser.add_argument("--ib_gid_index",
                        help="GID index for InfiniBand communication")
    parser.add_argument("--nodes_file",
                        help="File containing list of nodes to test against (one per line)")
    parser.add_argument("--ssh_port", type=int, default=22,
                        help="port for SSH to connect to (default: 22)")
    return parser.parse_args()

def build_ssh_cmd(port=None):
    """
    Build SSH command with optional port parameter

    Args:
        port (int, optional): SSH port number

    Returns:
        list: SSH command list
    """
    cmd = BASE_SSH_CMD.copy()
    if port and port != 22:
        cmd.extend(['-p', str(port)])
    return cmd

def get_hosts(hosts_file) -> List[str]:
    """
    Read hosts from file, skipping empty lines and comments

    Args:
        hosts_file (str): Path to hosts file

    Returns:
        list: List of host addresses
    """
    entries = []
    with open(hosts_file, "r") as file:
        for line in file:
            item = line.strip()
            if not item or item.startswith('#'):
                continue
            entries.append(item)
    return entries


def group_nodes(nodes, group_size=16):
    """
    Group nodes into batches of specified size.
    If the last group contains only one node, merge it into the previous group.

    Args:
        nodes (list): List of nodes to be grouped
        group_size (int): Maximum number of nodes per group, default is 16

    Returns:
        list: List of node groups, each element is a list containing nodes
    """
    if not nodes:
        return []

    # First, group nodes by group_size
    node_groups = [nodes[i:i + group_size] for i in range(0, len(nodes), group_size)]

    # If the last group has only one node and there are at least two groups,
    # merge it into the second-to-last group
    if len(node_groups) > 1 and len(node_groups[-1]) == 1:
        # Move the last node to the second-to-last group
        last_node = node_groups[-1][0]
        node_groups[-2].append(last_node)
        # Remove the last group (now empty)
        node_groups.pop()

    return node_groups


def output_client_error(client_ret):
    """
    Output client error message based on return code

    Args:
        client_ret (int): Client return code
    """
    log(f"Client failed with exit code: {client_ret}")
    if client_ret == 124:
        log(f"Client timed out")
    elif client_ret == 127:
        log(f"ib_write_bw command not found")
    else:
        log(f"Client execution failed")

def main():
    """
    Main function to run InfiniBand write bandwidth tests
    """
    # Parse command line arguments
    args = parse_args()

    # Access arguments and handle default values
    if args.ib_hca:
        ib_hca_list = args.ib_hca.split(',')
    else:
        # Auto-detect IB devices if not specified
        import glob
        ib_devices = glob.glob('/sys/class/infiniband/*')
        if ib_devices:
            ib_hca_list = [os.path.basename(dev) for dev in sorted(ib_devices)]
            log(f"Auto-detected IB HCA devices: {','.join(ib_hca_list)}")
        else:
            log("ERROR: No IB HCA specified and no InfiniBand devices found in /sys/class/infiniband/")
            sys.exit(1)
    
    socket_ifname = args.socket_ifname
    ib_gid_index = args.ib_gid_index if args.ib_gid_index else '3'
    ssh_cmd = build_ssh_cmd(args.ssh_port)
    nodes = get_hosts(args.nodes_file)
    if len(nodes) < 2:
        print("Error: At least 2 nodes are required.")
        sys.exit(0)

    # Get local IP address from socket interface
    local_ip = get_ip(socket_ifname)
    if not local_ip:
        log(f"failed to get local IP via {socket_ifname}")
        sys.exit(1)

    # SSH parameters for non-interactive connection
    ib_params = ['-s', '16777216', '-n', '50', '-F', '-x', ib_gid_index, '-q', '8', '-b', '--report_gbits']

    # Global list to track failed nodes
    failed_nodes_list = []

    # Split nodes into groups of 16 for concurrent testing
    node_groups = group_nodes(nodes, 16)

    # Process all groups concurrently
    with concurrent.futures.ThreadPoolExecutor(max_workers=len(node_groups)) as executor:
        # Submit all group tasks
        future_to_group = {}
        for group_idx, node_group in enumerate(node_groups):
            log(f"=== Preparing group {group_idx + 1}/{len(node_groups)} ===")
            future = executor.submit(
                test_node_group, local_ip, node_group, ib_hca_list, ib_params, ssh_cmd, group_idx
            )
            future_to_group[future] = group_idx

        # Collect results from all groups
        for future in concurrent.futures.as_completed(future_to_group):
            group_idx = future_to_group[future]
            try:
                failed_nodes = future.result()
                if len(failed_nodes) > 0:
                    failed_nodes_list.extend(failed_nodes)
                log(f"=== Group {group_idx + 1} completed ===")
            except Exception as exc:
                log(f"Group {group_idx + 1} generated an exception: {exc}")

    # === Final Summary ===
    log("=== All tests completed ===")
    if len(failed_nodes_list) == 0:
        log("[RESULT] ✅ all passed, obtained through ib_write_bw")
    else:
        log(f"[RESULT] unhealthy nodes: {failed_nodes_list}, obtained through ib_write_bw")
        sys.exit(1)

if __name__ == "__main__":
    main()