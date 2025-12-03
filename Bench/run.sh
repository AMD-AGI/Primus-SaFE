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

# ==== Output styles ====
GREEN="\033[1;32m"
YELLOW="\033[1;33m"
RED="\033[1;31m"
BLUE="\033[1;34m"
MAGENTA="\033[1;35m"
RESET="\033[0m"

log()    { echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')]${RESET} $1"; }
ok()     { echo -e "${GREEN}✔ $1${RESET}"; }
warn()   { echo -e "${YELLOW}⚠ $1${RESET}"; }
err()    { echo -e "${RED}✘ $1${RESET}"; }

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
    log "📂 Output directory: ${YELLOW}$OUTPUT_PATH${RESET}"

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
    ansible-playbook -i "$HOSTS_INI" "$PALYBOOKS/node_check.yaml" \
        -e workspace="$PRIMUSBENCH_PATH"  -e hf_token="$HF_TOKEN"  -vvv -f "$WORLD_SIZE" \
        > "$preflight_node_logname" 2>&1 &
    ansible_pid=$!

    NODE_LOG="/tmp/node.log"
    while [ ! -f "$NODE_LOG" ]; do sleep 1; done
    tail --pid=$ansible_pid -f "$NODE_LOG"
    wait $ansible_pid || true

    nodes=()
    nodes_ip=()
    declare -A node_ip_map ip_node_map

    while IFS= read -r line; do
        if [[ "$line" == *"All check passed\""* ]]; then
            node=$(echo "$line" | grep -oP '(?<=\[)[^]]+(?=\])' | head -n1)
            ip_addr=$(getent hosts "$node" | awk '{print $1; exit}')
            [[ $ip_addr == 127.* ]] && ip_addr=$(ip route get 8.8.8.8 | awk '{print $7}')
            node_ip_map[$node]=$ip_addr
            ip_node_map[$ip_addr]=$node
            nodes+=("$node")
            nodes_ip+=("$ip_addr")
        fi
    done < "$preflight_node_logname"

    if [ ${#nodes[@]} -eq 0 ]; then
        err "No healthy nodes found, aborting."
        exit 1
    fi
    ok "Detected ${#nodes[@]} healthy nodes."

    NETWORK_HOSTS="$PRIMUSBENCH_PATH/network_hosts.ini"
    printf "%s\n" "${nodes_ip[@]}" > "$NETWORK_HOSTS"

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
    log "Unhealthy nodes detected: ${YELLOW}${unhealthy_nodes[*]:-none}${RESET}"
    
    # Filter out unhealthy nodes from all nodes
    healthy_nodes_ip=()
    if [ ${#unhealthy_nodes[@]} -eq 0 ]; then
        # No unhealthy nodes, all nodes are healthy
        healthy_nodes_ip=("${nodes_ip[@]}")
    else
        # Filter out unhealthy nodes
        for ip in "${nodes_ip[@]}"; do
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
    ok "Network check complete. Healthy nodes (${#healthy_nodes_ip[@]}/${#nodes_ip[@]}): ${healthy_nodes_ip[*]}"
    
    # Exit if no healthy nodes
    if [ ${#healthy_nodes_ip[@]} -eq 0 ]; then
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
fi

log "${LOG_HEADER} [$(date +'%Y-%m-%d %H:%M:%S')] Waiting for rank 0 to complete bench..."
CUDA_VISIBLE_DEVICES="" torchrun \
    --nproc_per_node=1 \
    --nnodes=$WORLD_SIZE \
    --node_rank=$RANK \
    --master_addr=$MASTER_ADDR \
    --master_port=$MASTER_PORT \
    preflight/network/wait_ready.py

ok "✅ PrimusBench completed!"
