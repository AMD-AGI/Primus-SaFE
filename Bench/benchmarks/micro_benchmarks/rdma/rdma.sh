#!/bin/bash

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

export WORLD_SIZE=${WORLD_SIZE}
export RANK=${RANK}
export MASTER_ADDR=${MASTER_ADDR}
export MASTER_PORT=${MASTER_PORT}
export NCCL_SOCKET_IFNAME=${NCCL_SOCKET_IFNAME:-"eth0"}
export BNIC=${BNIC:-48}
export BXGMI=${BXGMI:-315}
export MAX_RETRY=${MAX_RETRY:-1}
export NCCL_IB_GID_INDEX=${NCCL_IB_GID_INDEX:-3}
export GPUS_PER_NODE=${GPUS_PER_NODE:-8}
export NCCL_TIMEOUT=7200
export TORCH_DISTRIBUTED_DEFAULT_TIMEOUT=$NCCL_TIMEOUT
export GLOO_TIMEOUT=$NCCL_TIMEOUT
export PRIMUSBENCH_PATH=${PRIMUSBENCH_PATH:-$(pwd)}
export PRIMUS_BENCH_OUTPUT_PATH=${PRIMUSBENCH_PATH}/workload_exporter/

NCCL_DEBUG_SUBSYS=ALL
TORCH_NCCL_TRACE_BUFFER_SIZE=10
NCCL_ASYNC_ERROR_HANDLING=1
RDMAV_FORK_SAFE="1"
PYTORCH_HIP_ALLOC_CONF="max_split_size_mb:512"
RCCL_DEBUG=INFO
RCCL_MSCCLPP_ENABLE="0"
TORCH_DISTRIBUTED_DEBUG=DETAIL
NCCL_CHECKS_DISABLE=1
NCCL_IB_GID_INDEX=3
NCCL_CROSS_NIC=0
GPUS_PER_NODE=8
CUDA_DEVICE_MAX_CONNECTIONS=1
NCCL_BLOCKING_WAIT=1
GLOO_SOCKET_IFNAME=eno0
IP_INTERFACE=eno0
NCCL_IB_HCA=rdma0:1,rdma1:1,rdma2:1,rdma3:1,rdma4:1,rdma5:1,rdma6:1,rdma7:1
HIP_VISIBLE_DEVICES=0,1,2,3,4,5,6,7

pip install primus-lens-workload-exporter==0.1.0 && \
mkdir -p \"${PRIMUS_BENCH_OUTPUT_PATH}\" && \
echo 'Starting workload exporter...' && \
nohup python -m primus_lens_workload_exporter.main \
    >> ${PRIMUS_BENCH_OUTPUT_PATH}/workload_exporter.log 2>&1 & \
    disown && \
    echo 'Installing fastapi uvicorn python-multipart websockets httpx...' && \
    pip install fastapi uvicorn python-multipart websockets httpx flash_attn && \
    echo 'Starting scheduler...' && \
    PYTHONPATH=. torchrun \
    --nproc_per_node 8 \
    --nnodes ${NNODES} \
    --node_rank ${NODE_RANK} \
    --master_addr ${MASTER_ADDR} \
    --master_port ${MASTER_PORT} \
    cli.py --sanity --bandwidth \
    2>&1 | tee $PREFLIGHT_LOG && \
    echo '[NODE-${NODE_RANK}(${HOSTNAME})]: end, time=$(date +"%Y.%m.%d %H:%M:%S")'
    " bash "${ARGS[@]}"