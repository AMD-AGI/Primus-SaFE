#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Runs bootstrap/upgrade.sh against the real chart with a stand-in `helm` and `kubectl`, and checks
# the cluster-scoped objects of the chart:
#   - a console deployment (CALLED_BY_CD=true) stops before it changes the cluster when a
#     ClusterRole, ClusterRoleBinding, admission policy or webhook configuration of the chart is
#     missing or cannot be updated by the deployment account, and names those objects;
#   - an administrator upgrade applies bootstrap/cd-installer-rbac.yaml after the chart.
#
# Needs helm. Usage: bootstrap/tests/upgrade_cd_rbac_test.sh

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFE_DIR="$(cd "$HERE/../.." && pwd)"
command -v helm > /dev/null || { echo "helm is required" >&2; exit 2; }
REAL_HELM="$(command -v helm)"
export REAL_HELM

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT
mkdir -p "$W/bin" "$W/bootstrap" "$W/charts"
cp -r "$SAFE_DIR/charts/primus-safe" "$SAFE_DIR/charts/primus-safe-cr" "$W/charts/"
cp "$SAFE_DIR/bootstrap/upgrade.sh" "$SAFE_DIR/bootstrap/cd-installer-rbac.yaml" "$W/bootstrap/"

cat > "$W/bin/helm" <<'EOF'
#!/bin/bash
echo "helm $*" >> "$FAKE_LOG"
args=("$@")
if [[ "$1" == "-n" ]]; then shift 2; fi
case "$1" in
  list)
    echo "NAME         NAMESPACE    REVISION  STATUS"
    echo "primus-safe  primus-safe  7         deployed"
    ;;
  get) echo '{}' ;;
  template) exec "$REAL_HELM" "${args[@]}" ;;
esac
exit 0
EOF
# FAKE_MISSING lists <kind>/<name> that `kubectl get` does not find; FAKE_DENIED lists
# <resource>/<name> that `kubectl auth can-i update` refuses. FAKE_CRD_ABSENT is a CRD
# file name that is not in the cluster: a directory-wide replace or create fails, and
# replace of that one file fails.
cat > "$W/bin/kubectl" <<'EOF'
#!/bin/bash
echo "kubectl $*" >> "$FAKE_LOG"
if [[ "$1" == "get" && " ${FAKE_MISSING:-} " == *" $2/$3 "* ]]; then
  exit 1
fi
if [[ "$1 $2 $3" == "auth can-i update" && " ${FAKE_DENIED:-} " == *" $4 "* ]]; then
  echo no
  exit 1
fi
if [[ -n "${FAKE_CRD_ABSENT:-}" && ( "$1" == "replace" || "$1" == "create" ) && "$2" == "-f" ]]; then
  target="$3"
  if [[ -d "$target" || "$target" == */crds || "$target" == */crds/ ]]; then
    exit 1
  fi
  if [[ "$1" == "replace" && "$(basename "$target")" == "$FAKE_CRD_ABSENT" ]]; then
    exit 1
  fi
fi
exit 0
EOF
printf '#!/bin/bash\nexit 0\n' > "$W/bin/sleep"
chmod +x "$W/bin/"*

cat > "$W/bootstrap/.env" <<'EOF'
ethernet_nic=eth0
rdma_nic=rdma0
cluster_scale=small
storage_class=local
s3_enable=false
sso_enable=false
ingress=nginx
sub_domain=test
proxy_image_registry=registry.example.com/proxy
helm_registry=registry.example.com
cd_require_approval=false
csi_volume_handle=h
install_node_agent=n
EOF

FAILS=0
CASE=0
# run_upgrade <called by cd> <missing> <denied> [absent crd file]: runs upgrade.sh; leaves the
# log in $OUT/log, the exit code in $RC and the recorded cluster calls in $FAKE_LOG.
run_upgrade() {
  CASE=$((CASE + 1))
  OUT="$W/out$CASE"
  mkdir -p "$OUT"
  FAKE_LOG="$OUT/calls"
  : > "$FAKE_LOG"
  (
    cd "$W/bootstrap" && export FAKE_LOG CALLED_BY_CD="$1" FAKE_MISSING="$2" FAKE_DENIED="$3" \
      FAKE_CRD_ABSENT="${4:-}" && PATH="$W/bin:$PATH" bash ./upgrade.sh
  ) > "$OUT/log" 2>&1
  RC=$?
}

ok() { echo "ok   - $1"; }
bad() { echo "FAIL - $1"; FAILS=$((FAILS + 1)); }
expect() { # expect <name> <command...>
  local name="$1"; shift
  if "$@" > /dev/null 2>&1; then ok "$name"; else bad "$name"; fi
}
# refused: the upgrade stopped before any call that changes the cluster
refused() {
  [[ $RC -ne 0 ]] && ! grep -q 'helm upgrade' "$FAKE_LOG" \
    && ! grep -qE '^kubectl (replace|create|apply|delete|annotate|label|patch)' "$FAKE_LOG"
}
applied() { [[ $RC -eq 0 ]] && grep -q 'helm upgrade -i primus-safe ' "$FAKE_LOG"; }
rbac_applied() { grep -q '^kubectl apply -f .*cd-installer-rbac.yaml' "$FAKE_LOG"; }

# 1. Console deployment, every cluster object present and updatable: applied, CD RBAC left alone.
run_upgrade true "" ""
expect "cd, objects present: applied" applied
expect "cd, objects present: checks the rendered ClusterRoles" grep -q '^kubectl get clusterrole primus-safe-resource-manager' "$FAKE_LOG"
expect "cd, objects present: does not apply cd-installer-rbac.yaml" bash -c '! grep -q cd-installer-rbac.yaml "$1"' _ "$FAKE_LOG"

# 2. A ClusterRole the chart adds does not exist yet: refused, named, with the administrator steps.
run_upgrade true "clusterrole/primus-safe-resource-manager" ""
expect "cd, ClusterRole missing: refused" refused
expect "cd, ClusterRole missing: names it" grep -q 'clusterrole/primus-safe-resource-manager' "$OUT/log"
expect "cd, ClusterRole missing: says what to do" grep -q 'cd-installer-rbac.yaml' "$OUT/log"

# 3. An admission policy binding exists but the deployment account cannot update it: refused, named.
run_upgrade true "" "validatingadmissionpolicybindings.admissionregistration.k8s.io/primus-safe-rbac-projection"
expect "cd, binding not updatable: refused" refused
expect "cd, binding not updatable: names it" grep -q 'validatingadmissionpolicybinding/primus-safe-rbac-projection' "$OUT/log"

# 4. Administrator upgrade: no preflight, cd-installer-rbac.yaml applied after the chart.
run_upgrade false "clusterrole/primus-safe-resource-manager" ""
expect "admin: applied" applied
expect "admin: applies cd-installer-rbac.yaml" rbac_applied
expect "admin: applies it after the chart" bash -c 'grep -n "" "$1" | grep -E "helm upgrade -i primus-safe |cd-installer-rbac.yaml" | head -1 | grep -q "helm upgrade"' _ "$FAKE_LOG"
expect "admin: no deployment-account check" bash -c '! grep -q "auth can-i" "$1"' _ "$FAKE_LOG"

# 5. One new CRD among CRDs that already exist: that file is created, the upgrade continues.
run_upgrade true "" "" "amd.com_qos.yaml"
expect "cd, new crd: applied" applied
expect "cd, new crd: replaces an existing crd" grep -q 'kubectl replace -f primus-safe/crds/amd.com_users.yaml ' "$FAKE_LOG"
expect "cd, new crd: creates the missing file" grep -q 'kubectl create -f primus-safe/crds/amd.com_qos.yaml ' "$FAKE_LOG"
expect "cd, new crd: does not apply the directory" bash -c '! grep -Eq "^kubectl (replace|create) -f primus-safe/crds/ " "$1"' _ "$FAKE_LOG"

echo
if [[ $FAILS -eq 0 ]]; then
  echo "all passed"
else
  echo "$FAILS failed"
  exit 1
fi
