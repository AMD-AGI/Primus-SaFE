#!/bin/bash

ulimit -n 65536
ulimit -u 8192

if [ -n "$SSH_PORT" ] && [ "$SSH_PORT" -gt 0 ]; then
    /bin/bash /shared-data/build_ssh.sh
fi

echo "$1" |base64 -d > .run.sh
/bin/sh .run.sh &
pid1=$!

if [ "${ENABLE_SUPERVISE}" == "true" ]; then
    /bin/bash /shared-data/run_scripts.sh &
    pid2=$!
    wait -n $pid2 $pid1
else
    wait -n $pid1
fi

exit $?