#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

# Source unified configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config.sh"

export PRIMUSBENCH_PATH="${PRIMUSBENCH_PATH:-$(pwd)}"
export LOG_HEADER="[$(hostname)] [NODE-$RANK]"

# Convert environment variables to JSON for Ansible
CONTAINER_ENV_JSON=$(python3 -c 'import os, json; print(json.dumps({"container_env": dict(os.environ)}))')

log()    { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"; }
ok()     { echo "✔ $1"; }
warn()   { echo "⚠ $1"; }
err()    { echo "✘ $1"; }

# Send SIGUSR1 signal and wait for background process
send_ready_signal() {
    log "🏁 Benchmarks completed. Synchronizing all nodes... $(date +'%Y-%m-%d %H:%M:%S')"
    kill -USR1 $WAIT_READY_PID
    wait $WAIT_READY_PID
}

# ==== Step 1: Start FIO server ====
if [ -n "${IO_BENCHMARK_MOUNT:-}" ]; then
    log "Starting FIO server..."
    /root/bin/fio --server > /tmp/fio.log 2>&1 &
fi

# ==== Step 2: SSH preflight ====
cd "${PRIMUSBENCH_PATH}/preflight/ssh"
bash run.sh

# ==== Step 3: Start Benchmark ====
log "${LOG_HEADER} [$(date +'%Y-%m-%d %H:%M:%S')] Starting Primus Bench..."
cd "$PRIMUSBENCH_PATH"

if [[ "$RANK" == "0" ]]; then
    # Use SIGUSR1 signal for synchronization
    # Use python3 directly (not torchrun) so signal goes to the Python process
    export USE_SIGNAL=true
    
    CUDA_VISIBLE_DEVICES="" python3 preflight/network/wait_ready.py &
    WAIT_READY_PID=$!
    
    export TIMESTMAP=${TIMESTMAP:-$(date +'%Y-%m-%d_%H-%M-%S')}

    if [ -z "${OUTPUT_PATH:-}" ]; then
        if [ -n "${SHARE_PATH:-}" ]; then
            OUTPUT_PATH="$SHARE_PATH/outputs/$TIMESTMAP"
        else
            OUTPUT_PATH="$PRIMUSBENCH_PATH/outputs/$TIMESTMAP"
        fi
    fi
    mkdir -p "$OUTPUT_PATH"
    log "📂 Output directory: $OUTPUT_PATH"

    # ==== Step 4: IO Benchmarks ====
    if [ -n "${IO_BENCHMARK_MOUNT:-}" ]; then
        log "⚙ Running I/O Benchmarks..."
        io_benchmarks_logname="${OUTPUT_PATH}/io_benchmarks.log"
        IPs=$(awk '{printf (NR==1?$0:","$0)} END{print ""}' "$HOSTS")
        bash "$PRIMUSBENCH_PATH/benchmarks/io_benchmarks/scripts/bench.sh" \
            --mount "$IO_BENCHMARK_MOUNT" \
            --hosts "$IPs" \
            --engine "$ENGINE" \
            --runtime "$RUNTIME" \
            --run_mdtest=1 2>&1 | tee "$io_benchmarks_logname"
        ok "I/O Benchmarks completed."
    fi

    # ==== Step 5: Node checks ====
    PALYBOOKS="$PRIMUSBENCH_PATH/playbooks"
    HOSTS_INI="$OUTPUT_PATH/primusbench_hosts.ini"
    (echo "[all]"; cat "$HOSTS") > "$HOSTS_INI"

    preflight_node_logname="${OUTPUT_PATH}/preflight_node.log"
    log "🔍 Running node preflight check..."
    NODE_CHECK_MASTER_PORT=$((RANDOM % 9999 + 30001))
    ansible-playbook -i "$HOSTS_INI" "$PALYBOOKS/node_check.yaml" \
        -e workspace="$PRIMUSBENCH_PATH" \
        -e hf_token="$HF_TOKEN" \
        -e master_port="$NODE_CHECK_MASTER_PORT" \
        -e output_dir="$OUTPUT_PATH" \
        --extra-vars "$CONTAINER_ENV_JSON" \
        -vvv -f "$WORLD_SIZE" \
        > "$preflight_node_logname" 2>&1 &
    ansible_pid=$!

    TIMEOUT=60
    ELAPSED=0
    NODE_LOG=""
    # Wait for directory and node.log to be created
    while [ -z "$NODE_LOG" ] || [ ! -f "$NODE_LOG" ]; do
        # Check if ansible process is still running
        if ! kill -0 $ansible_pid 2>/dev/null; then
            warn "Ansible process exited before creating log file"
            break
        fi
        # Check timeout
        if [ $ELAPSED -ge $TIMEOUT ]; then
            warn "Timeout waiting for node log file"
            break
        fi
        # Try to find the first directory
        FIRST_DIR=$(find "$OUTPUT_PATH" -maxdepth 1 -type d ! -path "$OUTPUT_PATH" 2>/dev/null | head -1)
        if [ -n "$FIRST_DIR" ]; then
            NODE_LOG="$FIRST_DIR/node.log"
        fi
        sleep 1
        ((ELAPSED++))
    done
    if [ -f "$NODE_LOG" ]; then
        tail -n +1 --pid=$ansible_pid -f "$NODE_LOG"
    fi
    wait $ansible_pid || true

    successed_nodes=()
    successed_nodes_ip=()
    failed_nodes=()
    all_nodes=()
    declare -A node_ip_map ip_node_map
    # Second pass: collect healthy nodes
    while IFS= read -r line; do
        status=true
        if [[ "$line" != *"All checks passed"* ]]; then
            if [[ "$line" != *"[NODE] [ERROR]"* ]]; then
                continue
            fi
            status=false
        fi

        mapfile -t fields < <(grep -oP '\[\K[^\]]+(?=\])' <<< "$line")

        node="${fields[0]}"

        if [[ -n "${node_ip_map[$node]}" ]]; then
            continue
        fi
        ip_addr=$(getent hosts "$node" | awk '{print $1; exit}')

        if [[ "$ip_addr" == 127.* ]]; then
          ip_addr=$(ip route get 8.8.8.8 | awk '{print $7}')
        fi
        node_ip_map[$node]="$ip_addr"
        ip_node_map[$ip_addr]="$node"
        all_nodes+=("$node")
        if $status; then
            successed_nodes+=("$node")
            successed_nodes_ip+=("$ip_addr")
        else
            failed_nodes+=("$node")
        fi
    done < "$preflight_node_logname"

        # Initialize bench report file
    BENCH_REPORT="${OUTPUT_PATH}/bench_report.txt"
    echo "================================================================================" > "$BENCH_REPORT"
    echo "                    PrimusBench Node Check Report" >> "$BENCH_REPORT"
    echo "================================================================================" >> "$BENCH_REPORT"
    echo "Generated at: $(date '+%Y-%m-%d %H:%M:%S')" >> "$BENCH_REPORT"
    echo "================================================================================" >> "$BENCH_REPORT"
    echo "" >> "$BENCH_REPORT"
    # Write failed nodes to report
    echo "Failed Nodes (Node Check) - ${#failed_nodes[@]} nodes" >> "$BENCH_REPORT"
    echo "--------------------------------------------------------------------------------" >> "$BENCH_REPORT"
    if [ ${#failed_nodes[@]} -gt 0 ]; then
        for node in "${failed_nodes[@]}"; do
            nodeIP="${node_ip_map[$node]:-unknown}"
            node_log="$OUTPUT_PATH/$node/node.log"
            if [ -f "$node_log" ]; then
                error=$(grep '\[NODE\] \[ERROR\]:' "$node_log" | sed 's/^\[.*\] \[.*\] \[NODE\] \[ERROR\]: \[[0-9-]* [0-9:]*\] //' | tr '\n' ' | ' | sed 's/ | $//')
            fi
            echo "  $node ($nodeIP): $error" >> "$BENCH_REPORT"
        done
    else
        echo "  -" >> "$BENCH_REPORT"
    fi
    echo "" >> "$BENCH_REPORT"


    if [ ${#successed_nodes[@]} -eq 0 ]; then
        cat "$BENCH_REPORT"
        err "No healthy nodes found, aborting."
        send_ready_signal
        err "PrimusBench failed!"
        exit 1
    fi
    ok "Detected ${#successed_nodes[@]} healthy nodes."

    NETWORK_HOSTS="$OUTPUT_PATH/network_hosts.ini"
    printf "%s\n" "${successed_nodes_ip[@]}" > "$NETWORK_HOSTS"

    preflight_network_logname="${OUTPUT_PATH}/preflight_network.log"
    log "🌐 Running network preflight check..."
    cd "$PRIMUSBENCH_PATH/preflight/network"
    NODES_FILE=$NETWORK_HOSTS \
    WAIT=false \
    bash run.sh 2>&1 | tee $preflight_network_logname
    cd $PRIMUSBENCH_PATH

    # Extract unhealthy nodes from network check
    match=$(grep -oP "unhealthy nodes: \[\K[^\]]+" "$preflight_network_logname" | tail -n1)
    if [[ -n "$match" ]]; then
        unhealthy_nodes=($(echo "$match" | tr -d "'" | tr ',' ' '))
    else
        unhealthy_nodes=()
    fi
    log "Unhealthy nodes detected: ${unhealthy_nodes[*]:-none}"
        
    # Filter out unhealthy nodes from all nodes
    healthy_nodes_ip=()
    if [ ${#unhealthy_nodes[@]} -eq 0 ]; then
        # No unhealthy nodes, all nodes are healthy
        healthy_nodes_ip=("${successed_nodes_ip[@]}")
    else
        # Filter out unhealthy nodes
        for ip in "${successed_nodes_ip[@]}"; do
            is_healthy=true
            for unhealthy_ip in "${unhealthy_nodes[@]}"; do
                if [[ "$ip" == "$unhealthy_ip" ]]; then
                    is_healthy=false
                    break
                fi
            done
            if $is_healthy; then
                healthy_nodes_ip+=("$ip")
            fi
        done
    fi
    ok "Network check complete. Healthy nodes (${#healthy_nodes_ip[@]}/${#all_nodes[@]}): ${healthy_nodes_ip[*]}"
    
    # Write network check results to report
    echo "Failed Nodes (Network Check) - ${#unhealthy_nodes[@]} nodes" >> "$BENCH_REPORT"
    echo "--------------------------------------------------------------------------------" >> "$BENCH_REPORT"
    if [ ${#unhealthy_nodes[@]} -gt 0 ]; then
        for unhealthy_ip in "${unhealthy_nodes[@]}"; do
            unhealthy_node="${ip_node_map[$unhealthy_ip]:-$unhealthy_ip}"
            echo "  $unhealthy_node ($unhealthy_ip)" >> "$BENCH_REPORT"
        done
    else
        echo "  -" >> "$BENCH_REPORT"
    fi
    echo "" >> "$BENCH_REPORT"
    
    # Write healthy nodes to report
    echo "Healthy Nodes (Passed All Checks) - ${#healthy_nodes_ip[@]} nodes" >> "$BENCH_REPORT"
    echo "--------------------------------------------------------------------------------" >> "$BENCH_REPORT"
    if [ ${#healthy_nodes_ip[@]} -gt 0 ]; then
        for ip in "${healthy_nodes_ip[@]}"; do
            healthy_node="${ip_node_map[$ip]}"
            echo "  $healthy_node ($ip)" >> "$BENCH_REPORT"
        done
    else
        echo "  -" >> "$BENCH_REPORT"
    fi
    echo "" >> "$BENCH_REPORT"
    echo "================================================================================" >> "$BENCH_REPORT"
    echo "" >> "$BENCH_REPORT"
    echo "Summary: ${#healthy_nodes_ip[@]} healthy nodes out of ${#all_nodes[@]} total nodes checked" >> "$BENCH_REPORT"
    echo "" >> "$BENCH_REPORT"
    echo "================================================================================" >> "$BENCH_REPORT"
    echo "" >> "$BENCH_REPORT"
    # Exit if no healthy nodes
    if [ ${#healthy_nodes_ip[@]} -eq 0 ]; then
        # Display bench report
        echo ""
        log "📋 Bench Report:"
        echo ""
        cat "$BENCH_REPORT"
        echo ""
        err "No healthy nodes available after network check, aborting."
        send_ready_signal
        err "PrimusBench failed!"
        exit 1
    fi
    
    INVENTORY_FILE="$OUTPUT_PATH/bench_inventory.ini"
    echo "[all]" > $INVENTORY_FILE
    for ip in "${healthy_nodes_ip[@]}"; do
        node=${ip_node_map[$ip]}
        echo "$node ansible_host=$ip" >> $INVENTORY_FILE
    done
    echo "[all:vars]" >> $INVENTORY_FILE
    echo "ansible_ssh_port=${SSH_PORT}" >> $INVENTORY_FILE
    cat $INVENTORY_FILE

    log "🧠 Running Computation-Communication Overlap benchmark..."
    CCO_MASTER_PORT=$((RANDOM % 9999 + 30001))
    cco_logname="$OUTPUT_PATH/cco_ansible.log"
    ansible-playbook -i "$INVENTORY_FILE" "$PALYBOOKS/computation_communication_overlap.yaml" \
        -e workspace="$PRIMUSBENCH_PATH" -e master_port="$CCO_MASTER_PORT" -e output_dir="$OUTPUT_PATH" \
        --extra-vars "$CONTAINER_ENV_JSON" \
        -vvv -f "$WORLD_SIZE" > "$cco_logname" 2>&1 &
    ansible_pid=$!
    first_ip="${healthy_nodes_ip[0]}"
    first_node="${ip_node_map[$first_ip]}"
    echo first_ip=$first_ip first_node=$first_node
    LOG="$OUTPUT_PATH/$first_node/cco.log"
    echo $LOG
    TIMEOUT=120
    ELAPSED=0
    while [ ! -f "$LOG" ]; do
        if ! kill -0 $ansible_pid 2>/dev/null; then
            warn "Ansible process exited before creating log file"
            break
        fi
        if [ $ELAPSED -ge $TIMEOUT ]; then
            warn "Timeout waiting for CCO log file"
            break
        fi
        sleep 1
        ((ELAPSED++))
    done
    if [ -f "$LOG" ]; then
        tail -n +1 --pid=$ansible_pid -f "$LOG"
    fi
    wait $ansible_pid || true
    ok "Computation-Communication benchmark completed."

    log "⚙ Running Kernel Launch Overhead benchmark..."
    kernel_launch_logname="$OUTPUT_PATH/kernel_launch_ansible.log"
    ansible-playbook -i "$INVENTORY_FILE" "$PALYBOOKS/kernel_launch_overhead.yaml" \
        -e output_dir="$OUTPUT_PATH" \
        --extra-vars "$CONTAINER_ENV_JSON" \
        -vvv -f "$WORLD_SIZE" \
        > "$kernel_launch_logname" 2>&1 &
    ansible_pid=$!
    LOG="$OUTPUT_PATH/$first_node/kernel_launch.log"
    TIMEOUT=120
    ELAPSED=0
    while [ ! -f "$LOG" ]; do
        if ! kill -0 $ansible_pid 2>/dev/null; then
            warn "Ansible process exited before creating log file"
            break
        fi
        if [ $ELAPSED -ge $TIMEOUT ]; then
            warn "Timeout waiting for kernel launch log file"
            break
        fi
        sleep 1
        ((ELAPSED++))
    done
    if [ -f "$LOG" ]; then
        tail -n +1 --pid=$ansible_pid -f "$LOG"
    fi
    wait $ansible_pid || true
    ok "Kernel Launch Overhead benchmark completed."

    echo
    log "📊 Computation Communication Overlap results:"
    jq . < "${OUTPUT_PATH}/overlap_results.json"
    echo
    log "📊 Kernel Launch Overhead results:"
    jq . < "${OUTPUT_PATH}/kernel_overhead_results.json"

    ok "✅ PrimusBench completed successfully!"

    # Display bench report
    echo ""
    log "📋 Bench Report:"
    echo ""
    cat "$BENCH_REPORT"
    echo ""
    send_ready_signal
else
    log "${LOG_HEADER} [$(date +'%Y-%m-%d %H:%M:%S')] Waiting for rank 0 to complete bench..."
    CUDA_VISIBLE_DEVICES="" python3 preflight/network/wait_ready.py
fi

ok "✅ PrimusBench completed! $(date +'%Y-%m-%d %H:%M:%S')"


