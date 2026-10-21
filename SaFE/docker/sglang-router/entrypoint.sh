#!/bin/bash
# Start sshd, then the container command.
# No command keeps the image CMD, which is tail -f /dev/null.

set -euo pipefail

/usr/local/bin/sshd-init.sh || echo "[entrypoint] sshd init failed (continuing)" >&2
# Mark init done so a later /bin/sh -c from the platform does not start sshd again.
mkdir -p /run
touch /run/sshd-init.done

if [[ $# -eq 0 ]]; then
    exec tail -f /dev/null
fi
exec "$@"
