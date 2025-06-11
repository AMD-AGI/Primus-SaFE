#!/bin/sh

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

ulimit -n 65536

if [ -n "$SSH_PORT" ] && [ "$SSH_PORT" -gt 0 ]; then
    /bin/bash /shared-data/build_ssh.sh
fi

echo "$1" |base64 -d > .run.sh
chmod +x .run.sh
/bin/sh .run.sh &
pid1=$!

if [ "${ENABLE_SUPERVISE}" = "true" ]; then
    chmod +x "/shared-data/run_check.sh"
    /bin/sh /shared-data/run_check.sh &
    pid2=$!
    
    while true; do
        kill -0 $pid1 2>/dev/null
        if [ $? -ne 0 ]; then
            wait $pid1
            exit $?
        fi

        if [ -n "$pid2" ]; then
            kill -0 $pid2 2>/dev/null
            if [ $? -ne 0 ]; then
                wait $pid2
                exit_code=$?
                if [ $exit_code -ne 0 ]; then
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
    exit $?
fi