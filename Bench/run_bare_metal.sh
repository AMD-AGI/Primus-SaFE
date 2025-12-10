#!/bin/bash
###############################################################################
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
#
# See LICENSE for license information.
###############################################################################

set -euo pipefail

# Source unified configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/config.sh"

export PRIMUSBENCH_PATH="${PRIMUSBENCH_PATH:-$(pwd)}"
PALYBOOKS="$PRIMUSBENCH_PATH/playbooks"
TIMESTMAP=$(date +'%Y-%m-%d_%H-%M-%S')
PRIMUSBENCH_OUTPUTS="$PRIMUSBENCH_PATH/outputs/$TIMESTMAP"
mkdir -p "$PRIMUSBENCH_OUTPUTS"
PRIMUSBENCH_LOG="$PRIMUSBENCH_OUTPUTS/primusbench.log"

# ==== Helper functions ====
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}
ok() {
    echo "✔ $1"
}
warn() {
    echo "⚠ $1"
}
err() {
    echo "✘ $1"
}

# ==== Print inventory ====
log "📋 Inventory file: $INVENTORY_FILE"
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

log "📂 All logs saved in: $PRIMUSBENCH_OUTPUTS"
exit $exit_code
