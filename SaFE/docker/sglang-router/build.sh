#!/bin/bash
# Build the SGLang router image with the Anthropic front.
# Usage: ./build.sh [image-tag]
# BASE_IMAGE overrides the Atom image the router is layered on.

set -euo pipefail

tag="${1:-primussafe/sglang-router:latest}"
base="${BASE_IMAGE:-harbor.tw325.primus-safe.amd.com/primussafe/atom-dev:20261019082550}"

root="$(cd "$(dirname "$0")" && pwd)"
docker build \
    -f "${root}/Dockerfile" \
    --build-arg "BASE_IMAGE=${base}" \
    -t "${tag}" \
    "${root}"
