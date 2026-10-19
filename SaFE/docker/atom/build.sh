#!/bin/bash
# Build an ATOM image on the SGLang or vLLM ROCm base.
# Usage: ./build.sh sglang|vllm [image-tag]

set -euo pipefail

engine="${1:-}"
tag="${2:-}"
sglang_base="${SGLANG_BASE:-docker.io/primussafe/sglang-rocm:20261013121117-8e371c}"
vllm_base="docker.io/vllm/vllm-openai-rocm:v0.29.0"

case "${engine}" in
    sglang)
        base="${sglang_base}"
        tag="${tag:-primussafe/atom:sglang}"
        ;;
    vllm)
        base="${vllm_base}"
        tag="${tag:-primussafe/atom:vllm}"
        ;;
    *)
        echo "usage: $0 sglang|vllm [image-tag]" >&2
        exit 2
        ;;
esac

root="$(cd "$(dirname "$0")" && pwd)"
docker build \
    -f "${root}/Dockerfile" \
    --build-arg "ENGINE=${engine}" \
    --build-arg "BASE_IMAGE=${base}" \
    -t "${tag}" \
    "${root}"
