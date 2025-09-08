#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Check input arguments
if [ "$#" -lt 4 ]; then
  echo "Usage: $0 <ib_hca_list> <socket_ifname> <ib_gid_index> <nodes_file>"
  echo "Example: $0 'bnxt_re0,bnxt_re1' eth0 3 nodes.txt"
  exit 1
fi

# Parse comma-separated list of IB devices
IFS=',' read -ra IB_HCA_LIST <<< "$1"
ib_gid_index=$3
nodes_file=$4

# Validate nodes file
if [ ! -f "$nodes_file" ]; then
  echo "ERROR: $nodes_file does not exist"
  exit 1
fi

# Get local IP address from socket interface
get_ip() {
    local interface=$1
    ifconfig "$interface" | grep -oE 'inet [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | awk '{print $2}' | head -n1
}
local_ip=$(get_ip "$2")
if [ -z "$local_ip" ]; then
  echo "ERROR: failed to get local IP via $2"
  exit 1
fi

# Use the first node in the file as the client (initiator)
export CLIENT=$(awk 'NR==1{print $1}' "$nodes_file")
echo "=== Starting ib_write_bw tests ==="
echo "Client initiator: $CLIENT"
echo "LocalIp = '$local_ip'"
echo "Node list: $(paste -sd ',' "$nodes_file")"
echo

# SSH parameters for non-interactive connection
ssh_params="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10"
ib_write_bw="/opt/rdma-perftest/bin/ib_write_bw"
ib_params="-s 16777216 -n 50 -F -x $ib_gid_index -q 4 --report_gbits"

# Function: Kill remote ib_write_bw process
kill_remote_listener() {
  local node=$1
  local dev=$2
  echo "[$node] Cleaning up remote ib_write_bw process..."
  ssh $ssh_params "$node" "pkill -f 'ib_write_bw.*-d $dev' || true" &
}

# Function: Check if ib_write_bw exists and device is present on remote node
check_remote_cmd() {
  local node=$1
  local dev=$2
  ssh $ssh_params "$node" "command -v $ib_write_bw >/dev/null 2>&1 && [ -e /sys/class/infiniband/$dev ]"
  if [ $? -ne 0 ]; then
    echo "ERROR: $ib_write_bw not found or device $dev not present on $node"
    return 1
  fi
  return 0
}

# Global map to track failed nodes (avoid retesting)
declare -A FAILED_NODES_MAP
# Ordered list to preserve output order
FAILED_NODES_LIST=()

# Main loop: iterate over each IB device
for ib_hca in "${IB_HCA_LIST[@]}"; do
  echo "=================================================="
  echo "[$(date +%H:%M:%S)] TESTING IB DEVICE: $ib_hca"
  echo "=================================================="

  # Read nodes from file, skip empty lines
  for node in $(awk '{print $1}' "$nodes_file")
  do
    [ -z "$node" ] && { echo "Skipping empty line"; continue; }
    # Skip if this node has already failed in a previous device test
    if [[ -n "${FAILED_NODES_MAP[$node]}" ]]; then
      echo "[$node] Skipping: previously failed"
      continue
    fi

    echo "--------------------------------------------------"
    echo "[$(date +%H:%M:%S)] TESTING: $node on $ib_hca"
    echo "--------------------------------------------------"

    REMOTE_LISTENER=false

    # === STEP 1: Start Server (Listener) ===
    if [[ "$node" == "$local_ip" ]]; then
      # Local server (no SSH)
      echo "[$node] Starting LOCAL server (no SSH)..."
      $ib_write_bw -d "$ib_hca" $ib_params &
      server_pid=$!
      echo "[$node] Local server started, PID: $server_pid"
      sleep 5

      # Verify server process is still running
      if ! kill -0 $server_pid 2>/dev/null; then
        echo "ERROR: Local server failed to start!"
        if [[ -z "${FAILED_NODES_MAP[$node]}" ]]; then
          FAILED_NODES_MAP[$node]=1
          FAILED_NODES_LIST+=("$node")
        fi
        continue
      fi
    else
      # Remote server via SSH
      echo "[$node] Starting REMOTE server via SSH..."
      if ! check_remote_cmd "$node" "$ib_hca"; then
        echo "[$node] Skipping test due to missing ib_write_bw or device"
        if [[ -z "${FAILED_NODES_MAP[$node]}" ]]; then
          FAILED_NODES_MAP[$node]=1
          FAILED_NODES_LIST+=("$node")
        fi
        continue
      fi

      ssh $ssh_params "$node" "$ib_write_bw -d $ib_hca $ib_params" &
      server_pid=$!
      echo "[$node] Remote server started via SSH, SSH PID: $server_pid"
      REMOTE_LISTENER=true
      sleep 5
    fi

    # === STEP 2: Start Client ===
    echo "[$node] Starting client from $CLIENT to $node..."

    if [[ "$local_ip" == "$CLIENT" ]]; then
      # Run client locally
      output=$(timeout 40s $ib_write_bw -d "$ib_hca" $ib_params "$node" 2>&1)
      client_ret=$?
    else
      # Run client via SSH on the designated client node
      output=$(timeout 40s ssh $ssh_params "$CLIENT" "$ib_write_bw -d $ib_hca $ib_params $node" 2>&1)
      client_ret=$?
    fi

    # === STEP 3: Show Debug Output ===
    echo "[$node] CLIENT OUTPUT:"
    echo "$output" | sed 's/^/    > &/'
    echo

    # === STEP 4: Cleanup Server ===
    if [[ "$REMOTE_LISTENER" == "true" ]]; then
      kill_remote_listener "$node" "$ib_hca"
      sleep 1
    else
      kill $server_pid 2>/dev/null || true
      wait $server_pid 2>/dev/null || true
    fi

    # === STEP 5: Determine PASS/FAIL ===
    if [ $client_ret -eq 0 ] && echo "$output" | grep -q "Gb/sec"; then
      echo "RESULT: $node on $ib_hca PASSES"
    else
      echo "RESULT: $node on $ib_hca FAILS (exit code: $client_ret)"
      # Highlight common errors
      echo "$output" | grep -E "(Couldn't connect|No route|Device not found|Permission denied|timeout)" | sed 's/^/    [ERROR] &/'
      if [[ -z "${FAILED_NODES_MAP[$node]}" ]]; then
        FAILED_NODES_MAP[$node]=1
        FAILED_NODES_LIST+=("$node")
      fi
    fi

    sleep 2
  done
  echo
done

# === Final Summary ===
echo "=== All tests completed ==="
if [ ${#FAILED_NODES_LIST[@]} -eq 0 ]; then
  echo "[SUCCESS] ✅ all passed, obtained through ib_write_bw"
else
  printf '[ERROR] unhealthy nodes: ['
  for i in "${!FAILED_NODES_LIST[@]}"; do
    printf "'%s'" "${FAILED_NODES_LIST[i]}"
    if [ $i -lt $((${#FAILED_NODES_LIST[@]} - 1)) ]; then
      printf ", "
    fi
  done
  printf "], obtained through ib_write_bw\n"
  exit 1
fi