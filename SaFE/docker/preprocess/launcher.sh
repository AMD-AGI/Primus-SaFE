#!/bin/sh

#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

input="$1"

export NODE_RANK="${PET_NODE_RANK:-${NODE_RANK}}"
export NNODES="${PET_NNODES:-${NNODES}}"
export WORKLOAD_KIND=$WORKLOAD_KIND

# Export variables for build scripts
# AINIC driver: AINIC_DRIVER_VERSION (e.g., 1.117.5-a-56)
export AINIC_DRIVER_VERSION=${AINIC_DRIVER_VERSION}
# BNXT driver: BNXT_DRIVER_VERSION or PATH_TO_BNXT_TAR_PACKAGE
export BNXT_DRIVER_VERSION=${BNXT_DRIVER_VERSION}
export PATH_TO_BNXT_TAR_PACKAGE=${PATH_TO_BNXT_TAR_PACKAGE}

/bin/sh /shared-data/build_ainic.sh
/bin/sh /shared-data/build_bnxt.sh
/bin/sh /shared-data/build_ssh.sh

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