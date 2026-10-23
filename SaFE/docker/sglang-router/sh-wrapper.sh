#!/usr/bin/dash
# /usr/bin/sh replacement. Platform entrypoints invoke /bin/sh -c, so sshd
# still starts when the image entrypoint is replaced. The real shell is dash.
# The done marker skips a second start. mkdir claims the init race.
if [ ! -f /run/sshd-init.done ]; then
    if mkdir /run/sshd-init.lock 2>/dev/null; then
        if [ ! -f /run/sshd-init.done ]; then
            if [ -x /usr/local/bin/sshd-init.sh ]; then
                /usr/local/bin/sshd-init.sh || true
            fi
            touch /run/sshd-init.done
        fi
    else
        i=0
        while [ ! -f /run/sshd-init.done ] && [ "$i" -lt 50 ]; do
            i=$((i + 1))
            sleep 0.05
        done
    fi
fi
exec /usr/bin/dash "$@"
