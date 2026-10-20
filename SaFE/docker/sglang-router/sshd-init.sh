#!/usr/bin/env bash
# Start sshd when Brain injects an authorized key.
# Host keys are generated here, not in the image build.
# MN_SSH_PORT is the prefill base. Decode adds 10 and router adds 20 so
# hostNetwork pods on one node do not bind the same port.

set -euo pipefail

mkdir -p /root/.ssh /run/sshd
chmod 700 /root/.ssh

if [ -z "${MN_SSH_AUTHORIZED_KEY:-}" ]; then
    echo "[sshd-init] MN_SSH_AUTHORIZED_KEY unset; sshd not started"
    exit 0
fi
if [ -z "${MN_SSH_PORT:-}" ]; then
    echo "[sshd-init] MN_SSH_PORT unset; sshd not started" >&2
    exit 0
fi
case "${MN_SSH_PORT}" in
    *[!0-9]*)
        echo "[sshd-init] MN_SSH_PORT is not a number; sshd not started" >&2
        exit 0
        ;;
esac

offset=0
case "${ATOM_ROLE:-}" in
    decode) offset=10 ;;
    router) offset=20 ;;
esac
port=$((MN_SSH_PORT + offset))
if [ "${port}" -lt 1 ] || [ "${port}" -gt 65535 ]; then
    echo "[sshd-init] ssh port ${port} is outside 1-65535; sshd not started" >&2
    exit 0
fi

printf '%s\n' "${MN_SSH_AUTHORIZED_KEY}" > /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys

ssh-keygen -A

mkdir -p /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/sglang-router-sshd.conf <<EOF
Port ${port}
PermitRootLogin prohibit-password
PubkeyAuthentication yes
PasswordAuthentication no
EOF
if ! grep -qE '^\s*Include\s+/etc/ssh/sshd_config\.d/\*\.conf' /etc/ssh/sshd_config 2>/dev/null; then
    echo "Include /etc/ssh/sshd_config.d/*.conf" >> /etc/ssh/sshd_config
fi

/usr/sbin/sshd -e
echo "[sshd-init] sshd listening on :${port}"
