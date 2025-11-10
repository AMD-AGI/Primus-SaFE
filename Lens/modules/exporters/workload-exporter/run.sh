#!/usr/bin/env bash
set -euo pipefail

if [ -n "${SLURM_NODEID:-}" ]; then
    export NODE_RANK="$SLURM_NODEID"
elif [ -n "${PET_NODE_RANK:-}" ]; then
    export NODE_RANK="$PET_NODE_RANK"
elif [ -n "${NODE_RANK:-}" ]; then
    export NODE_RANK="$NODE_RANK"
else
    export NODE_RANK=0
fi

echo "Running on NODE_RANK=${NODE_RANK}"

python3 -m primus_lens_workload_exporter.main "$@"
