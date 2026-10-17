#!/usr/bin/env bash
# Start sshd on port 2222 for key-based root login.
# Port 2222 avoids clashing with the node sshd when the pod uses hostNetwork.
# MN_SSH_AUTHORIZED_KEY, when set, replaces /root/.ssh/authorized_keys.

set -euo pipefail

MN_SSH_PORT="${MN_SSH_PORT:-2222}"

mkdir -p /root/.ssh /run/sshd
chmod 700 /root/.ssh

if [ -n "${MN_SSH_AUTHORIZED_KEY:-}" ]; then
    printf '%s\n' "${MN_SSH_AUTHORIZED_KEY}" > /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
fi

ssh-keygen -A

mkdir -p /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/atom-sshd.conf <<EOF
Port ${MN_SSH_PORT}
PermitRootLogin prohibit-password
PubkeyAuthentication yes
PasswordAuthentication no
EOF
if ! grep -qE '^\s*Include\s+/etc/ssh/sshd_config\.d/\*\.conf' /etc/ssh/sshd_config 2>/dev/null; then
    echo "Include /etc/ssh/sshd_config.d/*.conf" >> /etc/ssh/sshd_config
fi

/usr/sbin/sshd -e
echo "[sshd-init] sshd listening on :${MN_SSH_PORT}"
