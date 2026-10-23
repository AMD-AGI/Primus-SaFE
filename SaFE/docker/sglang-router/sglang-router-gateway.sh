#!/bin/bash
# Run the SGLang model gateway on loopback and the Anthropic front on the public port.
# Usage: sglang-router-gateway [--port N] [--upstream-port N] -- <router command>
# The router command must listen on 127.0.0.1:<upstream-port>.
# The container exits when either process exits.

set -euo pipefail

port=8000
upstream_port=8001
while [[ $# -gt 0 ]]; do
    case "$1" in
        --port) port="$2"; shift 2 ;;
        --upstream-port) upstream_port="$2"; shift 2 ;;
        --) shift; break ;;
        *) echo "unknown argument: $1" >&2; exit 2 ;;
    esac
done
if [[ $# -eq 0 ]]; then
    echo "usage: $0 [--port N] [--upstream-port N] -- <router command>" >&2
    exit 2
fi

"$@" &
router_pid=$!
python3 /opt/sglang-router/anthropic_proxy.py \
    --host 0.0.0.0 --port "${port}" --upstream "http://127.0.0.1:${upstream_port}" &
proxy_pid=$!

trap 'kill "${router_pid}" "${proxy_pid}" 2>/dev/null || true' TERM INT

status=0
wait -n "${router_pid}" "${proxy_pid}" || status=$?
kill "${router_pid}" "${proxy_pid}" 2>/dev/null || true
wait 2>/dev/null || true
exit "${status}"
