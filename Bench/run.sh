#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

export NCCL_TIMEOUT=7200
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=$NCCL_TIMEOUT
export GLOO_TIMEOUT=$NCCL_TIMEOUT
export PRIMUSBENCH_PATH=$(pwd)
export LOG_HEADER="[$(hostname)] [NODE-$RANK]"
export HF_TOKEN=${HF_TOKEN}

HOSTS=/root/hosts
ENGINE="${ENGINE:-psync}"
RUNTIME="${RUNTIME:-30}"

log()    { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"; }
ok()     { echo "✔ $1"; }
warn()   { echo "⚠ $1"; }
err()    { echo "✘ $1"; }

# ==== Step 1: Start FIO server ====
if [ -n "${IO_BENCHMARK_MOUNT:-}" ]; then
    log "Starting FIO server..."
    /root/bin/fio --server > /tmp/fio.log 2>&1 &
fi

# ==== Step 2: SSH preflight ====
export SSH_PORT=${SSH_PORT:-22366}
cd "${PRIMUSBENCH_PATH}/preflight/ssh"
bash run.sh

# ==== Step 3: Start Benchmark ====
log "${LOG_HEADER} [$(date +'%Y-%m-%d %H:%M:%S')] Starting Primus Bench..."
cd "$PRIMUSBENCH_PATH"

if [[ "$RANK" == "0" ]]; then
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
    HOSTS_INI="primusbench_hosts.ini"
    (echo "[all]"; cat "$HOSTS") > "$HOSTS_INI"

    preflight_node_logname="${OUTPUT_PATH}/preflight_node.log"
    log "🔍 Running node preflight check..."
    NODE_CHECK_MASTER_PORT=$((RANDOM % 9999 + 30001))
    ansible-playbook -i "$HOSTS_INI" "$PALYBOOKS/node_check.yaml" \
        -e workspace="$PRIMUSBENCH_PATH"  -e hf_token="$HF_TOKEN" -e master_port="$NODE_CHECK_MASTER_PORT"  -vvv -f "$WORLD_SIZE" \
        > "$preflight_node_logname" 2>&1 &
    ansible_pid=$!

    NODE_LOG="/tmp/node.log"
    while [ ! -f "$NODE_LOG" ]; do sleep 1; done
    tail --pid=$ansible_pid -f "$NODE_LOG"
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



    if [ ${#successed_nodes[@]} -eq 0 ]; then
        err "No healthy nodes found, aborting."
        CUDA_VISIBLE_DEVICES="" torchrun \
        --nproc_per_node=1 \
        --nnodes=$WORLD_SIZE \
        --node_rank=$RANK \
        --master_addr=$MASTER_ADDR \
        --master_port=$MASTER_PORT \
        preflight/network/wait_ready.py
        err "PrimusBench failed!"
        exit 1
    fi
    ok "Detected ${#successed_nodes[@]} healthy nodes."

    NETWORK_HOSTS="$PRIMUSBENCH_PATH/network_hosts.ini"
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
    

    # Initialize bench report file
    BENCH_REPORT="${OUTPUT_PATH}/bench_report.txt"
    echo "================================================================================" > "$BENCH_REPORT"
    echo "                    PrimusBench Node Check Report" >> "$BENCH_REPORT"
    echo "================================================================================" >> "$BENCH_REPORT"
    echo "Generated at: $(date '+%Y-%m-%d %H:%M:%S')" >> "$BENCH_REPORT"
    echo "" >> "$BENCH_REPORT"
    echo "Summary: ${#healthy_nodes_ip[@]} healthy nodes out of ${#all_nodes[@]} total nodes checked" >> "$BENCH_REPORT"
    echo "" >> "$BENCH_REPORT"
    echo "================================================================================" >> "$BENCH_REPORT"
    echo "" >> "$BENCH_REPORT"
    # Write failed nodes to report
    echo "Failed Nodes (Node Check) - ${#failed_nodes[@]} nodes" >> "$BENCH_REPORT"
    echo "--------------------------------------------------------------------------------" >> "$BENCH_REPORT"
    if [ ${#failed_nodes[@]} -gt 0 ]; then
        for node in "${failed_nodes[@]}"; do
            nodeIP="${node_ip_map[$node]:-unknown}"
            echo "  $node ($nodeIP)" >> "$BENCH_REPORT"
        done
    else
        echo "  -" >> "$BENCH_REPORT"
    fi
    echo "" >> "$BENCH_REPORT"

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
    
    # Exit if no healthy nodes
    if [ ${#healthy_nodes_ip[@]} -eq 0 ]; then
        # Display bench report
        echo ""
        log "📋 Bench Report:"
        echo ""
        cat "$BENCH_REPORT"
        echo ""
        err "No healthy nodes available after network check, aborting."
        CUDA_VISIBLE_DEVICES="" torchrun \
        --nproc_per_node=1 \
        --nnodes=$WORLD_SIZE \
        --node_rank=$RANK \
        --master_addr=$MASTER_ADDR \
        --master_port=$MASTER_PORT \
        preflight/network/wait_ready.py
        err "PrimusBench failed!"
        exit 1
    fi
    
    INVENTORY_FILE="bench_inventory.ini"
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
        -vvv -f "$WORLD_SIZE" > "$cco_logname" 2>&1 &
    ansible_pid=$!
    first_ip="${healthy_nodes_ip[0]}"
    first_node="${ip_node_map[$first_ip]}"
    echo first_ip=$first_ip first_node=$first_node
    LOG="$OUTPUT_PATH/$first_node/cco.log"
    echo $LOG
    while [ ! -f "$LOG" ]; do sleep 1; done
    tail --pid=$ansible_pid -f "$LOG"
    wait $ansible_pid || true
    ok "Computation-Communication benchmark completed."

    log "⚙ Running Kernel Launch Overhead benchmark..."
    kernel_launch_logname="$OUTPUT_PATH/kernel_launch_ansible.log"
    ansible-playbook -i "$INVENTORY_FILE" "$PALYBOOKS/kernel_launch_overhead.yaml" \
        -e output_dir="$OUTPUT_PATH" -vvv -f "$WORLD_SIZE" \
        > "$kernel_launch_logname" 2>&1 &
    ansible_pid=$!
    LOG="$OUTPUT_PATH/$first_node/kernel_launch.log"
    while [ ! -f "$LOG" ]; do sleep 1; done
    tail --pid=$ansible_pid -f "$LOG"
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
else
    log "${LOG_HEADER} [$(date +'%Y-%m-%d %H:%M:%S')] Waiting for rank 0 to complete bench..."
fi

CUDA_VISIBLE_DEVICES="" torchrun \
    --nproc_per_node=1 \
    --nnodes=$WORLD_SIZE \
    --node_rank=$RANK \
    --master_addr=$MASTER_ADDR \
    --master_port=$MASTER_PORT \
    preflight/network/wait_ready.py

ok "✅ PrimusBench completed!"

