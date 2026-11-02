#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Runs bootstrap/upgrade.sh against the real charts with a stand-in `helm` and `kubectl`, and checks
# what it does with the auth_* keys of .env: they reach the rendered chart, and an upgrade never
# silently turns the external authentication of the running release off.
#
# The stand-in `helm upgrade` runs the real `helm template` with the same chart and values files
# instead of installing, and the stand-in `kubectl` only records its calls, so a refused upgrade is
# seen to leave the cluster alone (no helm upgrade, no kubectl call at all).
#
# Needs helm and jq. Usage: bootstrap/tests/upgrade_auth_test.sh

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAFE_DIR="$(cd "$HERE/../.." && pwd)"
for tool in helm jq; do
  command -v "$tool" > /dev/null || { echo "$tool is required" >&2; exit 2; }
done
REAL_HELM="$(command -v helm)"
export REAL_HELM

W="$(mktemp -d)"
trap 'rm -rf "$W"' EXIT
mkdir -p "$W/bin" "$W/bootstrap" "$W/charts"
cp -r "$SAFE_DIR/charts/primus-safe" "$SAFE_DIR/charts/primus-safe-cr" "$W/charts/"
cp "$SAFE_DIR/bootstrap/upgrade.sh" "$W/bootstrap/"

cat > "$W/bin/helm" <<'EOF'
#!/bin/bash
echo "helm $*" >> "$FAKE_LOG"
args=("$@")
if [[ "$1" == "-n" ]]; then shift 2; fi
case "$1" in
  list)
    [[ -z "${FAKE_LIST_FAIL:-}" ]] || exit 1
    if [[ -n "${FAKE_LIVE_VALUES:-}" ]]; then
      echo "NAME         NAMESPACE    REVISION  STATUS"
      echo "primus-safe  primus-safe  7         deployed"
    fi
    ;;
  get)
    [[ -z "${FAKE_GET_FAIL:-}" ]] || exit 1
    case "$*" in
      *"-o json"*) cat "$FAKE_LIVE_VALUES" ;;
      # helm get values -o yaml: sorted keys, a list at the indent of its key
      *"-o yaml"*) python3 - "$FAKE_LIVE_VALUES" <<'PY'
import json, sys
def scalar(v):
    if v is None: return "null"
    if v is True: return "true"
    if v is False: return "false"
    if isinstance(v, (int, float)): return str(v)
    return json.dumps(v) if v == "" or any(c in v for c in ':#{}[],&*!|>\'"%@`') else v
def flat(v): return "{}" if v == {} else "[]" if v == [] else scalar(v)
def emit(o, ind):
    pad = " " * ind
    if isinstance(o, dict):
        for k in sorted(o):
            v = o[k]
            if isinstance(v, (dict, list)) and v:
                print(f"{pad}{k}:")
                emit(v, ind + 2 if isinstance(v, dict) else ind)
            else:
                print(f"{pad}{k}: {flat(v)}")
    else:
        for item in o:
            if isinstance(item, dict) and item:
                for i, k in enumerate(sorted(item)):
                    lead = pad + ("- " if i == 0 else "  ")
                    v = item[k]
                    if isinstance(v, (dict, list)) and v:
                        print(f"{lead}{k}:")
                        emit(v, ind + 4 if isinstance(v, dict) else ind + 2)
                    else:
                        print(f"{lead}{k}: {flat(v)}")
            else:
                print(f"{pad}- {flat(item)}")
emit(json.load(open(sys.argv[1])), 0)
PY
        ;;
      *) exit 1 ;;
    esac
    ;;
  template)
    exec "$REAL_HELM" "${args[@]}"
    ;;
  upgrade)
    name="$3"; chart="$4"; shift 4
    files=()
    while (($#)); do
      if [[ "$1" == "-f" ]]; then files+=("$2"); shift; fi
      shift
    done
    if [[ "$name" == "primus-safe" ]]; then
      fargs=()
      for f in "${files[@]}"; do fargs+=(-f "$f"); done
      "$REAL_HELM" template "$name" "$chart" -n primus-safe "${fargs[@]}" > "$FAKE_OUT/rendered.yaml" || exit 1
      echo "${#files[@]}" > "$FAKE_OUT/value-files"
    fi
    ;;
esac
exit 0
EOF
printf '#!/bin/bash\necho "kubectl $*" >> "$FAKE_LOG"\nexit 0\n' > "$W/bin/kubectl"
printf '#!/bin/bash\nexit 0\n' > "$W/bin/sleep"
chmod +x "$W/bin/"*
# A PATH without jq, for a host that has helm and kubectl only.
mkdir -p "$W/nojq"
for tool in bash sed cp rm mkdir grep awk cat python3 env dirname; do
  ln -s "$(command -v "$tool")" "$W/nojq/$tool"
done
cp "$W/bin/helm" "$W/bin/kubectl" "$W/bin/sleep" "$W/nojq/"

BASE_ENV='ethernet_nic=eth0
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
install_node_agent=n'

AUTH_ENV="auth_mode=external
auth_login_url=https://idp.example.com/sign-in/start
auth_client_secret=safe-introspection
auth_authenticators='[{\"type\":\"oidc\",\"issuer\":\"https://idp.example.com\",\"audience\":\"primus-safe\"}]'
auth_group_mappings='[{\"group\":\"admins\",\"roles\":[\"system-admin\"]}]'
auth_key_actors='[\"client-a\"]'"

# What helm get values says for a release that is running with that auth setting.
LIVE_EXTERNAL='{"global":{"sub_domain":"test"},"auth":{"mode":"external","login_url":"https://idp.example.com/sign-in/start","client_secret":"safe-introspection","authenticators":[{"type":"oidc","issuer":"https://idp.example.com","audience":"primus-safe"}],"group_mappings":[{"group":"admins","roles":["system-admin"]}],"key_actors":["client-a"]}}'
LIVE_BUILTIN='{"global":{"sub_domain":"test"},"auth":{"mode":"builtin","authenticators":[],"group_mappings":[],"login_url":""}}'

FAILS=0
CASE=0
# run_upgrade <live values json or ""> <extra .env lines>: runs upgrade.sh; leaves the log in $OUT/log,
# the exit code in $RC, the recorded cluster calls in $FAKE_LOG and the rendered chart in $OUT/rendered.yaml.
run_upgrade() {
  local live="$1" extra="$2"
  CASE=$((CASE + 1))
  OUT="$W/out$CASE"
  mkdir -p "$OUT"
  FAKE_OUT="$OUT"
  FAKE_LOG="$OUT/calls"
  : > "$FAKE_LOG"
  printf '%s\n%s\n' "$BASE_ENV" "$extra" > "$W/bootstrap/.env"
  unset FAKE_LIVE_VALUES
  if [[ -n "$live" ]]; then
    printf '%s' "$live" > "$OUT/live.json"
    FAKE_LIVE_VALUES="$OUT/live.json"
  fi
  (
    cd "$W/bootstrap" && export FAKE_LOG FAKE_OUT FAKE_LIVE_VALUES CALLED_BY_CD=true && PATH="${RUN_PATH:-$W/bin:$PATH}" bash ./upgrade.sh
  ) > "$OUT/log" 2>&1
  RC=$?
}

ok() { echo "ok   - $1"; }
bad() { echo "FAIL - $1"; FAILS=$((FAILS + 1)); }
expect() { # expect <name> <command...>
  local name="$1"; shift
  if "$@" > /dev/null 2>&1; then ok "$name"; else bad "$name"; fi
}
refused() { # the upgrade stopped before it touched the cluster
  [[ $RC -ne 0 ]] && ! grep -q 'helm upgrade' "$FAKE_LOG" && ! grep -q '^kubectl' "$FAKE_LOG"
}
applied() { [[ $RC -eq 0 ]] && grep -q 'helm upgrade -i primus-safe ' "$FAKE_LOG" && [[ -s "$OUT/rendered.yaml" ]]; }
rendered() { grep -qxF -- "      $1" "$OUT/rendered.yaml"; }

# 1. No auth_* keys, nothing installed: the chart default, builtin; no second values file.
run_upgrade "" ""
expect "no auth keys, fresh install: applied" applied
expect "no auth keys, fresh install: builtin" rendered 'mode: "builtin"'
expect "no auth keys, fresh install: only the one values file" grep -qx 1 "$OUT/value-files"

# 2. The auth_* keys reach the rendered chart, on a release that is not external yet.
run_upgrade "$LIVE_BUILTIN" "$AUTH_ENV"
expect "all auth keys: applied" applied
expect "all auth keys: mode" rendered 'mode: "external"'
expect "all auth keys: login_url" rendered 'login_url: "https://idp.example.com/sign-in/start"'
expect "all auth keys: authenticators" rendered 'authenticators: [{"audience":"primus-safe","issuer":"https://idp.example.com","type":"oidc"}]'
expect "all auth keys: group_mappings" rendered 'group_mappings: [{"group":"admins","roles":["system-admin"]}]'
expect "all auth keys: client secret mounted into the apiserver" grep -qxF '            secretName: safe-introspection' "$OUT/rendered.yaml"
expect "all auth keys: key_actors reach the values" grep -qx 2 "$OUT/value-files"
expect "all auth keys: the script prints no auth value" bash -c '! grep -q "idp.example.com" "$1"' _ "$OUT/log"
# Rendered with and without: only the auth segment differs.
cp "$OUT/rendered.yaml" "$W/with-auth.yaml"
run_upgrade "" ""
diff_lines="$(diff "$OUT/rendered.yaml" "$W/with-auth.yaml" | grep -E '^[<>]' | grep -vE 'mode:|authenticators:|group_mappings:|login_url:|auth-client-secret|mountPath: /etc/secrets/auth|readOnly: true|secretName: safe-introspection|secret:$' || true)"
expect "rendering with and without the keys differs only in the auth segment" test -z "$diff_lines"
[[ -z "$diff_lines" ]] || echo "$diff_lines"

# 3. The running release is external and .env has no auth_* keys: refuse, and touch nothing.
run_upgrade "$LIVE_EXTERNAL" ""
expect "live external, no auth keys: refused before the cluster is touched" refused
expect "live external, no auth keys: the message names what is missing" grep -q 'does not set: auth_mode auth_login_url auth_client_secret auth_authenticators auth_group_mappings auth_key_actors' "$OUT/log"
expect "live external, no auth keys: the message says how to go back on purpose" grep -q 'auth_mode=builtin' "$OUT/log"

# 4. One key missing is enough, here the one that grants the administrator roles.
run_upgrade "$LIVE_EXTERNAL" "$(grep -v '^auth_group_mappings' <<<"$AUTH_ENV")"
expect "live external, group_mappings missing: refused" refused
expect "live external, group_mappings missing: names it" grep -q 'does not set: auth_group_mappings$' "$OUT/log"

# 5. Every key present: the upgrade goes through and renders what the release runs.
run_upgrade "$LIVE_EXTERNAL" "$AUTH_ENV"
expect "live external, all keys in .env: applied" applied
expect "live external, all keys in .env: still external" rendered 'mode: "external"'

# 6. A key the release does not have is not demanded.
run_upgrade '{"auth":{"mode":"external","authenticators":[{"type":"builtin"}]}}' 'auth_mode=external
auth_authenticators='"'"'[{"type":"builtin"}]'"'"
expect "live external without optional keys: applied" applied

# 7. auth_mode=builtin is the deliberate way back: allowed, and the other keys still apply.
run_upgrade "$LIVE_EXTERNAL" "$(sed 's/^auth_mode=.*/auth_mode=builtin/' <<<"$AUTH_ENV")"
expect "break-glass, auth_mode=builtin: applied" applied
expect "break-glass, auth_mode=builtin: builtin" rendered 'mode: "builtin"'
expect "break-glass, auth_mode=builtin: the chain is kept for the way back" rendered 'authenticators: [{"audience":"primus-safe","issuer":"https://idp.example.com","type":"oidc"}]'
run_upgrade "$LIVE_EXTERNAL" 'auth_mode=builtin'
expect "break-glass with nothing else in .env: applied" applied
expect "break-glass with nothing else in .env: builtin" rendered 'mode: "builtin"'

# 8. A release that is builtin has nothing to lose.
run_upgrade "$LIVE_BUILTIN" ""
expect "live builtin, no auth keys: applied" applied
expect "live builtin, no auth keys: builtin" rendered 'mode: "builtin"'
run_upgrade '{"global":{"sub_domain":"test"}}' ""
expect "an older release that has no auth values: applied" applied
run_upgrade 'null' ""
expect "a release with no user-supplied values: applied" applied

# 9. Could not tell is not "nothing to lose".
FAKE_GET_FAIL=1 run_upgrade "$LIVE_EXTERNAL" ""
expect "release values cannot be read: refused" refused
expect "release values cannot be read: says so" grep -q 'cannot read the values of release primus-safe' "$OUT/log"
FAKE_LIST_FAIL=1 run_upgrade "$LIVE_EXTERNAL" ""
expect "releases cannot be listed: refused" refused
expect "releases cannot be listed: says so" grep -q 'cannot list the helm releases' "$OUT/log"
# but an explicit mode needs no reading of the release's values
FAKE_GET_FAIL=1 run_upgrade "$LIVE_EXTERNAL" "$AUTH_ENV"
expect "release values cannot be read, .env has every key: applied" applied

# 10. Values that cannot be used are refused before anything is changed.
run_upgrade "$LIVE_BUILTIN" "auth_authenticators=not-json"
expect "authenticators that are not JSON: refused" refused
expect "authenticators that are not JSON: says so" grep -q 'auth_authenticators in .env is not a JSON array' "$OUT/log"
run_upgrade "$LIVE_BUILTIN" "auth_key_actors='{\"a\":1}'"
expect "key_actors that is not an array: refused" refused
expect "key_actors that is not an array: says so" grep -q 'auth_key_actors in .env is not a JSON array' "$OUT/log"
run_upgrade "$LIVE_BUILTIN" "auth_mode=extrenal"
expect "a mode that is neither builtin nor external: refused" refused
expect "a mode that is neither builtin nor external: says so" grep -q 'auth_mode in .env must be builtin or external' "$OUT/log"
run_upgrade "$LIVE_BUILTIN" "auth_mode=external"
expect "external without a chain: refused" refused
expect "external without a chain: says why" grep -q 'needs a non-empty auth_authenticators' "$OUT/log"
run_upgrade "$LIVE_BUILTIN" "auth_key_actors="
expect "an empty key_actors is the empty list: applied" applied

# 11. A host without jq: a release that is not external upgrades as it always did, and the break-glass
# needs no jq; an external release asks for it instead of guessing.
export NOJQ="$W/nojq"
RUN_PATH="$NOJQ" run_upgrade "$LIVE_BUILTIN" ""
expect "no jq, live builtin, no auth keys: applied" applied
expect "no jq, live builtin, no auth keys: builtin" rendered 'mode: "builtin"'
RUN_PATH="$NOJQ" run_upgrade '{"global":{"sub_domain":"test"}}' ""
expect "no jq, release without auth values: applied" applied
RUN_PATH="$NOJQ" run_upgrade "" ""
expect "no jq, fresh install: applied" applied
RUN_PATH="$NOJQ" run_upgrade "$LIVE_EXTERNAL" ""
expect "no jq, live external, no auth keys: refused" refused
expect "no jq, live external, no auth keys: asks for jq" grep -q 'jq is required' "$OUT/log"
RUN_PATH="$NOJQ" run_upgrade "$LIVE_EXTERNAL" 'auth_mode=builtin'
expect "no jq, break-glass: applied" applied
expect "no jq, break-glass: builtin" rendered 'mode: "builtin"'
RUN_PATH="$NOJQ" run_upgrade "$LIVE_EXTERNAL" "$AUTH_ENV"
expect "no jq, every key in .env: refused, the values cannot be written" refused
expect "no jq, every key in .env: says why" grep -q 'jq is required when .env sets auth_' "$OUT/log"
RUN_PATH="$NOJQ" run_upgrade "$LIVE_BUILTIN" "auth_mode=extrenal"
expect "no jq, a mode that is neither: refused" refused

echo
if ((FAILS > 0)); then
  echo "$FAILS check(s) failed; logs are in $W until this shell exits"
  trap - EXIT
  exit 1
fi
echo "all checks passed"
