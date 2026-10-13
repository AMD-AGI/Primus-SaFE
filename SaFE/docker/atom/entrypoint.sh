#!/bin/bash
# Starts atomesh or the ATOM OpenAI server.
# Mode is the first argument, or ATOM_ROLE: router -> atomesh, prefill/decode -> atom.
# Extra arguments replace the default flags.

set -euo pipefail

mode="${1:-}"
if [[ "${mode}" == "atomesh" || "${mode}" == "atom" ]]; then
    shift
else
    case "${ATOM_ROLE:-}" in
        router) mode="atomesh" ;;
        prefill|decode) mode="atom" ;;
        *)
            echo "usage: atom-entrypoint atomesh|atom [args...]" >&2
            echo "or set ATOM_ROLE=router|prefill|decode" >&2
            exit 2
            ;;
    esac
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
