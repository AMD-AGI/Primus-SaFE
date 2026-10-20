#!/usr/bin/dash
# /usr/bin/sh replacement. Platform entrypoints invoke /bin/sh -c, so sshd
# still starts when the image entrypoint is replaced. The real shell is dash.
if [ ! -f /run/sshd-init.done ]; then
    mkdir -p /run
    touch /run/sshd-init.done
    if [ -x /usr/local/bin/sshd-init.sh ]; then
        /usr/local/bin/sshd-init.sh || true
    fi
fi
exec /usr/bin/dash "$@"
