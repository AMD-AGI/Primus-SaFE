#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Checks the RBAC that the primus-safe chart renders and that the console deployment account is given
# (bootstrap/cd-installer-rbac.yaml):
# - ClusterRole primus-safe reads ClusterRoles and deletes the hook RBAC, and does not write them;
#   ClusterRole primus-safe-resource-manager is the only one that writes clusterroles and clusterrolebindings;
# - the pre-install/pre-upgrade hook account secret-manager gets a Role in the release namespace limited
#   to the secrets its jobs write, never a ClusterRole, and the cleanup job removes what was rendered;
# - nothing binds cluster-admin;
# - the deployment account may escalate exactly the ClusterRoles the chart renders, and holds every
#   right of the hook Role, so it can create that Role without escalate.
#
# Needs helm, yq (v4) and jq. Usage: bootstrap/tests/chart_rbac_test.sh

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFE_DIR="$(cd "$HERE/../.." && pwd)"
CHART="$SAFE_DIR/charts/primus-safe"
INSTALLER="$SAFE_DIR/bootstrap/cd-installer-rbac.yaml"
for tool in helm yq jq; do
  command -v "$tool" > /dev/null || { echo "$tool is required" >&2; exit 2; }
done

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT
FAILED=0
fail() { echo "FAIL: $*" >&2; FAILED=1; }
pass() { echo "ok: $*"; }

# render NAME [helm args...] writes the chart's documents as one JSON array to $W/NAME.json.
render() {
  local name="$1"; shift
  if ! helm template primus-safe "$CHART" -n primus-safe "$@" > "$W/$name.yaml" 2> "$W/$name.err"; then
    cat "$W/$name.err" >&2
    fail "helm template ($name) failed"
    echo '[]' > "$W/$name.json"
    return
  fi
  yq ea -o=json -I=0 '[.] | map(select(. != null))' "$W/$name.yaml" > "$W/$name.json"
}

# q NAME FILTER runs a jq filter over a rendered set.
q() { jq -e "$2" "$W/$1.json" > /dev/null; }

render default
render crossns --set opensearch.secret_namespace=logging --set opensearch.secret=os-creds
render nocopy --set opensearch.username=u --set opensearch.password=p
yq ea -o=json -I=0 '[.] | map(select(. != null))' "$INSTALLER" > "$W/installer.json"

# 1. The components' ClusterRoles. Workspace projection is written only by the resource-manager role.
CR='.[] | select(.kind == "ClusterRole" and .metadata.name == "primus-safe")'
RM='.[] | select(.kind == "ClusterRole" and .metadata.name == "primus-safe-resource-manager")'
RBAC_RULES="[$CR | .rules[] | select(.apiGroups | index(\"rbac.authorization.k8s.io\"))]"
RM_RULES="[$RM | .rules[] | select(.apiGroups | index(\"rbac.authorization.k8s.io\"))]"
q default "$RBAC_RULES | length > 0" || fail "ClusterRole primus-safe has no rbac.authorization.k8s.io rules"
PROJ='(.resources | sort) == ["clusterrolebindings", "clusterroles"] and (.verbs | sort) == ["create", "delete", "get", "list", "patch", "update", "watch"] and .resourceNames == null'
READ='((.verbs - ["get", "list", "watch", "delete"]) | length) == 0 and ((.verbs | index("delete")) == null or .resourceNames == ["secret-manager"])'
q default "$RBAC_RULES | map(select($PROJ)) | length == 0" \
  && pass "ClusterRole primus-safe does not project workspace RBAC" \
  || fail "ClusterRole primus-safe still writes clusterroles: $(jq -c "$RBAC_RULES" "$W/default.json")"
q default "$RM_RULES | map(select($PROJ)) | length == 1" \
  && pass "ClusterRole primus-safe-resource-manager projects workspace RBAC" \
  || fail "ClusterRole primus-safe-resource-manager projection rule is missing: $(jq -c "$RM_RULES" "$W/default.json")"
q default "$RBAC_RULES | all(.[]; ($READ))" \
  && pass "ClusterRole primus-safe writes no other RBAC" \
  || fail "ClusterRole primus-safe can write RBAC: $(jq -c "$RBAC_RULES" "$W/default.json")"
q default '[.[] | select(.kind == "ValidatingAdmissionPolicy" and .metadata.name == "primus-safe-rbac-projection")] | length == 1' \
  || fail "ValidatingAdmissionPolicy primus-safe-rbac-projection is not rendered"

# 2. Hook RBAC: namespaced, scoped to the secrets of the jobs.
for set in default crossns nocopy; do
  q "$set" '[.[] | select((.kind == "ClusterRole" or .kind == "ClusterRoleBinding") and .metadata.name == "secret-manager")] | length == 0' \
    && pass "$set: no cluster-wide secret-manager RBAC" || fail "$set: chart renders cluster-wide secret-manager RBAC"
  q "$set" '[.[] | select(.kind == "Role" and .metadata.name == "secret-manager" and .metadata.namespace == "primus-safe")] | length == 1' \
    || fail "$set: no Role secret-manager in the release namespace"
  q "$set" '[.[] | select(.kind == "Role" and .metadata.name == "secret-manager") | .rules[] | .verbs[]] | index("*") | not' \
    || fail "$set: hook Role grants *"
  q "$set" '[.[] | select(.kind == "Role" and .metadata.name == "secret-manager") | .rules[] | select(.resourceNames == null) | .verbs] | all(. == ["create"])' \
    || fail "$set: hook Role grants more than create without a resource name"
  q "$set" '[.[] | select(.kind == "Role" or .kind == "RoleBinding" or .kind == "ServiceAccount") | select(.metadata.name == "secret-manager") | .metadata.annotations["helm.sh/hook"]] | all(. == "pre-install,pre-upgrade")' \
    || fail "$set: hook RBAC is not a pre-install,pre-upgrade hook"
done

NAMED='[.[] | select(.kind == "Role" and .metadata.name == "secret-manager" and .metadata.namespace == "primus-safe") | .rules[] | .resourceNames // [] | .[]]'
q default "$NAMED | (index(\"primus-safe-crypto\") and index(\"primus-safe-ssh-proxy\") and index(\"primus-safe-opensearch-config\"))" \
  && pass "default: hook Role names the crypto, ssh and opensearch secrets" || fail "default: hook Role misses a secret: $(jq -c "$NAMED" "$W/default.json")"
q nocopy "$NAMED | index(\"primus-safe-opensearch-config\") | not" \
  && pass "nocopy: no opensearch secret when copy-opensearch-secret is not rendered" || fail "nocopy: hook Role still names the opensearch secret"
q nocopy '[.[] | select(.kind == "Job" and .metadata.name == "copy-opensearch-secret")] | length == 0' \
  || fail "nocopy: copy-opensearch-secret rendered although credentials are set"

# Every job that runs as secret-manager is a hook that runs before the hook RBAC is removed.
q default '[.[] | select(.kind == "Job" and .spec.template.spec.serviceAccountName == "secret-manager") | .metadata.annotations["helm.sh/hook"]] | length > 0 and all(. == "pre-install,pre-upgrade")' \
  || fail "a job running as secret-manager is not a pre-install,pre-upgrade hook"

# Cross-namespace opensearch source: a Role there, and the cleanup job removes it.
q crossns '[.[] | select(.kind == "Role" and .metadata.name == "secret-manager" and .metadata.namespace == "logging") | .rules] == [[{"apiGroups": [""], "resources": ["secrets"], "resourceNames": ["os-creds"], "verbs": ["get"]}]]' \
  && pass "crossns: Role in the source namespace reads only the source secret" || fail "crossns: source-namespace Role wrong"
q crossns '[.[] | select(.kind == "RoleBinding" and .metadata.name == "secret-manager" and .metadata.namespace == "logging") | .subjects[0]] == [{"kind": "ServiceAccount", "name": "secret-manager", "namespace": "primus-safe"}]' \
  || fail "crossns: source-namespace RoleBinding wrong"
q default '[.[] | select(.metadata.namespace != null and .metadata.namespace != "primus-safe")] | length == 0' \
  || fail "default: objects rendered outside the release namespace"

# The cleanup job deletes exactly what the hook rendered (and the legacy cluster-wide objects).
cleanup() { jq -r '.[] | select(.kind == "Job" and .metadata.name == "delete-preinstall-rbac") | .spec.template.spec.containers[0].command[2]' "$W/$1.json"; }
CLEANUP_FAILED=$FAILED
for set in default crossns; do
  script="$(cleanup "$set")"
  while IFS=$'\t' read -r kind ns; do
    lower="$(tr '[:upper:]' '[:lower:]' <<< "$kind")"
    if [[ -n "$ns" ]]; then
      grep -qF "kubectl -n $ns delete $lower secret-manager --ignore-not-found" <<< "$script" \
        || fail "$set: cleanup job does not delete $kind $ns/secret-manager"
    else
      grep -qF "kubectl delete $lower secret-manager --ignore-not-found" <<< "$script" \
        || fail "$set: cleanup job does not delete $kind secret-manager"
    fi
  done < <(jq -r '.[] | select(.metadata.name == "secret-manager" and .metadata.annotations["helm.sh/hook"] != null) | [.kind, (.metadata.namespace // "")] | @tsv' "$W/$set.json"
           printf 'ClusterRole\t\nClusterRoleBinding\t\n')
  grep -q '^set -e$' <<< "$script" || fail "$set: cleanup job ignores failures"
done
[[ $FAILED -eq $CLEANUP_FAILED ]] && pass "cleanup job deletes all hook RBAC"

# 3. Nothing binds cluster-admin.
for f in default crossns installer; do
  q "$f" '[.[] | select(.roleRef.name == "cluster-admin")] | length == 0' || fail "$f binds cluster-admin"
done

# 4. The deployment account.
IR='.[] | select(.kind == "ClusterRole" and .metadata.name == "primus-safe-cd-installer") | .rules[]'
q installer "[$IR | select(.verbs | index(\"*\"))] | length == 0" || fail "installer ClusterRole grants *"
q installer "[$IR | select(.verbs | index(\"escalate\") or index(\"bind\")) | .resourceNames] | all(. != null)" \
  && pass "installer escalate/bind are limited by name" || fail "installer escalate or bind without resourceNames"
q installer "[$IR | select(.apiGroups | index(\"rbac.authorization.k8s.io\")) | select(.verbs | index(\"create\"))] | length == 0" \
  && pass "installer cannot create ClusterRoles or ClusterRoleBindings" || fail "installer can create cluster-wide RBAC"
q installer "[$IR | select(.apiGroups | index(\"\")) | select(.resources | index(\"secrets\"))] | length == 0" \
  || fail "installer reads secrets cluster-wide"
# Derived from what the chart renders: every ClusterRole that upgrade.sh replaces must be escalatable.
ESC="$(jq -c "[$IR | select(.verbs | index(\"escalate\")) | .resourceNames[]] | sort" "$W/installer.json")"
ALL_CR="$(jq -c '[.[] | select(.kind == "ClusterRole") | .metadata.name] | sort' "$W/default.json")"
[[ "$ESC" == "$ALL_CR" ]] && pass "installer escalates exactly the chart's ClusterRoles $ALL_CR" \
  || fail "installer escalate names $ESC differ from the chart's ClusterRoles $ALL_CR"
# The hook Role must be a subset of what the installer holds in the release namespace.
IRole='.[] | select(.kind == "Role" and .metadata.name == "primus-safe-cd-installer" and .metadata.namespace == "primus-safe") | .rules[]'
HELD="$(jq -c "[$IRole | select(.apiGroups | index(\"\")) | select(.resources | index(\"secrets\")) | .verbs[]] | unique" "$W/installer.json")"
NEED="$(jq -c '[.[] | select(.kind == "Role" and .metadata.name == "secret-manager" and .metadata.namespace == "primus-safe") | .rules[].verbs[]] | unique' "$W/default.json")"
jq -e -n --argjson held "$HELD" --argjson need "$NEED" '$need - $held | length == 0' > /dev/null \
  && pass "installer holds every right of the hook Role ($NEED)" || fail "installer lacks hook Role rights: need $NEED, holds $HELD"
q installer "[$IRole | select(.apiGroups | index(\"rbac.authorization.k8s.io\")) | .resources[]] | index(\"roles\") and index(\"rolebindings\")" \
  || fail "installer cannot create the hook Role/RoleBinding"

if [[ $FAILED -ne 0 ]]; then
  echo "chart RBAC test FAILED" >&2
  exit 1
fi
echo "chart RBAC test passed"
