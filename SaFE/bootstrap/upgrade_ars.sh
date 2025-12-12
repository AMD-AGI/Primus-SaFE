#!/usr/bin/env bash
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

set -euo pipefail

# Update AutoscalingRunnerSet container images from charts/primus-safe/values.yaml
# - Reads cicd.runner and cicd.unified_job from values.yaml (or a provided file)
# - Iterates all AutoscalingRunnerSet resources across all namespaces
# - Replaces:
#     - container name "runner"       -> image = cicd.runner
#     - container name "unified-job"  -> image = cicd.unified_job
#
# Requirements: kubectl, jq
#
# Usage:
#   ./charts/hack/update-ars-images.sh [PATH_TO_VALUES_YAML]
#     PATH_TO_VALUES_YAML defaults to charts/primus-safe/values.yaml

VALUES_FILE="$1"
if [[ ! -f "${VALUES_FILE}" ]]; then
  echo "[error] values file not found: ${VALUES_FILE}" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq command not found. Please install it first."
  exit 1
fi

# Extract values from YAML without yq, by simple awk/regex within cicd block
extract_yaml_value() {
  local key="$1"
  awk -v key="${key}" '
    /^[[:space:]]*#/ {next}
    /^[[:space:]]*$/ {next}
    /^cicd:/ {inblk=1; next}
    inblk && /^[^[:space:]]/ {inblk=0}
    inblk && $0 ~ "^[[:space:]]*" key ":" {
      match($0, key":[[:space:]]*\"?([^\"#]+)\"?", m);
      val=m[1]; gsub(/[[:space:]]+$/, "", val);
      print val; exit
    }
  ' "${VALUES_FILE}"
}

RUNNER_IMAGE="$(extract_yaml_value runner)"
UNIFIED_JOB_IMAGE="$(extract_yaml_value unified_job)"

if [[ -z "${RUNNER_IMAGE:-}" || -z "${UNIFIED_JOB_IMAGE:-}" ]]; then
  echo "[error] failed to parse runner/unified_job from ${VALUES_FILE}" >&2
  echo "  runner: '${RUNNER_IMAGE:-}'" >&2
  echo "  unified_job: '${UNIFIED_JOB_IMAGE:-}'" >&2
  exit 1
fi

echo "[info] runner image: ${RUNNER_IMAGE}"
echo "[info] unified-job image: ${UNIFIED_JOB_IMAGE}"

# Fetch all AutoscalingRunnerSets across all namespaces
ars_json="$(kubectl get autoscalingrunnersets.actions.github.com -A -o json)"
count="$(jq -r '.items | length' <<<"${ars_json}")"
if [[ "${count}" == "0" ]]; then
  echo "[info] no AutoscalingRunnerSet resources found"
  exit 0
fi

patched_any=0
for i in $(seq 0 $((count-1))); do
  ns="$(jq -r ".items[$i].metadata.namespace" <<<"${ars_json}")"
  name="$(jq -r ".items[$i].metadata.name" <<<"${ars_json}")"

  # Determine container indices by name
  idx_runner="$(jq -r ".items[$i].spec.template.spec.containers | to_entries[]? | select(.value.name==\"runner\") | .key" <<<"${ars_json}" || true)"
  idx_unified="$(jq -r ".items[$i].spec.template.spec.containers | to_entries[]? | select(.value.name==\"unified-job\") | .key" <<<"${ars_json}" || true)"

  patch_ops=()
  if [[ -n "${idx_runner}" ]]; then
    current_runner_image="$(jq -r ".items[$i].spec.template.spec.containers[${idx_runner}].image" <<<"${ars_json}")"
    # Keep prefix up to and including 'primussafe/' then replace the tail
    new_runner_image="${RUNNER_IMAGE}"
    if [[ "${current_runner_image}" == *"primussafe/"* ]]; then
      prefix="$(echo "${current_runner_image}" | sed -E 's|(.*primussafe/).*|\\1|')"
      # If sed captured, rebuild
      if [[ -n "${prefix}" ]]; then
        # undo escape for bash variable
        prefix="${prefix//\\1/}"
        prefix="$(echo "${current_runner_image}" | sed -E 's|(.*primussafe/).*|\\1|' | sed -E 's|\\1||')"
      fi
      # Simpler robust approach: cut at 'primussafe/' using parameter expansion
      prefix="${current_runner_image%primussafe/*}primussafe/"
      new_runner_image="${prefix}${RUNNER_IMAGE}"
    fi
    patch_ops+=("{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/${idx_runner}/image\",\"value\":\"${new_runner_image}\"}")
  fi
  if [[ -n "${idx_unified}" ]]; then
    current_unified_image="$(jq -r ".items[$i].spec.template.spec.containers[${idx_unified}].image" <<<"${ars_json}")"
    new_unified_image="${UNIFIED_JOB_IMAGE}"
    if [[ "${current_unified_image}" == *"primussafe/"* ]]; then
      prefix="${current_unified_image%primussafe/*}primussafe/"
      new_unified_image="${prefix}${UNIFIED_JOB_IMAGE}"
    fi
    patch_ops+=("{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/${idx_unified}/image\",\"value\":\"${new_unified_image}\"}")
  fi

  if [[ "${#patch_ops[@]}" -eq 0 ]]; then
    echo "[info] ${ns}/${name}: no target containers found (runner/unified-job), skip"
    continue
  fi

  patch_payload="[$(IFS=,; echo "${patch_ops[*]}")]"
  echo "[info] patching ${ns}/${name} ..."
  kubectl -n "${ns}" patch autoscalingrunnersets.actions.github.com "${name}" --type='json' -p "${patch_payload}" >/dev/null
  echo "[info] patched ${ns}/${name} successfully"
  patched_any=1
done

if [[ "${patched_any}" -eq 0 ]]; then
  echo "[info] nothing patched"
else
  echo "[info] done"
fi