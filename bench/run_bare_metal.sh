#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

set -euo pipefail

# ==== Environment variables ====
export PRIMUSBENCH_PATH=$(pwd)
export IMAGE=${IMAGE:-"primussafe/primusbench:202510210028"}
export INVENTORY_FILE=${INVENTORY_FILE:-"hosts.ini"}
PALYBOOKS="$PRIMUSBENCH_PATH/playbooks"
TIMESTMAP=$(date +'%Y-%m-%d_%H-%M-%S')
PRIMUSBENCH_OUTPUTS="$PRIMUSBENCH_PATH/outputs/$TIMESTMAP"
mkdir -p "$PRIMUSBENCH_OUTPUTS"
PRIMUSBENCH_LOG="$PRIMUSBENCH_OUTPUTS/primusbench.log"

# ==== Colors ====
GREEN="\033[1;32m"
YELLOW="\033[1;33m"
RED="\033[1;31m"
BLUE="\033[1;34m"
RESET="\033[0m"

# ==== Helper functions ====
log() {
    echo -e "${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')]${RESET} $1"
}
ok() {
    echo -e "${GREEN}✔ $1${RESET}"
}
warn() {
    echo -e "${YELLOW}⚠ $1${RESET}"
}
err() {
    echo -e "${RED}✘ $1${RESET}"
}

# ==== Print inventory ====
log "📋 Inventory file: ${YELLOW}$INVENTORY_FILE${RESET}"
cat "$INVENTORY_FILE"
echo

# ==== Install Docker ====
log "🚀 Installing Docker on all nodes..."
ansible-playbook -i "$INVENTORY_FILE" "$PALYBOOKS/bare_metal/install_docker.yaml" \
    --become --become-user=root -vvv \
    > "$PRIMUSBENCH_OUTPUTS/install_docker.log" 2>&1 &
ansible_pid=$!

wait $ansible_pid
install_exit_code=$?
if [ $install_exit_code -eq 0 ]; then
    ok "Docker installation completed successfully!"
else
    err "Docker installation failed! Check log: $PRIMUSBENCH_OUTPUTS/install_docker.log"
    exit $install_exit_code
fi

# ==== Run Benchmark ====
log "🏁 Starting PrimusBench bare metal benchmark..."
ansible-playbook -i "$INVENTORY_FILE" "$PALYBOOKS/bare_metal/bench.yaml" \
    --become --become-user=root \
    -e primus_bench_path="$PRIMUSBENCH_PATH" \
    -e timestmap="$TIMESTMAP" \
    -e image="$IMAGE" \
    -e master_port=$(( RANDOM % 9999 + 30001 )) \
    -e ssh_port=$(( RANDOM % 9999 + 30001 )) \
    -e io_benchmark_mount=IO_BENCHMARK_MOUNT \
    -vvv > "$PRIMUSBENCH_OUTPUTS/bare_metal.log" 2>&1 &
ansible_pid=$!

# Wait for log file to be created
while [ ! -f "$PRIMUSBENCH_LOG" ]; do
    sleep 1
done

log "📜 Streaming benchmark logs..."
tail --pid=$ansible_pid -f "$PRIMUSBENCH_LOG"
wait $ansible_pid
exit_code=$?

echo
if [ $exit_code -eq 0 ]; then
    ok "🎉 Benchmark finished successfully!"
else
    err "Benchmark failed! Check logs under: $PRIMUSBENCH_OUTPUTS"
fi

log "📂 All logs saved in: ${YELLOW}$PRIMUSBENCH_OUTPUTS${RESET}"
exit $exit_code
