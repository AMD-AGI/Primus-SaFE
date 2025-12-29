#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

export WORLD_SIZE=${WORLD_SIZE}
export RANK=${RANK}
export MASTER_ADDR=${MASTER_ADDR}
export MASTER_PORT=${MASTER_PORT}
export NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME:-"eno0"}
export BNIC=${BNIC:-48}
export BXGMI=${BXGMI:-315}
export MAX_RETRY=${MAX_RETRY:-1}
export NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX:-3}
export GPUS_PER_NODE=${GPUS_PER_NODE:-8}
export NCCL_TIMEOUT=7200
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=$NCCL_TIMEOUT
export GLOO_TIMEOUT=$NCCL_TIMEOUT

export NCCL_DEBUG_SUBSYS=${NCCL_DEBUG_SUBSYS:-ALL}
export TORCH_NCCL_TRACE_BUFFER_SIZE=${TORCH_NCCL_TRACE_BUFFER_SIZE:-10}
export NCCL_ASYNC_ERROR_HANDLING=${NCCL_ASYNC_ERROR_HANDLING:-1}
export RDMAV_FORK_SAFE=${RDMAV_FORK_SAFE:-1}
export PYTORCH_HIP_ALLOC_CONF=${PYTORCH_HIP_ALLOC_CONF:-"max_split_size_mb:512"}
export RCCL_DEBUG=${RCCL_DEBUG:-INFO}
export RCCL_MSCCLPP_ENABLE=${RCCL_MSCCLPP_ENABLE:-0}
export TORCH_DISTRIBUTED_DEBUG=${TORCH_DISTRIBUTED_DEBUG:-DETAIL}
export NCCL_CHECKS_DISABLE=${NCCL_CHECKS_DISABLE:-1}
export NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX:-3}
export NCCL_CROSS_NIC=${NCCL_CROSS_NIC:-0}
export GPUS_PER_NODE=${GPUS_PER_NODE:-8}
export CUDA_DEVICE_MAX_CONNECTIONS=${CUDA_DEVICE_MAX_CONNECTIONS:-1}
export NCCL_BLOCKING_WAIT=${NCCL_BLOCKING_WAIT:-1}
export GLOO_SOCKET_IFNAME=${GLOO_SOCKET_IFNAME:-eno0}
export IP_INTERFACE=${IP_INTERFACE:-eno0}
export NCCL_IB_HCA=${NCCL_IB_HCA:-rdma0:1,rdma1:1,rdma2:1,rdma3:1,rdma4:1,rdma5:1,rdma6:1,rdma7:1}
export HIP_VISIBLE_DEVICES=${HIP_VISIBLE_DEVICES:-0,1,2,3,4,5,6,7}

# AINIC (AMD Network Plugin) configuration
export ENABLE_AINIC=${ENABLE_AINIC:-false}
if [[ "$ENABLE_AINIC" == "true" ]]; then
    echo "Configuring for AINIC (AMD Network Plugin)..."
    # Update LD_LIBRARY_PATH for ANP
    export LD_LIBRARY_PATH="/opt/amd-anp/build:/opt/rccl/build/release:${LD_LIBRARY_PATH}"
    # AINIC specific NCCL settings
    export NCCL_NET_GDR_LEVEL=2
    export NCCL_NET_GDR_READ=1
    export NCCL_PXN_DISABLE=0
    export NCCL_DMABUF_ENABLE=0
    export NCCL_GDR_FLUSH_DISABLE=1
    export NCCL_IGNORE_CPU_AFFINITY=1
    export NCCL_IB_QPS_PER_CONNECTION=1
    # Use socket interface for UCX TCP when AINIC is enabled
    export UCX_NET_DEVICES=${NCCL_SOCKET_IFNAME}
    # Additional AINIC optimizations
    export NCCL_IB_DISABLE=0
    export NCCL_IB_PCI_RELAXED_ORDERING=1
    export NCCL_SHM_DISABLE=1
else
    # Standard configuration without AINIC
    export NCCL_NET_GDR_LEVEL=${NCCL_NET_GDR_LEVEL:-2}
    export NCCL_NET_GDR_READ=${NCCL_NET_GDR_READ:-1}
    # Use IB HCA for UCX when AINIC is disabled
    # Extract first device from NCCL_IB_HCA
    IB_HCA_FIRST=$(echo $NCCL_IB_HCA | cut -d',' -f1)
    # Check if port suffix is already present (format: device:port)
    if [[ "$IB_HCA_FIRST" == *":"* ]]; then
        # Already has port suffix, use as-is
        export UCX_NET_DEVICES=${UCX_NET_DEVICES:-"${IB_HCA_FIRST}"}
    else
        # No port suffix, add default port 1 for UCX
        export UCX_NET_DEVICES=${UCX_NET_DEVICES:-"${IB_HCA_FIRST}:1"}
    fi
    # Standard IB settings
    export NCCL_IB_DISABLE=${NCCL_IB_DISABLE:-0}
    export NCCL_IB_PCI_RELAXED_ORDERING=${NCCL_IB_PCI_RELAXED_ORDERING:-1}
    export NCCL_SHM_DISABLE=${NCCL_SHM_DISABLE:-1}
fi

echo WORLD_SIZE=$WORLD_SIZE RANK=$RANK MASTER_ADDR=$MASTER_ADDR \
    MASTER_PORT=$MASTER_PORT NCCL_SOCKET_IFNAME=$NCCL_SOCKET_IFNAME NCCL_IB_HCA=$NCCL_IB_HCA \
    ENABLE_AINIC=$ENABLE_AINIC

    
torchrun --nproc_per_node=$GPUS_PER_NODE \
    --nnodes=$WORLD_SIZE \
    --node_rank=$RANK \
    --master_addr=$MASTER_ADDR \
    --master_port=$MASTER_PORT \
    computation-communication-overlap.py