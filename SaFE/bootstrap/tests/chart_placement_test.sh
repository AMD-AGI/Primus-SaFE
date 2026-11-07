#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Checks where the primus-safe chart places the management components (apiserver, resource-manager,
# job-manager, webhooks, web, nginx and the database instances):
# - with global.management_node_selector and global.management_tolerations unset, no component gets
#   a nodeSelector, tolerations or a required node affinity, and the default preferences stay;
# - with both set, every Deployment carries the selector and the tolerations, and the database
#   instances carry the tolerations and the selector as one required node-affinity term (all labels
#   must match), next to the affinity they already had.
#
# Needs helm, yq (v4) and jq. Usage: bootstrap/tests/chart_placement_test.sh

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFE_DIR="$(cd "$HERE/../.." && pwd)"
CHART="$SAFE_DIR/charts/primus-safe"
for tool in helm yq jq; do
  command -v "$tool" > /dev/null || { echo "$tool is required" >&2; exit 2; }
done

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT
FAILED=0
fail() { echo "FAIL: $*" >&2; FAILED=1; }
pass() { echo "ok: $*"; }

# render NAME [helm args...] writes the chart's documents as one JSON array to $W/NAME.json.
# net.ingress=nginx so that the nginx Deployment is rendered too.
render() {
  local name="$1"; shift
  if ! helm template primus-safe "$CHART" -n primus-safe --set net.ingress=nginx "$@" \
      > "$W/$name.yaml" 2> "$W/$name.err"; then
    cat "$W/$name.err" >&2
    fail "helm template ($name) failed"
    echo '[]' > "$W/$name.json"
    return
  fi
  yq ea -o=json -I=0 '[.] | map(select(. != null))' "$W/$name.yaml" > "$W/$name.json"
}

# q NAME FILTER runs a jq filter over a rendered set.
q() { jq -e "$2" "$W/$1.json" > /dev/null; }

# expect NAME FILTER OK BAD passes with OK when FILTER holds on set NAME, else fails with BAD.
expect() { if q "$1" "$2"; then pass "$3"; else fail "$4"; fi; }

cat > "$W/control-plane.yaml" << 'EOF'
global:
  management_node_selector:
    node-role.kubernetes.io/control-plane: ""
  management_tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
EOF
cat > "$W/two-labels.yaml" << 'EOF'
global:
  management_node_selector:
    pool: system
    disk: ssd
EOF

render default
render cp -f "$W/control-plane.yaml"
render two -f "$W/two-labels.yaml"

COMPONENTS='["apiserver", "resource-manager", "job-manager", "webhooks", "web", "nginx"]'
DEPLOYS="[.[] | select(.kind == \"Deployment\")
  | select((.metadata.name | ltrimstr(\"primus-safe-\")) as \$c | $COMPONENTS | index(\$c) != null)]"
DB='[.[] | select(.kind == "PostgresCluster") | .spec.instances[]]'
SELECTOR='{"node-role.kubernetes.io/control-plane": ""}'
TOLERATIONS='[{"key": "node-role.kubernetes.io/control-plane", "operator": "Exists", "effect": "NoSchedule"}]'
PREFERS_NON_GPU='any(.spec.template.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution[];
  any(.preference.matchExpressions[]; .key == "feature.node.kubernetes.io/amd-gpu"))'
NODE_TERMS='.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms'

expect default "$DEPLOYS | length == 6" \
  "default: the 6 management Deployments are rendered" \
  "expected the 6 management Deployments, got $(jq -c "$DEPLOYS | map(.metadata.name)" "$W/default.json")"
expect default "$DB | length > 0" \
  "default: the database instances are rendered" \
  "the chart renders no PostgresCluster instance"

# 1. Unset: placement is left to the scheduler.
expect default "$DEPLOYS | all(.spec.template.spec | has(\"nodeSelector\") or has(\"tolerations\") | not)" \
  "default: no Deployment gets a nodeSelector or tolerations" \
  "default: a Deployment gets a nodeSelector or tolerations"
expect default "$DEPLOYS | all($PREFERS_NON_GPU)" \
  "default: every Deployment still prefers non-GPU nodes" \
  "default: a Deployment lost its non-GPU preference"
expect default "$DB | all((has(\"tolerations\") | not) and (.affinity | has(\"nodeAffinity\") | not))" \
  "default: the database instances get no tolerations or node affinity" \
  "default: the database instances get tolerations or a node affinity"

# 2. Set: every component goes where the selector points.
expect cp "$DEPLOYS | all(.spec.template.spec.nodeSelector == $SELECTOR and .spec.template.spec.tolerations == $TOLERATIONS)" \
  "set: every Deployment carries the selector and the tolerations" \
  "set: Deployments without the selector or tolerations: $(jq -c "$DEPLOYS | map(select(.spec.template.spec.nodeSelector != $SELECTOR or .spec.template.spec.tolerations != $TOLERATIONS) | .metadata.name)" "$W/cp.json")"
expect cp "$DEPLOYS | all($PREFERS_NON_GPU)" \
  "set: the Deployments keep their default affinity" \
  "set: a Deployment lost its default affinity"
expect cp "$DB | all(.tolerations == $TOLERATIONS)" \
  "set: the database instances carry the tolerations" \
  "set: the database instances do not carry the tolerations"
expect cp "$DB | all($NODE_TERMS == [{\"matchExpressions\": [
    {\"key\": \"node-role.kubernetes.io/control-plane\", \"operator\": \"In\", \"values\": [\"\"]}]}])" \
  "set: the database instances require the selector's node" \
  "set: database node affinity is $(jq -c "$DB | map(.affinity.nodeAffinity)" "$W/cp.json")"
expect cp "$DB | all(.affinity | has(\"podAntiAffinity\"))" \
  "set: the database instances keep their anti-affinity" \
  "set: the database instances lost their anti-affinity"

# 3. Several labels stay one term, so a node must carry all of them, as with a nodeSelector.
expect two "$DB | all($NODE_TERMS == [{\"matchExpressions\": [
    {\"key\": \"disk\", \"operator\": \"In\", \"values\": [\"ssd\"]},
    {\"key\": \"pool\", \"operator\": \"In\", \"values\": [\"system\"]}]}])" \
  "two labels: the database instances require both in one term" \
  "two labels: database node affinity is $(jq -c "$DB | map(.affinity.nodeAffinity)" "$W/two.json")"
expect two "$DEPLOYS | all(.spec.template.spec | (.nodeSelector == {\"pool\": \"system\", \"disk\": \"ssd\"}) and (has(\"tolerations\") | not))" \
  "two labels: the Deployments get both labels and no tolerations" \
  "two labels: Deployments render $(jq -c "$DEPLOYS | map(.spec.template.spec | {nodeSelector, tolerations})" "$W/two.json")"

exit "$FAILED"
