#!/bin/bash
# Start sshd, then the container command.
# A specified command is executed as given.
# With no command, keep the container alive with tail -f /dev/null.

set -euo pipefail

/usr/local/bin/sshd-init.sh || echo "[entrypoint] sshd init failed (continuing)" >&2

if [[ $# -eq 0 ]]; then
    exec tail -f /dev/null
fi
exec "$@"
