#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# When this script is container PID 1, reparented zombies (e.g. vLLM workers after python
# exits) must be reaped here. If PID 1 is sh (common with /bin/sh -c exec ...), re-exec
# under bash when available and reap on SIGCHLD; images without bash keep plain sh.
if [ "$$" -eq 1 ] && [ -z "${BASH_VERSION:-}" ]; then
  if [ -x /usr/bin/bash ]; then
    exec /usr/bin/bash "$0" "$@"
  elif [ -x /bin/bash ]; then
    exec /bin/bash "$0" "$@"
  fi
fi
if [ "$$" -eq 1 ] && [ -n "${BASH_VERSION:-}" ]; then
  trap 'while wait -n 2>/dev/null; do :; done' CHLD
fi

input="$1"

export NODE_RANK="${PET_NODE_RANK:-${NODE_RANK}}"
export NNODES="${PET_NNODES:-${NNODES}}"

# Build AINIC driver
if [ -n "${AINIC_DRIVER_VERSION}" ]; then
  /bin/sh /shared-data/build_ainic.sh
  if [ $? -ne 0 ]; then
    echo "ERROR: Failed to build AINIC with driver version ${AINIC_DRIVER_VERSION}. Please check input or remove installation"
    exit 1
  fi
  export USING_AINIC=1
  echo "INFO: AINIC support enabled (USING_AINIC=1)"
fi

# Pensando AINIC: NCCL_IB_TC / NCCL_IB_FIFO_TC (logic in detect_nccl_ib_tc.sh; stdout is eval-safe export lines only).
if [ -f /shared-data/detect_nccl_ib_tc.sh ] && [ -x /bin/sh ]; then
    eval "$(/bin/sh /shared-data/detect_nccl_ib_tc.sh)" || true
fi

/bin/sh /shared-data/build_bnxt.sh
/bin/sh /shared-data/build_authoring.sh

if [ -z "$input" ]; then
    exit 0
fi

echo "$input" |base64 -d > ".run.sh"
chmod +x ".run.sh"
if [ -x /usr/bin/bash ]; then
    /usr/bin/bash -o pipefail ".run.sh" &
elif [ -x /bin/bash ]; then
    /bin/bash -o pipefail ".run.sh" &
else
    /bin/sh ".run.sh" &
fi
pid1=$!

if [ "${ENABLE_SUPERVISE}" = "true" ]; then
    chmod +x "/shared-data/run_check.sh"
    /bin/sh /shared-data/run_check.sh &
    pid2=$!
    
    while true; do
        kill -0 $pid1 2>/dev/null
        if [ $? -ne 0 ]; then
            wait $pid1
            exit_code=$?
            echo "=== LAUNCHER: run.sh exited with code $exit_code ===" >&2
            exit $exit_code
        fi

        if [ -n "$pid2" ]; then
            kill -0 $pid2 2>/dev/null
            if [ $? -ne 0 ]; then
                wait $pid2
                exit_code=$?
                if [ $exit_code -ne 0 ]; then
                    echo "=== LAUNCHER: run_check.sh exited with code $exit_code ===" >&2
                    exit $exit_code
                else
                    pid2=""
                fi
            fi
        fi
        sleep 1
    done
else
    wait $pid1
    exit_code=$?
    echo "=== LAUNCHER: run.sh exited with code $exit_code ===" >&2
    exit $exit_code
fi