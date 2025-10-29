#!/usr/bin/env bash
set -euo pipefail

# -----------------------------
# 1. 设置 NODE_RANK
# -----------------------------
# SLURM 下
if [ -n "${SLURM_NODEID:-}" ]; then
    export NODE_RANK="$SLURM_NODEID"
# PyTorchJob 下（假设使用常见的环境变量 WORLD_RANK 或 NODE_RANK）
elif [ -n "${PET_NODE_RANK:-}" ]; then
    export NODE_RANK="$PET_NODE_RANK"
elif [ -n "${NODE_RANK:-}" ]; then
    # 已经有 NODE_RANK 就直接用
    export NODE_RANK="$NODE_RANK"
else
    # 默认 0
    export NODE_RANK=0
fi

echo "Running on NODE_RANK=${NODE_RANK}"

# -----------------------------
# 2. 启动 Python 程序
# -----------------------------
# 假设你的模块路径是 mycollector
python3 -m primus_lens_workload_exporter.main "$@"
