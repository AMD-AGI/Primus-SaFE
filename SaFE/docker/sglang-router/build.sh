#!/bin/bash
# Build the SGLang router image from the upstream SGLang ROCm image.
# Usage: ./build.sh [image-tag]
# SGLANG_BASE overrides the upstream SGLang ROCm image.
# LIBIONIC overrides the ABI-1 provider file.
# BUILDER overrides the container builder. On vm07:
#   BUILDER="sudo podman --root /mnt/node-data/router-podman" ./build.sh <tag>

set -euo pipefail

tag="${1:-primussafe/sglang-router:latest}"
sglang_base="${SGLANG_BASE:-docker.io/lmsysorg/sglang-rocm:v0.5.21-rocm720-mi35x-20261008}"
root="$(cd "$(dirname "$0")" && pwd)"
atom_root="$(cd "${root}/../atom" && pwd)"
ionic="${LIBIONIC:-${root}/libionic.so.1.1.54.0-187}"
ionic_name="libionic.so.1.1.54.0-187"
if [[ ! -s "${ionic}" ]]; then
    echo "missing ABI-1 ${ionic_name}; set LIBIONIC to libionic.so.1.1.54.0-187" >&2
    exit 1
fi
size="$(stat -c '%s' "${ionic}")"
if [[ "${size}" != "84120" ]]; then
    echo "libionic size ${size} is not the ABI-1 file (84120 bytes)" >&2
    exit 1
fi

builder="${BUILDER:-docker}"
atom_tag="primussafe/atom:sglang-router-base"
SGLANG_BASE="${sglang_base}" BUILDER="${builder}" "${atom_root}/build.sh" sglang "${atom_tag}"

stage="$(mktemp -d)"
trap 'rm -rf "${stage}"' EXIT
cp "${root}/Dockerfile" \
    "${root}/anthropic_proxy.py" \
    "${root}/sglang-router-gateway.sh" \
    "${root}/sshd-init.sh" \
    "${root}/sh-wrapper.sh" \
    "${root}/entrypoint.sh" \
    "${stage}/"
cp "${ionic}" "${stage}/${ionic_name}"

# shellcheck disable=SC2086
${builder} build \
    -f "${stage}/Dockerfile" \
    --build-arg "BASE_IMAGE=${atom_tag}" \
    -t "${tag}" \
    "${stage}"
