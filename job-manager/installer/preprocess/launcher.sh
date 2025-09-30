#!/bin/sh

#
# Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

export NODE_RANK="${PET_NODE_RANK:-${NODE_RANK}}"
export NNODES="${PET_NNODES:-${NNODES}}"

if [[ -f "$PATH_TO_BNXT_TAR_PACKAGE" ]]; then
  echo "Rebuild bnxt from $PATH_TO_BNXT_TAR_PACKAGE ..." && \
  tar xzf "${PATH_TO_BNXT_TAR_PACKAGE}" -C /tmp/ && \
  mv /tmp/libbnxt_re-* /tmp/libbnxt && \
  mv /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so /usr/lib/x86_64-linux-gnu/libibverbs/libbnxt_re-rdmav34.so.inbox && \
  cd /tmp/libbnxt/ && sh ./autogen.sh && ./configure && \
  make -C /tmp/libbnxt clean all install && \
  echo '/usr/local/lib' > /etc/ld.so.conf.d/libbnxt_re.conf && \
  ldconfig && \
  cp -f /tmp/libbnxt/bnxt_re.driver /etc/libibverbs.d/ && \
  cd "${PRIMUS_PATH}" && \
  echo "Rebuild libbnxt done."
else
  echo "Skip bnxt rebuild. PATH_TO_BNXT_TAR_PACKAGE=$PATH_TO_BNXT_TAR_PACKAGE"
fi

echo "$1" |base64 -d > .run.sh
chmod +x .run.sh
if command -v bash >/dev/null 2>&1; then
    /usr/bin/bash -o pipefail .run.sh &
else
    /bin/sh .run.sh &
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