#!/bin/bash
# Start sshd, then the container command.
# No command keeps the image CMD, which is tail -f /dev/null.

set -euo pipefail

/usr/local/bin/sshd-init.sh || echo "[entrypoint] sshd init failed (continuing)" >&2

if [[ $# -eq 0 ]]; then
    exec tail -f /dev/null
fi
exec "$@"
