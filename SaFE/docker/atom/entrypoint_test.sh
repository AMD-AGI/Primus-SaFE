#!/usr/bin/env bash
# Verify that the image entrypoint preserves commands and defaults to tail.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp_dir="$(mktemp -d)"
tail_pid=""

cleanup() {
    if [ -n "${tail_pid}" ]; then
        kill "${tail_pid}" 2>/dev/null || true
        wait "${tail_pid}" 2>/dev/null || true
    fi
    rm -rf "${tmp_dir}"
}
trap cleanup EXIT

cat > "${tmp_dir}/sshd-init.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "${tmp_dir}/sshd-init.sh"

sed "s|/usr/local/bin/sshd-init.sh|${tmp_dir}/sshd-init.sh|" \
    "${script_dir}/entrypoint.sh" > "${tmp_dir}/entrypoint.sh"
chmod +x "${tmp_dir}/entrypoint.sh"

actual="$("${tmp_dir}/entrypoint.sh" /bin/printf '%s\n' 'specified command')"
if [ "${actual}" != "specified command" ]; then
    echo "specified command was changed: ${actual}" >&2
    exit 1
fi

ATOM_ROLE=router "${tmp_dir}/entrypoint.sh" tail -f /dev/null &
tail_pid=$!
sleep 0.1
if ! kill -0 "${tail_pid}" 2>/dev/null; then
    echo "default tail command exited for router role" >&2
    exit 1
fi
kill "${tail_pid}"
wait "${tail_pid}" 2>/dev/null || true
tail_pid=""

ATOM_ROLE=prefill "${tmp_dir}/entrypoint.sh" &
tail_pid=$!
sleep 0.1
if ! kill -0 "${tail_pid}" 2>/dev/null; then
    echo "empty command did not keep the container alive" >&2
    exit 1
fi
