#!/bin/bash
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Unified environment configuration for PrimusBench
# Source this file in all entry scripts: source config.sh

# ==============================================================================
# SLURM Configuration (Override in run_slurm.sh or user env as needed)
export NNODES="${NNODES:-2}"
# export EXCLUDE_NODES="chi[2742,2815-2817]"
# export NODELIST="${NODELIST:-chi[2770-2772]}"

export TIME="${TIME:-4:30:00}"
export PARTITION="${PARTITION:-mi355x}"
export CPUS_PER_TASK="${CPUS_PER_TASK:-128}"
export USE_SALLOC="${USE_SALLOC:-true}"

# ==============================================================================
# Container Configuration
# ==============================================================================
export IMAGE="${IMAGE:-docker.io/primussafe/primusbench:rocm7.0.3_gfx950_ainic_202512281440}"
export ENABLE_IMAGE_WARMUP="${ENABLE_IMAGE_WARMUP:-true}"
# ==============================================================================
# Cluster Configuration
# ==============================================================================

export GPUS_PER_NODE="${GPUS_PER_NODE:-8}"
export MASTER_ADDR="${MASTER_ADDR:-localhost}"
export MASTER_PORT="${MASTER_PORT:-12345}"
export SSH_PORT="${SSH_PORT:-22366}"

# ==============================================================================
# Network Interface Configuration
# ==============================================================================
export IP_INTERFACE="${IP_INTERFACE:-enp193s0f0np0}"
export NCCL_SOCKET_IFNAME="${NCCL_SOCKET_IFNAME:-${IP_INTERFACE}}"
export GLOO_SOCKET_IFNAME="${GLOO_SOCKET_IFNAME:-${IP_INTERFACE}}"
export NCCL_IB_HCA="${NCCL_IB_HCA:-"ionic_0,ionic_1,ionic_2,ionic_3,ionic_4,ionic_5,ionic_6,ionic_7"}"
export ENABLE_AINIC="${ENABLE_AINIC:-true}"

# ==============================================================================
# GPU Configuration (MI300X/MI325X/MI355X)
# ==============================================================================
export GPU_PRODUCT="${GPU_PRODUCT:-MI355X}"
export HSA_ENABLE_SDMA="${HSA_ENABLE_SDMA:-1}"
export HSA_NO_SCRATCH_RECLAIM="${HSA_NO_SCRATCH_RECLAIM:-1}"

# ==============================================================================
# NCCL/RCCL Configuration
# ==============================================================================
export NCCL_TIMEOUT="${NCCL_TIMEOUT:-7200}"
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT="${TORCH_DISTRIBUTED_DEFAULT_TIMEOUT:-${NCCL_TIMEOUT}}"
export GLOO_TIMEOUT="${GLOO_TIMEOUT:-${NCCL_TIMEOUT}}"

export NCCL_IB_GID_INDEX="${NCCL_IB_GID_INDEX:-1}"
export NCCL_CROSS_NIC="${NCCL_CROSS_NIC:-0}"
export NCCL_IB_GDR_LEVEL="${NCCL_IB_GDR_LEVEL:-2}"
export NCCL_NET_GDR_LEVEL="${NCCL_NET_GDR_LEVEL:-2}"
export NCCL_CHECKS_DISABLE="${NCCL_CHECKS_DISABLE:-1}"
export NCCL_DEBUG="${NCCL_DEBUG:-VERSION}"
export RCCL_MSCCL_ENABLE="${RCCL_MSCCL_ENABLE:-0}"
export NCCL_IB_TIMEOUT=23  
export NCCL_IB_RETRY_CNT=11  

# ==============================================================================
# Torch/CUDA Configuration
# ==============================================================================
export CUDA_DEVICE_MAX_CONNECTIONS="${CUDA_DEVICE_MAX_CONNECTIONS:-1}"
export TORCH_NCCL_HIGH_PRIORITY="${TORCH_NCCL_HIGH_PRIORITY:-1}"
export OMP_NUM_THREADS="${OMP_NUM_THREADS:-1}"
export GPU_MAX_HW_QUEUES="${GPU_MAX_HW_QUEUES:-2}"

# ==============================================================================
# Benchmark Configuration
# ==============================================================================
export ENGINE="${ENGINE:-psync}"
export RUNTIME="${RUNTIME:-30}"
export BNIC="${BNIC:-50}"
export BXGMI="${BXGMI:-315}"

# ==============================================================================
# Path Configuration
# ==============================================================================
export PRIMUSBENCH_PATH="${PRIMUSBENCH_PATH:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
export LOG_DIR="${LOG_DIR:-${PRIMUSBENCH_PATH}/outputs}"
export INVENTORY_FILE="${INVENTORY_FILE:-hosts.ini}"
export HOSTS="${HOSTS:-/root/hosts}"

# ==============================================================================
# Optional: HuggingFace Token (required for some benchmarks)
# ==============================================================================

export HF_TOKEN="${HF_TOKEN:-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}"

# ==============================================================================
# Docker/Container Options
# ==============================================================================
export CLEAN_DOCKER_CONTAINER="${CLEAN_DOCKER_CONTAINER:-1}"
export ADD_LOG_HEADER="${ADD_LOG_HEADER:-true}"


# ==============================================================================
# Helper function to print configuration
# ==============================================================================
print_config() {
    echo "================================================================================"
    echo "                    PrimusBench Configuration"
    echo "================================================================================"
    echo "Container:"
    echo "  IMAGE:                  $IMAGE"
    echo ""
    echo "Cluster:"
    echo "  NNODES:                 $NNODES"
    echo "  GPUS_PER_NODE:          $GPUS_PER_NODE"
    echo "  MASTER_ADDR:            $MASTER_ADDR"
    echo "  MASTER_PORT:            $MASTER_PORT"
    echo "  SSH_PORT:               $SSH_PORT"
    if [ -n "$NODELIST" ]; then
        echo "  NODELIST:               $NODELIST"
    fi
    echo ""
    echo "Network:"
    echo "  IP_INTERFACE:           $IP_INTERFACE"
    echo "  NCCL_SOCKET_IFNAME:     $NCCL_SOCKET_IFNAME"
    echo "  GLOO_SOCKET_IFNAME:     $GLOO_SOCKET_IFNAME"
    echo ""
    echo "GPU:"
    echo "  GPU_PRODUCT:            $GPU_PRODUCT"
    echo ""
    echo "Paths:"
    echo "  PRIMUSBENCH_PATH:       $PRIMUSBENCH_PATH"
    echo "  LOG_DIR:                $LOG_DIR"
    echo ""
    echo "Container Warmup:"
    echo "  ENABLE_IMAGE_WARMUP:    $ENABLE_IMAGE_WARMUP"
    echo "================================================================================"
}

