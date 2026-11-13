#!/bin/bash
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

# Runs bootstrap/upgrade.sh against the real charts with a stand-in `helm` and `kubectl`, and checks
# what it does with the auth_*, sso_* and console_* keys of .env: they reach the rendered chart, and an upgrade never
# silently turns the external authentication, or the SSO settings, of the running release off.
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
auth_key_actors='[\"client-a\"]'
auth_api_keys_url=/api/v1/apikeys
auth_api_keys_upstream=identity.idns.svc.cluster.local:8080"

# What helm get values says for a release that is running with that auth setting.
LIVE_EXTERNAL='{"global":{"sub_domain":"test"},"auth":{"mode":"external","login_url":"https://idp.example.com/sign-in/start","client_secret":"safe-introspection","authenticators":[{"type":"oidc","issuer":"https://idp.example.com","audience":"primus-safe"}],"group_mappings":[{"group":"admins","roles":["system-admin"]}],"key_actors":["client-a"],"api_keys_upstream":"identity.idns.svc.cluster.local:8080"}}'
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
expect "all auth keys: key_actors" rendered 'key_actors: ["client-a"]'
expect "all auth keys: the path of the provider's key route is reported" rendered 'api_keys_url: "/api/v1/apikeys"'
expect "all auth keys: the console's nginx gets the provider's service" grep -qxF '              value: "identity.idns.svc.cluster.local:8080"' "$OUT/rendered.yaml"
expect "all auth keys: the script prints no auth value" bash -c '! grep -q "idp.example.com" "$1"' _ "$OUT/log"
# Rendered with and without: only the auth segment differs.
cp "$OUT/rendered.yaml" "$W/with-auth.yaml"
run_upgrade "" ""
diff_lines="$(diff "$OUT/rendered.yaml" "$W/with-auth.yaml" | grep -E '^[<>]' | grep -vE 'mode:|authenticators:|group_mappings:|login_url:|key_actors:|api_keys_url:|API_KEYS_UPSTREAM|nginx proxies /api/v1/apikeys|value: "identity.idns.svc.cluster.local:8080"|env:$|auth-client-secret|mountPath: /etc/secrets/auth|readOnly: true|secretName: safe-introspection|secret:$' || true)"
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

# 4b. The console's route to the identity provider is one of them: an upgrade must not drop it either.
run_upgrade "$LIVE_EXTERNAL" "$(grep -v '^auth_api_keys_upstream' <<<"$AUTH_ENV")"
expect "live external, api_keys_upstream missing: refused" refused
expect "live external, api_keys_upstream missing: names it" grep -q 'does not set: auth_api_keys_upstream$' "$OUT/log"

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
# but an explicit mode needs no reading of the release's values (nor do explicit SSO settings, nor
# an explicit external_user_dir_mounts, nor explicit console_return_prefixes)
FAKE_GET_FAIL=1 run_upgrade "$LIVE_EXTERNAL" "$AUTH_ENV
sso_login_enable=false
sso_bridge_allowlist='[]'
external_user_dir_mounts=true
console_return_prefixes='[]'"
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

# The console's sign-in at an OIDC provider (sso_login_*): off unless .env says so; then on, with the
# other provider's Secret kept or removed (sso_secret); and refused before anything is touched when
# a value is not usable.
SSO_ON="sso_enable=true"
run_upgrade "" "$SSO_ON"
expect "sso, no sso_login keys: applied" applied
expect "sso, no sso_login keys: sso.enable is true" rendered 'enable: true'
expect "sso, no sso_login keys: sso.login stays off" grep -qxF '        enable: false' "$OUT/rendered.yaml"
expect "sso, no sso_login keys: the sso Secret is mounted" grep -qxF '            secretName: primus-safe-sso' "$OUT/rendered.yaml"
SSO_LOGIN="sso_login_enable=true
sso_login_issuer=https://idp.example.com
sso_login_client_id=console
sso_login_redirect_uri=https://console.example.com/sso/callback"
run_upgrade "" "$SSO_ON
$SSO_LOGIN"
expect "sso_login: applied" applied
expect "sso_login: enabled" grep -qxF '        enable: true' "$OUT/rendered.yaml"
expect "sso_login: issuer" grep -qxF '        issuer: "https://idp.example.com"' "$OUT/rendered.yaml"
expect "sso_login: client id" grep -qxF '        client_id: "console"' "$OUT/rendered.yaml"
expect "sso_login: redirect uri" grep -qxF '        redirect_uri: "https://console.example.com/sso/callback"' "$OUT/rendered.yaml"
expect "sso_login: the other provider's Secret is still mounted" grep -qxF '            secretName: primus-safe-sso' "$OUT/rendered.yaml"
run_upgrade "" "$SSO_ON
$SSO_LOGIN
sso_secret="
expect "sso_login, sso_secret empty: applied" applied
expect "sso_login, sso_secret empty: no sso Secret is mounted" bash -c '! grep -q "sso-secret" "$1"' _ "$OUT/rendered.yaml"
expect "sso_login, sso_secret empty: sso stays enabled" rendered 'enable: true'
run_upgrade "" "$SSO_ON
sso_login_enable=yes"
expect "sso_login_enable neither true nor false: refused" refused
run_upgrade "" "$SSO_ON
sso_login_enable=true
sso_login_issuer=https://idp.example.com"
expect "sso_login without a client id: refused" refused
run_upgrade "" "$SSO_ON
$SSO_LOGIN
sso_login_client_id='a|b'"
expect "sso_login with a '|' in a value: refused" refused

# A value that the sed replacement would mangle ('&' is the matched text there) is refused.
run_upgrade "" "$SSO_ON
$SSO_LOGIN
sso_login_redirect_uri='https://console.example.com/sso/callback?a=1&b=2'"
expect "sso_login with a '&' in a value: refused" refused
expect "sso_login with a '&' in a value: says so" grep -q "sso_login_redirect_uri in .env must not contain" "$OUT/log"
# sso.login is an SSO setting: on with SSO off is refused, not silently ignored.
run_upgrade "" "$SSO_LOGIN"
expect "sso_login_enable=true with sso_enable=false: refused" refused
expect "sso_login_enable=true with sso_enable=false: says so" grep -q 'sso_login_enable=true in .env needs sso_enable=true' "$OUT/log"
run_upgrade "" "sso_login_enable=false"
expect "sso_login_enable=false with sso_enable=false: applied" applied
# Removing the Secret leaves the apiserver without a provider unless sso.login is on.
run_upgrade "" "$SSO_ON
sso_secret="
expect "sso_secret empty without sso_login: refused" refused
expect "sso_secret empty without sso_login: says why" grep -q 'needs sso_login_enable=true' "$OUT/log"

# sso_bridge_allowlist: a JSON array that reaches sso.bridge_allowlist, alone or next to the auth_* keys.
ALLOW="sso_bridge_allowlist='[\"http://localhost:5173\", \"http://127.0.0.1:5173\"]'"
run_upgrade "" "$SSO_ON
$ALLOW"
expect "bridge allowlist: applied" applied
expect "bridge allowlist: rendered" rendered 'bridge_allowlist: ["http://localhost:5173","http://127.0.0.1:5173"]'
expect "bridge allowlist: a second values file" grep -qx 2 "$OUT/value-files"
run_upgrade "$LIVE_BUILTIN" "$SSO_ON
$AUTH_ENV
$ALLOW"
expect "bridge allowlist with auth keys: applied" applied
expect "bridge allowlist with auth keys: allowlist rendered" rendered 'bridge_allowlist: ["http://localhost:5173","http://127.0.0.1:5173"]'
expect "bridge allowlist with auth keys: auth still rendered" rendered 'mode: "external"'
run_upgrade "" "$SSO_ON
sso_bridge_allowlist="
expect "empty bridge allowlist is the empty list: applied" applied
expect "empty bridge allowlist is the empty list: rendered" rendered 'bridge_allowlist: []'
run_upgrade "" "$SSO_ON
sso_bridge_allowlist=http://localhost:5173"
expect "bridge allowlist that is not JSON: refused" refused
expect "bridge allowlist that is not JSON: says so" grep -q 'sso_bridge_allowlist in .env is not a JSON array' "$OUT/log"
run_upgrade "" "$SSO_ON
sso_bridge_allowlist='[1]'"
expect "bridge allowlist with a non-string entry: refused" refused
RUN_PATH="$NOJQ" run_upgrade "" "$SSO_ON
$ALLOW"
expect "no jq, bridge allowlist: refused" refused
expect "no jq, bridge allowlist: asks for jq" grep -q 'jq is required when .env sets sso_bridge_allowlist' "$OUT/log"

# A release that runs with sso.login on or a bridge allowlist: an .env without them is refused before the
# cluster is touched (the next upgrade would sign everyone out, or cut the bridge); with them it applies.
LIVE_SSO='{"global":{"sub_domain":"test"},"sso":{"enable":true,"secret":"primus-safe-sso","bridge_allowlist":["http://localhost:5173","http://127.0.0.1:5173"],"login":{"enable":true,"issuer":"https://idp.example.com","client_id":"console","redirect_uri":"https://console.example.com/sso/callback"}}}'
LIVE_SSO_LOGIN_ONLY='{"sso":{"enable":true,"bridge_allowlist":[],"login":{"enable":true,"issuer":"https://idp.example.com"}}}'
LIVE_SSO_ALLOW_ONLY='{"sso":{"enable":true,"bridge_allowlist":["http://localhost:5173"],"login":{"enable":false}}}'
LIVE_SSO_OFF='{"sso":{"enable":true,"bridge_allowlist":[],"login":{"enable":false,"issuer":""}}}'
for jqmode in jq nojq; do
  rp=""; [[ "$jqmode" == "nojq" ]] && rp="$NOJQ"
  RUN_PATH="${rp:-}" run_upgrade "$LIVE_SSO" "$SSO_ON"
  expect "$jqmode, live sso.login and allowlist, .env without them: refused" refused
  expect "$jqmode, live sso.login and allowlist, .env without them: names them" grep -q 'does not set: sso_login_enable sso_login_issuer sso_login_client_id sso_login_redirect_uri sso_bridge_allowlist$' "$OUT/log"
  RUN_PATH="${rp:-}" run_upgrade "$LIVE_SSO_LOGIN_ONLY" "$SSO_ON"
  expect "$jqmode, live sso.login only: refused" refused
  expect "$jqmode, live sso.login only: names sso_login only" grep -q 'does not set: sso_login_enable sso_login_issuer sso_login_client_id sso_login_redirect_uri$' "$OUT/log"
  RUN_PATH="${rp:-}" run_upgrade "$LIVE_SSO_ALLOW_ONLY" "$SSO_ON"
  expect "$jqmode, live allowlist only: refused" refused
  expect "$jqmode, live allowlist only: names sso_bridge_allowlist only" grep -q 'does not set: sso_bridge_allowlist$' "$OUT/log"
  RUN_PATH="${rp:-}" run_upgrade "$LIVE_SSO_OFF" "$SSO_ON"
  expect "$jqmode, live sso.login off and no allowlist: applied" applied
  RUN_PATH="${rp:-}" run_upgrade "$LIVE_SSO" "$SSO_ON
sso_login_enable=false"
  expect "$jqmode, live allowlist, .env turns sso.login off but leaves the allowlist out: refused" refused
done
# helm keeps a value set with --set-string as a string: "true" is on all the same.
for jqmode in jq nojq; do
  rp=""; [[ "$jqmode" == "nojq" ]] && rp="$NOJQ"
  RUN_PATH="${rp:-}" run_upgrade '{"sso":{"enable":true,"login":{"enable":"true"}}}' "$SSO_ON"
  expect "$jqmode, live sso.login.enable is the string \"true\": refused" refused
done
FAKE_GET_FAIL=1 run_upgrade "$LIVE_SSO" "$SSO_ON"
expect "live SSO values cannot be read: refused" refused
run_upgrade "$LIVE_SSO" "$SSO_ON
$SSO_LOGIN
$ALLOW"
expect "live sso.login and allowlist, .env carries them: applied" applied
expect "live sso.login and allowlist, .env carries them: sso.login stays on" grep -qxF '        enable: true' "$OUT/rendered.yaml"
expect "live sso.login and allowlist, .env carries them: allowlist kept" rendered 'bridge_allowlist: ["http://localhost:5173","http://127.0.0.1:5173"]'
run_upgrade "$LIVE_SSO" "$SSO_ON
sso_login_enable=false
sso_bridge_allowlist='[]'"
expect "live sso.login and allowlist, dropped on purpose: applied" applied
expect "live sso.login and allowlist, dropped on purpose: sso.login off" grep -qxF '        enable: false' "$OUT/rendered.yaml"
expect "live sso.login and allowlist, dropped on purpose: allowlist empty" rendered 'bridge_allowlist: []'

# external_execution.user_dir_mounts: .env wins, otherwise the running release keeps its value, so an
# upgrade never silently goes back to external Pods mounting a hostpath volume whole.
UDM_ON='      user_dir_mounts: true'
UDM_OFF='      user_dir_mounts: false'
run_upgrade "" ""
expect "user_dir_mounts, fresh install: applied" applied
expect "user_dir_mounts, fresh install: chart default false" grep -qxF "$UDM_OFF" "$OUT/rendered.yaml"
for jqmode in jq nojq; do
  rp=""; [[ "$jqmode" == "nojq" ]] && rp="$NOJQ"
  RUN_PATH="${rp:-}" run_upgrade '{"external_execution":{"enabled":true,"user_dir_mounts":true}}' ""
  expect "$jqmode, live user_dir_mounts on, no .env key: applied" applied
  expect "$jqmode, live user_dir_mounts on, no .env key: stays on" grep -qxF "$UDM_ON" "$OUT/rendered.yaml"
  RUN_PATH="${rp:-}" run_upgrade '{"external_execution":{"user_dir_mounts":"true"}}' ""
  expect "$jqmode, live user_dir_mounts is the string \"true\": stays on" grep -qxF "$UDM_ON" "$OUT/rendered.yaml"
  RUN_PATH="${rp:-}" run_upgrade '{"external_execution":{"enabled":true}}' ""
  expect "$jqmode, live release without the value: chart default false" grep -qxF "$UDM_OFF" "$OUT/rendered.yaml"
done
run_upgrade '{"external_execution":{"user_dir_mounts":true}}' 'external_user_dir_mounts=false'
expect "live on, .env external_user_dir_mounts=false: applied" applied
expect "live on, .env external_user_dir_mounts=false: off on purpose" grep -qxF "$UDM_OFF" "$OUT/rendered.yaml"
run_upgrade '{"external_execution":{"user_dir_mounts":false}}' 'external_user_dir_mounts=true'
expect "live off, .env external_user_dir_mounts=true: on" grep -qxF "$UDM_ON" "$OUT/rendered.yaml"
run_upgrade "" 'external_user_dir_mounts=yes'
expect "external_user_dir_mounts=yes: refused" refused
expect "external_user_dir_mounts=yes: names the key" grep -q 'external_user_dir_mounts in .env is' "$OUT/log"
FAKE_GET_FAIL=1 run_upgrade '{"external_execution":{"user_dir_mounts":true}}' ""
expect "live user_dir_mounts cannot be read: refused" refused

# console.return_prefixes: .env wins (a JSON array of path prefixes); otherwise the running release keeps
# its list, so an upgrade never silently sends an app added to it back to the console's 404 page.
CRP='console_return_prefixes='"'"'["/dispatron","/autopilot","/binding","/blog-drafts","/claw-api"]'"'"
CRP_RENDERED='return_prefixes: ["/dispatron","/autopilot","/binding","/blog-drafts","/claw-api"]'
no_return_prefixes() { ! grep -q 'return_prefixes:' "$OUT/rendered.yaml"; }
run_upgrade "" ""
expect "return prefixes, fresh install: applied" applied
expect "return prefixes, fresh install: left to the apiserver's default" no_return_prefixes
run_upgrade "" "$CRP"
expect "return prefixes from .env: applied" applied
expect "return prefixes from .env: rendered" rendered "$CRP_RENDERED"
run_upgrade "" "console_return_prefixes="
expect "empty return prefixes is the empty list: rendered" rendered 'return_prefixes: []'
for bad in '/claw-api' '["/"]' '["claw-api"]' '["/claw-api/"]' '["/a/../b"]' '["//evil.example.net"]' '[1]'; do
  run_upgrade "" "console_return_prefixes='$bad'"
  expect "return prefixes $bad: refused" refused
  expect "return prefixes $bad: says so" grep -q 'console_return_prefixes in .env is not a JSON array of path prefixes' "$OUT/log"
done
RUN_PATH="$NOJQ" run_upgrade "" "$CRP"
expect "no jq, return prefixes: refused" refused
expect "no jq, return prefixes: asks for jq" grep -q 'jq is required when .env sets console_return_prefixes' "$OUT/log"
LIVE_CRP='{"console":{"access_contact":"x","return_prefixes":["/dispatron","/autopilot","/binding","/blog-drafts","/claw-api"]}}'
run_upgrade "$LIVE_CRP" ""
expect "live return prefixes, no .env key: applied" applied
expect "live return prefixes, no .env key: kept" rendered "$CRP_RENDERED"
run_upgrade '{"console":{"return_prefixes":[]}}' ""
expect "live empty return prefixes, no .env key: kept empty" rendered 'return_prefixes: []'
run_upgrade '{"console":{"access_contact":"x"}}' ""
expect "live release without return prefixes: left to the apiserver's default" no_return_prefixes
run_upgrade "$LIVE_CRP" "console_return_prefixes='[\"/dispatron\"]'"
expect "live return prefixes, .env wins: rendered" rendered 'return_prefixes: ["/dispatron"]'
RUN_PATH="$NOJQ" run_upgrade "$LIVE_CRP" ""
expect "no jq, live return prefixes, no .env key: refused" refused
expect "no jq, live return prefixes, no .env key: names the key" grep -q 'Add console_return_prefixes to .env' "$OUT/log"
RUN_PATH="$NOJQ" run_upgrade '{"console":{"access_contact":"x"}}' ""
expect "no jq, live release without return prefixes: applied" applied
FAKE_GET_FAIL=1 run_upgrade "$LIVE_CRP" "external_user_dir_mounts=false"
expect "live return prefixes cannot be read: refused" refused
run_upgrade "$LIVE_BUILTIN" "$SSO_ON
$AUTH_ENV
$ALLOW
$CRP"
expect "return prefixes with auth keys and allowlist: applied" applied
expect "return prefixes with auth keys and allowlist: prefixes rendered" rendered "$CRP_RENDERED"
expect "return prefixes with auth keys and allowlist: allowlist rendered" rendered 'bridge_allowlist: ["http://localhost:5173","http://127.0.0.1:5173"]'
expect "return prefixes with auth keys and allowlist: auth rendered" rendered 'mode: "external"'

echo
if ((FAILS > 0)); then
  echo "$FAILS check(s) failed; logs are in $W until this shell exits"
  trap - EXIT
  exit 1
fi
echo "all checks passed"
