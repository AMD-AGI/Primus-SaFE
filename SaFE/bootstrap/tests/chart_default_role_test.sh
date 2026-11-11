#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Checks the SaFE roles that the primus-safe-cr chart renders:
# - no role other than system-admin, system-admin-readonly and workspace-admin (applied only to
#   the managers of a workspace, who administer its workloads) grants get on another user's
#   workload (grantedUsers workspace-user or "*"): get returns the workload's env, which often
#   holds the owner's credentials, and also authorizes pod logs, service and SSH/webshell;
# - the default role still lets the owner get its own workloads and members create and list
#   workloads in their workspaces.
#
# Needs helm, yq (v4) and jq. Usage: bootstrap/tests/chart_default_role_test.sh

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFE_DIR="$(cd "$HERE/../.." && pwd)"
CHART="$SAFE_DIR/charts/primus-safe-cr"
for tool in helm yq jq; do
  command -v "$tool" > /dev/null || { echo "$tool is required" >&2; exit 2; }
done

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT
FAILED=0
fail() { echo "FAIL: $*" >&2; FAILED=1; }
pass() { echo "ok: $*"; }

if ! helm template primus-safe-cr "$CHART" -n primus-safe > "$W/cr.yaml" 2> "$W/cr.err"; then
  cat "$W/cr.err" >&2
  echo "FAIL: helm template primus-safe-cr failed" >&2
  exit 1
fi
yq ea -o=json -I=0 '[.] | map(select(. != null))' "$W/cr.yaml" > "$W/cr.json"

# q FILTER runs a jq filter over the rendered documents.
q() { jq -e "$1" "$W/cr.json" > /dev/null; }

ROLES='[.[] | select(.kind == "Role")]'
q "$ROLES | map(.metadata.name) | index(\"default\") != null" || fail "the chart renders no default Role"

# A rule that reaches workloads (by name, by priority sub-resource, or "*") and grants get ("*" too)
# to someone other than the owner.
# shellcheck disable=SC2016 # a jq program, not shell
WORKLOAD_GET_TO_OTHERS='[
  .[] | select(.kind == "Role")
  | select(.metadata.name as $n | ["system-admin", "system-admin-readonly", "workspace-admin"] | index($n) | not)
  | .metadata.name as $role
  | .rules[]
  | select(any(.resources[]; . == "*" or . == "workload" or startswith("workload/")))
  | select(any(.verbs[]; . == "get" or . == "*"))
  | select(any(.grantedUsers[]; . != "owner"))
  | {role: $role, grantedUsers, resources, verbs}
]'
if q "$WORKLOAD_GET_TO_OTHERS | length == 0"; then
  pass "no non-admin role grants get on another user's workload"
else
  fail "roles grant get on other users' workloads: $(jq -c "$WORKLOAD_GET_TO_OTHERS" "$W/cr.json")"
fi

DEFAULT_RULES='[.[] | select(.kind == "Role" and .metadata.name == "default") | .rules[]]'
q "$DEFAULT_RULES | any(
    (.grantedUsers | index(\"owner\")) and (.resources | index(\"workload\")) and (.verbs | index(\"get\")))" \
  && pass "default role: the owner gets its own workloads" \
  || fail "default role: the owner can no longer get its own workloads"
for verb in create list; do
  q "$DEFAULT_RULES | any(
      (.grantedUsers | index(\"workspace-user\")) and (.resources | index(\"workload\")) and (.verbs | index(\"$verb\")))" \
    && pass "default role: workspace members $verb workloads" \
    || fail "default role: workspace members can no longer $verb workloads"
done

exit "$FAILED"
