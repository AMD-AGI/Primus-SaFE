#!/bin/bash
# Start sshd, then the container command.
# No command and no ATOM_ROLE: tail -f /dev/null.
# atomesh|atom, or ATOM_ROLE=router|prefill|decode, starts that service.
# Any other arguments are executed as the user command.

set -euo pipefail

/usr/local/bin/sshd-init.sh || echo "[entrypoint] sshd init failed (continuing)" >&2

# The image CMD is tail -f /dev/null. Treat that the same as no arguments
# so ATOM_ROLE can still select a service.
if [[ $# -eq 0 || ( $# -eq 3 && "$1" == "tail" && "$2" == "-f" && "$3" == "/dev/null" ) ]]; then
    case "${ATOM_ROLE:-}" in
        router) mode="atomesh" ;;
        prefill|decode) mode="atom" ;;
        *) exec tail -f /dev/null ;;
    esac
elif [[ "$1" == "atomesh" || "$1" == "atom" ]]; then
    mode="$1"
    shift
else
    exec "$@"
fi

port="${ATOM_SERVER_PORT:-8000}"

if [[ "${mode}" == "atomesh" ]]; then
    if [[ $# -gt 0 ]]; then
        exec atomesh launch "$@"
    fi
    args=(
        launch
        --host 0.0.0.0
        --port "${port}"
        --backend "${ATOM_MESH_BACKEND:-atom}"
        --policy random
        --log-level info
    )
    if [[ -n "${ATOM_PREFILL_ENDPOINT:-}" && -n "${ATOM_DECODE_ENDPOINT:-}" ]]; then
        args+=(--pd-disaggregation --prefill "${ATOM_PREFILL_ENDPOINT}" --decode "${ATOM_DECODE_ENDPOINT}")
    fi
    exec atomesh "${args[@]}"
fi

if [[ $# -gt 0 ]]; then
    exec /usr/local/bin/atom-python -m atom.entrypoints.openai_server "$@"
fi
exec /usr/local/bin/atom-python -m atom.entrypoints.openai_server --host 0.0.0.0 --port "${port}"
