
#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

#!/bin/bash



# This script only applies if install.sh has been previously executed and
# the environment configuration and code directory have not changed.

set -euo pipefail
if ! command -v helm &> /dev/null; then
  echo "Error: helm command not found. Please install it first."
  exit 1
fi

if ! command -v kubectl &> /dev/null; then
  echo "Error: kubectl command not found. Please install it first."
  exit 1
fi

NAMESPACE="primus-safe"
chart_name="primus-safe"

# $3 is an optional second values file; it is applied after $2, so it wins.
install_or_upgrade_helm_chart() {
  local chart_name="$1"
  local values_yaml="$2"
  local extra_values="${3:-}"
  local chart_path="./$chart_name"
  local extra=()
  if [[ -n "$extra_values" ]]; then
    extra=(-f "$extra_values")
  fi

  helm upgrade -i "$chart_name" "$chart_path" -n "$NAMESPACE" -f $values_yaml ${extra[@]+"${extra[@]}"}
  echo "✅ $chart_name installed in namespace("$NAMESPACE")"
  echo
}

# Pluggable authentication (auth.*, see docs/apis/external-authentication.md) comes from .env, like
# the other settings of this script. Every key is optional; one that .env does not set keeps the
# chart default, except that a release which is already external refuses to lose it (check_live_auth).
#   auth_mode            builtin | external                                  -> auth.mode
#   auth_login_url       where the console sends users to sign in            -> auth.login_url
#   auth_client_secret   name of the Secret holding the client secret of an
#                        introspection authenticator (a name, not the secret) -> auth.client_secret
#   auth_authenticators  JSON array, the authenticator chain                 -> auth.authenticators
#   auth_group_mappings  JSON array, identity groups to roles                -> auth.group_mappings
#   auth_key_actors      JSON array, client ids that may fetch the keys of
#                        the user a delegated token acts for                 -> auth.key_actors
#   auth_api_keys_url    path where API key creation reaches the identity
#                        provider, when something else routes it              -> auth.api_keys_url
#   auth_api_keys_upstream  host[:port] of the identity provider's service,
#                        which the console's nginx routes /api/v1/apikeys to  -> auth.api_keys_upstream
# JSON values are single-quoted in .env, e.g. auth_key_actors='["client-a"]'.
AUTH_ENV_KEYS=(auth_mode auth_login_url auth_client_secret auth_authenticators auth_group_mappings auth_key_actors auth_api_keys_url auth_api_keys_upstream)

# check_live_auth refuses an upgrade that would silently turn the external authentication of the
# running release off. helm renders every value from the chart defaults and the files it is given, so a
# release that was switched to external, by hand or by an earlier .env, goes back to builtin on the
# next upgrade whose .env carries no auth_* keys: the apiserver stops accepting the identity
# provider's credentials and the platform roles its groups granted. The same goes for any auth.*
# setting that the release has and .env leaves out. auth_mode=builtin in .env is the deliberate way
# back (the break-glass for an identity provider outage) and is always allowed.
check_live_auth() {
  local releases live live_mode var key present unset_keys="" missing=""
  if ! releases=$(helm -n "$NAMESPACE" list); then
    echo "Error: cannot list the helm releases in $NAMESPACE, so the authentication mode of the running release is unknown" >&2
    return 1
  fi
  if ! grep -q "^$chart_name " <<<"$releases"; then
    return 0 # nothing installed yet, nothing to lose
  fi
  if [[ "${auth_mode:-}" == "builtin" ]]; then
    return 0
  fi
  for var in "${AUTH_ENV_KEYS[@]}"; do
    if [[ -z "${!var+x}" ]]; then
      unset_keys="$unset_keys $var"
    fi
  done
  if [[ -z "$unset_keys" ]]; then
    return 0 # .env says everything: nothing can be dropped
  fi
  if command -v jq &> /dev/null; then
    if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o json); then
      echo "Error: cannot read the values of release $chart_name, so its authentication mode is unknown" >&2
      return 1
    fi
    if ! live_mode=$(jq -r '.auth.mode // "builtin"' <<<"$live"); then
      echo "Error: cannot read auth.mode from the values of release $chart_name" >&2
      return 1
    fi
  else
    # Without jq only the mode can be read, from the YAML: a release that is not external has nothing
    # to lose and needs nothing else, so it upgrades as it always did.
    if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o yaml); then
      echo "Error: cannot read the values of release $chart_name, so its authentication mode is unknown" >&2
      return 1
    fi
    if ! live_mode=$(awk '/^auth:/ {a = 1; next} /^[^ #]/ {a = 0} a && /^  mode:/ {gsub(/["\047]/, "", $2); print $2; exit}' <<<"$live"); then
      echo "Error: cannot read auth.mode from the values of release $chart_name" >&2
      return 1
    fi
  fi
  if [[ "$live_mode" != "external" ]]; then
    return 0
  fi
  if ! command -v jq &> /dev/null; then
    echo "Error: release $chart_name is running with auth.mode=external, and jq is required to see which of its auth settings .env must carry" >&2
    echo "       Nothing has been changed. Install jq, or set every auth_* key in .env, or set auth_mode=builtin to go back" >&2
    echo "       to the built-in authentication on purpose." >&2
    return 1
  fi
  for var in $unset_keys; do
    key="${var#auth_}"
    if ! present=$(jq -r --arg k "$key" '.auth[$k] | (. != null and . != "" and . != [] and . != {})' <<<"$live"); then
      echo "Error: cannot read auth.$key from the values of release $chart_name" >&2
      return 1
    fi
    if [[ "$present" == "true" ]]; then
      missing="$missing $var"
    fi
  done
  if [[ -n "$missing" ]]; then
    echo "Error: release $chart_name is running with auth.mode=external, but .env does not set:$missing" >&2
    echo "       Upgrading now would drop those settings: the apiserver would stop accepting the identity" >&2
    echo "       provider's credentials, or the roles its groups grant. Nothing has been changed." >&2
    echo "       Add them to .env (the live values are: helm -n $NAMESPACE get values $chart_name -o json | jq .auth)," >&2
    echo "       or set auth_mode=builtin to go back to the built-in authentication on purpose." >&2
    return 1
  fi
}

# write_auth_values writes the auth_* keys of .env to the values file $1 (JSON, which helm reads as
# YAML) and sets AUTH_VALUES_FILE to it; it stays empty when .env sets none. It returns 1, with a
# message, for a value that cannot be used, before anything is changed.
write_auth_values() {
  local out="$1" json='{}' var key val any=false
  AUTH_VALUES_FILE=""
  for var in "${AUTH_ENV_KEYS[@]}"; do
    if [[ -n "${!var+x}" ]]; then
      any=true
    fi
  done
  if [[ "$any" != "true" ]]; then
    return 0
  fi
  if ! command -v jq &> /dev/null; then
    # The break-glass alone, auth_mode=builtin, needs no jq to be written.
    if [[ "${auth_mode:-}" == "builtin" && -z "${auth_login_url+x}${auth_client_secret+x}${auth_authenticators+x}${auth_group_mappings+x}${auth_key_actors+x}${auth_api_keys_url+x}${auth_api_keys_upstream+x}" ]]; then
      printf '{"auth":{"mode":"builtin"}}\n' > "$out"
      AUTH_VALUES_FILE="$out"
      return 0
    fi
    echo "Error: jq is required when .env sets auth_* keys other than auth_mode=builtin alone" >&2
    return 1
  fi
  for var in "${AUTH_ENV_KEYS[@]}"; do
    if [[ -z "${!var+x}" ]]; then
      continue
    fi
    val="${!var}"
    key="${var#auth_}"
    case "$var" in
      auth_authenticators|auth_group_mappings|auth_key_actors)
        if [[ -z "${val//[[:space:]]/}" ]]; then
          val='[]'
        fi
        if ! jq -e 'type == "array"' <<<"$val" &> /dev/null; then
          echo "Error: $var in .env is not a JSON array" >&2
          return 1
        fi
        json=$(jq -c --arg k "$key" --argjson v "$val" '.[$k] = $v' <<<"$json")
        ;;
      auth_mode)
        if [[ "$val" != "builtin" && "$val" != "external" ]]; then
          echo "Error: auth_mode in .env must be builtin or external" >&2
          return 1
        fi
        json=$(jq -c --arg k "$key" --arg v "$val" '.[$k] = $v' <<<"$json")
        ;;
      *)
        json=$(jq -c --arg k "$key" --arg v "$val" '.[$k] = $v' <<<"$json")
        ;;
    esac
  done
  # The apiserver refuses to start in external mode without a chain.
  if [[ "${auth_mode:-}" == "external" ]] && ! jq -e '(.authenticators // []) | length > 0' <<<"$json" &> /dev/null; then
    echo "Error: auth_mode=external in .env needs a non-empty auth_authenticators" >&2
    return 1
  fi
  jq -n --argjson a "$json" '{auth: $a}' > "$out"
  AUTH_VALUES_FILE="$out"
}

# check_live_sso refuses an upgrade that would silently undo the SSO settings of the running release
# that only .env can carry: helm renders sso.login and sso.bridge_allowlist from the chart defaults
# (off, empty) unless .env sets them, so a release that signs the console in at an OIDC provider
# would send everyone back to the other provider (and sign them out), and other web apps would lose
# the SSO bridge. sso_login_enable=false and sso_bridge_allowlist='[]' drop them on purpose.
check_live_sso() {
  local releases live state login allow missing=""
  if [[ -n "${sso_login_enable:-}" && -n "${sso_bridge_allowlist+x}" ]]; then
    return 0 # .env says both: nothing can be dropped
  fi
  if ! releases=$(helm -n "$NAMESPACE" list); then
    echo "Error: cannot list the helm releases in $NAMESPACE, so the SSO settings of the running release are unknown" >&2
    return 1
  fi
  if ! grep -q "^$chart_name " <<<"$releases"; then
    return 0 # nothing installed yet, nothing to lose
  fi
  if command -v jq &> /dev/null; then
    if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o json); then
      echo "Error: cannot read the values of release $chart_name, so its SSO settings are unknown" >&2
      return 1
    fi
    if ! state=$(jq -r '[((.sso.login.enable // false) | tostring) == "true", ((.sso.bridge_allowlist // []) | length > 0)] | map(tostring) | join(" ")' <<<"$live"); then
      echo "Error: cannot read the SSO settings from the values of release $chart_name" >&2
      return 1
    fi
  else
    if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o yaml); then
      echo "Error: cannot read the values of release $chart_name, so its SSO settings are unknown" >&2
      return 1
    fi
    # sso.login.enable, and whether sso.bridge_allowlist has an entry (inline, or a list on the next lines).
    if ! state=$(awk '
      /^sso:/ {s = 1; next}
      /^[^ #]/ {s = 0}
      !s {next}
      /^  [^ -]/ {l = 0; a = 0}
      /^  login:/ {l = 1; next}
      l && /^    enable: *"?true"?[[:space:]]*$/ {login = 1}
      /^  bridge_allowlist:/ {v = $0; sub(/^  bridge_allowlist:[[:space:]]*/, "", v); if (v == "") a = 1; else if (v != "[]" && v != "null") allow = 1; next}
      a && /^  - / {allow = 1}
      END {printf "%s %s\n", (login ? "true" : "false"), (allow ? "true" : "false")}' <<<"$live"); then
      echo "Error: cannot read the SSO settings from the values of release $chart_name" >&2
      return 1
    fi
  fi
  read -r login allow <<<"$state"
  if [[ "$login" == "true" && -z "${sso_login_enable:-}" ]]; then
    missing="$missing sso_login_enable sso_login_issuer sso_login_client_id sso_login_redirect_uri"
  fi
  if [[ "$allow" == "true" && -z "${sso_bridge_allowlist+x}" ]]; then
    missing="$missing sso_bridge_allowlist"
  fi
  if [[ -n "$missing" ]]; then
    echo "Error: release $chart_name is running with SSO settings that .env does not set:$missing" >&2
    echo "       Upgrading now would turn them off: the console would sign in at the other provider again (everyone" >&2
    echo "       signed out), or other web apps would lose the SSO bridge. Nothing has been changed." >&2
    echo "       Add them to .env (the live values are: helm -n $NAMESPACE get values $chart_name -o json | jq .sso)," >&2
    echo "       or set sso_login_enable=false / sso_bridge_allowlist='[]' to drop them on purpose." >&2
    return 1
  fi
}

# write_console_values adds console.return_prefixes (CONSOLE_RETURN_PREFIXES, see
# resolve_console_return_prefixes) to the extra values file AUTH_VALUES_FILE, writing it to $1 if no
# earlier step wrote one; nothing when it is unset, which leaves the apiserver's default list.
write_console_values() {
  local out="$1" base='{}'
  if [[ -z "$CONSOLE_RETURN_PREFIXES" ]]; then
    return 0
  fi
  if [[ -n "$AUTH_VALUES_FILE" ]]; then
    base=$(cat "$AUTH_VALUES_FILE")
    out="$AUTH_VALUES_FILE"
  fi
  jq -n --argjson b "$base" --argjson l "$CONSOLE_RETURN_PREFIXES" '$b * {console: {return_prefixes: $l}}' > "$out.tmp"
  mv "$out.tmp" "$out"
  AUTH_VALUES_FILE="$out"
}

# write_sso_values adds sso.bridge_allowlist from .env (checked above) to the extra values file
# AUTH_VALUES_FILE, writing it to $1 if write_auth_values wrote none; nothing when .env does not set it.
write_sso_values() {
  local out="$1" base='{}'
  if [[ -z "${sso_bridge_allowlist+x}" ]]; then
    return 0
  fi
  if [[ -n "$AUTH_VALUES_FILE" ]]; then
    base=$(cat "$AUTH_VALUES_FILE")
    out="$AUTH_VALUES_FILE"
  fi
  jq -n --argjson b "$base" --argjson l "$sso_bridge_allowlist" '$b * {sso: {bridge_allowlist: $l}}' > "$out.tmp"
  mv "$out.tmp" "$out"
  AUTH_VALUES_FILE="$out"
}

echo "====================================="
echo "🔧 Step 1: Load Parameters from .env"
echo "====================================="

if [ -f ".env" ]; then
  source .env
else
  echo "Error: .env file not found. pls execute install.sh first"
  exit 1
fi

echo "✅ Ethernet nic: \"$ethernet_nic\""
echo "✅ Rdma nic: \"$rdma_nic\""
echo "✅ Cluster Scale: \"$cluster_scale\""
echo "✅ Storage Class: \"$storage_class\""
lens_enable="false"
echo "✅ Robust data-plane: managed via AddonTemplate"
echo "✅ Support S3: \"$s3_enable\""
echo "✅ Support SSO: \"$sso_enable\""
echo "✅ SSO console sign-in at an OIDC provider (sso.login): \"${sso_login_enable:-unset}\""
echo "✅ SSO bridge allowlist: $([[ -n "${sso_bridge_allowlist+x}" ]] && echo set || echo unset)"
echo "✅ Console return prefixes: $([[ -n "${console_return_prefixes+x}" ]] && echo set || echo unset)"
echo "✅ Support Tracing: \"${tracing_enable:-false}\" (mode: ${tracing_mode:-error_only})"
echo "✅ Support LLM Gateway: \"${llm_gateway_enable:-false}\""
echo "✅ Grafana: enable=\"${grafana_enable:-true}\" (set grafana_enable=false in .env to disable)"
echo "✅ Ingress Name: \"$ingress\""
if [[ "$ingress" == "higress" ]]; then
  echo "✅ Cluster Name: \"$sub_domain\""
fi
echo "✅ Image Registry: \"$proxy_image_registry\""
echo "✅ Helm Registry: \"$helm_registry\""
echo "✅ CD Require Approval: \"$cd_require_approval\""
echo "✅ Install Node Agent: \"${install_node_agent:-y}\""
echo "✅ CSI Volume Handle: \"$csi_volume_handle\""
echo "✅ Node Agent GPU Driver: \"${node_agent_gpu_driver:-6.12.12}\""
echo "✅ Node Agent ROCm Version: \"${node_agent_rocm_version:-6.4}\""
echo "✅ Node Agent Toggles: net_bnxt_load_204=${node_agent_toggle_net_bnxt_load_204:-off}, net_ainic_load_205=${node_agent_toggle_net_ainic_load_205:-off}, net_ainic_devices_208=${node_agent_toggle_net_ainic_devices_208:-off}, sys_csi_wekafs_309=${node_agent_toggle_sys_csi_wekafs_309:-on}"
echo "✅ Auth mode: \"${auth_mode:-unset}\""

# The console's sign-in at an OpenID Connect provider (sso.login, see docs/installation/install.md),
# from .env like the rest. Written only when .env sets sso_login_enable, so a site without it keeps
# the chart default (off, the console signs in at the provider of the sso Secret).
#   sso_login_enable        true | false                                 -> sso.login.enable
#   sso_login_issuer        the provider's issuer                         -> sso.login.issuer
#   sso_login_client_id     the console's client id at the provider       -> sso.login.client_id
#   sso_login_redirect_uri  the console's own callback, as registered     -> sso.login.redirect_uri
#   sso_secret              the Secret of the other SSO provider (default
#                           primus-safe-sso); empty removes it            -> sso.secret
#   sso_bridge_allowlist    JSON array of the other web apps (origin, optionally
#                           with a path prefix) that may receive an SSO code
#                           through the console, e.g.
#                           sso_bridge_allowlist='["https://app.example.com"]' -> sso.bridge_allowlist
# Like auth_*, a release that already runs with sso.login on or a non-empty bridge allowlist refuses
# an upgrade whose .env leaves them out (check_live_sso); set them to false / '[]' to drop them on purpose.
SSO_LOGIN_ON="false"
if [[ -n "${sso_login_enable:-}" ]]; then
  if [[ "$sso_login_enable" != "true" && "$sso_login_enable" != "false" ]]; then
    echo "Error: sso_login_enable in .env must be true or false" >&2
    exit 1
  fi
  SSO_LOGIN_ON="$sso_login_enable"
  # The values are written with sed, '|' as the delimiter; '&' in a replacement is the matched text.
  unsafe_value='["|&\\[:space:]]'
  for key in sso_login_issuer sso_login_client_id sso_login_redirect_uri; do
    value="${!key:-}"
    if [[ "$sso_login_enable" == "true" && -z "$value" ]]; then
      echo "Error: sso_login_enable=true in .env needs $key" >&2
      exit 1
    fi
    if [[ "$value" =~ $unsafe_value ]]; then
      echo "Error: $key in .env must not contain spaces, quotes, backslashes, '|' or '&'" >&2
      exit 1
    fi
  done
fi
# sso.login is part of the SSO settings: with sso_enable=false the apiserver runs no SSO at all, and
# the console would not offer the sign-in that .env asks for.
if [[ "$SSO_LOGIN_ON" == "true" && "$sso_enable" != "true" ]]; then
  echo "Error: sso_login_enable=true in .env needs sso_enable=true" >&2
  exit 1
fi
if [[ "${sso_secret:-}" =~ [^a-z0-9.-] ]]; then
  echo "Error: sso_secret in .env must be the name of a Secret" >&2
  exit 1
fi
# Without the Secret, sso.login is the only provider the apiserver has; it refuses to start with neither.
if [[ "$sso_enable" == "true" && -n "${sso_secret+x}" && -z "$sso_secret" && "$SSO_LOGIN_ON" != "true" ]]; then
  echo "Error: sso_secret= (empty) in .env needs sso_login_enable=true: the apiserver would have no SSO provider" >&2
  exit 1
fi
if [[ -n "${sso_bridge_allowlist+x}" ]]; then
  if [[ -z "${sso_bridge_allowlist//[[:space:]]/}" ]]; then
    sso_bridge_allowlist='[]'
  fi
  if ! command -v jq &> /dev/null; then
    echo "Error: jq is required when .env sets sso_bridge_allowlist" >&2
    exit 1
  fi
  if ! jq -e 'type == "array" and all(.[]; type == "string" and length > 0)' <<<"$sso_bridge_allowlist" &> /dev/null; then
    echo "Error: sso_bridge_allowlist in .env is not a JSON array of non-empty strings" >&2
    exit 1
  fi
fi
# The path prefixes on the console's host that the console returns to after a sign-in another web
# app started (/login?redirect=<path>); see console.return_prefixes in the chart's values.yaml.
#   console_return_prefixes  JSON array of plain path prefixes, e.g.
#                            console_return_prefixes='["/dispatron","/autopilot","/binding","/blog-drafts","/claw-api"]'
#                            -> console.return_prefixes
# Left out, the running release keeps its list (resolve_console_return_prefixes), and a fresh install
# gets the apiserver's default; '[]' allows none.
if [[ -n "${console_return_prefixes+x}" ]]; then
  if [[ -z "${console_return_prefixes//[[:space:]]/}" ]]; then
    console_return_prefixes='[]'
  fi
  if ! command -v jq &> /dev/null; then
    echo "Error: jq is required when .env sets console_return_prefixes" >&2
    exit 1
  fi
  if ! jq -e 'type == "array" and all(.[]; type == "string" and test("^(/[A-Za-z0-9_~-][A-Za-z0-9._~-]*)+$"))' <<<"$console_return_prefixes" &> /dev/null; then
    echo "Error: console_return_prefixes in .env is not a JSON array of path prefixes such as \"/dispatron\"" >&2
    exit 1
  fi
fi

# resolve_user_dir_mounts sets USER_DIR_MOUNTS to the external_execution.user_dir_mounts the upgrade
# renders. external_user_dir_mounts in .env (true or false) wins. Without it the running release keeps
# its value: helm renders every value from the chart defaults and the files it is given, so a release
# whose switch was turned on (by hand or by an earlier .env) would go back to the chart's false on the
# next upgrade, and external Pods would again mount a hostpath volume whole, other users' directories
# included. A release that cannot be read stops the upgrade rather than guessing.
resolve_user_dir_mounts() {
  local releases live
  USER_DIR_MOUNTS=""
  if [[ -n "${external_user_dir_mounts:-}" ]]; then
    if [[ "$external_user_dir_mounts" != "true" && "$external_user_dir_mounts" != "false" ]]; then
      echo "Error: external_user_dir_mounts in .env is '$external_user_dir_mounts'; set it to true or false" >&2
      return 1
    fi
    USER_DIR_MOUNTS="$external_user_dir_mounts"
    return 0
  fi
  if ! releases=$(helm -n "$NAMESPACE" list); then
    echo "Error: cannot list the helm releases in $NAMESPACE, so external_execution.user_dir_mounts of the running release is unknown" >&2
    return 1
  fi
  if ! grep -q "^$chart_name " <<<"$releases"; then
    return 0 # nothing installed yet: the chart default applies
  fi
  if command -v jq &> /dev/null; then
    if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o json); then
      echo "Error: cannot read the values of release $chart_name, so its external_execution.user_dir_mounts is unknown" >&2
      return 1
    fi
    if ! USER_DIR_MOUNTS=$(jq -r '.external_execution.user_dir_mounts // empty | tostring' <<<"$live"); then
      echo "Error: cannot read external_execution.user_dir_mounts from the values of release $chart_name" >&2
      return 1
    fi
  else
    if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o yaml); then
      echo "Error: cannot read the values of release $chart_name, so its external_execution.user_dir_mounts is unknown" >&2
      return 1
    fi
    USER_DIR_MOUNTS=$(awk '/^external_execution:/ {a = 1; next} /^[^ #]/ {a = 0} a && /^  user_dir_mounts:/ {gsub(/["\047]/, "", $2); print $2; exit}' <<<"$live")
  fi
  # helm keeps a value set with --set-string as a string; anything but true or false is the chart default.
  if [[ "$USER_DIR_MOUNTS" != "true" && "$USER_DIR_MOUNTS" != "false" ]]; then
    USER_DIR_MOUNTS=""
  fi
}

# resolve_console_return_prefixes sets CONSOLE_RETURN_PREFIXES to the console.return_prefixes (JSON) the
# upgrade renders, or "" for the apiserver's default. console_return_prefixes in .env wins. Without it the
# running release keeps its list: helm renders every value from the chart defaults and the files it is
# given, so a list set by hand would be dropped by the next upgrade, and the apps added to it would land
# on the console's 404 page after sign-in again. Without jq a release that has a list cannot carry it
# over, so the upgrade stops and asks for the key in .env.
resolve_console_return_prefixes() {
  local releases live
  CONSOLE_RETURN_PREFIXES=""
  if [[ -n "${console_return_prefixes+x}" ]]; then
    CONSOLE_RETURN_PREFIXES="$console_return_prefixes"
    return 0
  fi
  if ! releases=$(helm -n "$NAMESPACE" list); then
    echo "Error: cannot list the helm releases in $NAMESPACE, so console.return_prefixes of the running release is unknown" >&2
    return 1
  fi
  if ! grep -q "^$chart_name " <<<"$releases"; then
    return 0 # nothing installed yet: the apiserver's default applies
  fi
  if command -v jq &> /dev/null; then
    if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o json); then
      echo "Error: cannot read the values of release $chart_name, so its console.return_prefixes is unknown" >&2
      return 1
    fi
    if ! CONSOLE_RETURN_PREFIXES=$(jq -c '.console.return_prefixes // empty | select(type == "array")' <<<"$live"); then
      echo "Error: cannot read console.return_prefixes from the values of release $chart_name" >&2
      return 1
    fi
    return 0
  fi
  if ! live=$(helm -n "$NAMESPACE" get values "$chart_name" -o yaml); then
    echo "Error: cannot read the values of release $chart_name, so its console.return_prefixes is unknown" >&2
    return 1
  fi
  if awk '/^console:/ {c = 1; next} /^[^ #]/ {c = 0} c && /^  return_prefixes:/ && !/: *null *$/ {found = 1} END {exit !found}' <<<"$live"; then
    echo "Error: release $chart_name runs with console.return_prefixes, which .env does not set, and without jq it" >&2
    echo "       cannot be carried over. Nothing has been changed. Add console_return_prefixes to .env" >&2
    echo "       (the live value is: helm -n $NAMESPACE get values $chart_name -o yaml, under console)." >&2
    return 1
  fi
}

# Before anything is changed: an upgrade must not silently turn the external authentication off.
check_live_auth
# ... and must not silently turn the console's sign-in at the OIDC provider, or the SSO bridge, off.
check_live_sso
# ... and must not silently turn the per-user directories of external Pods off.
resolve_user_dir_mounts || exit 1
# ... and must not silently drop the console's return prefixes.
resolve_console_return_prefixes || exit 1

echo

replicas=1
cpu=2000m
memory=4Gi
if [[ "$cluster_scale" == "medium" ]]; then
  replicas=2
  cpu=4000m
  memory=8Gi
elif [[ "$cluster_scale" == "large" ]]; then
  replicas=2
  cpu=8000m
  memory=16Gi
fi
IMAGE_PULL_SECRET="$NAMESPACE-image"
S3_SECRET="$NAMESPACE-s3"
SSO_SECRET="$NAMESPACE-sso"

echo
echo "========================================="
echo "🔧 Step 2: upgrade primus-safe admin plane"
echo "========================================="

cd ../charts/
src_values_yaml="primus-safe/values.yaml"
if [ ! -f "$src_values_yaml" ]; then
  echo "Error: $src_values_yaml does not exist"
  exit 1
fi
values_yaml="primus-safe/.values.yaml"
cp "$src_values_yaml" "${values_yaml}"

safe_image=$(printf '%s\n' "$proxy_image_registry" | sed 's/[&/\]/\\&/g')
sed -i '/global:/,/^[a-z]/ s/image_registry: .*/image_registry: "'"$safe_image"'"/' "$values_yaml"
sed -i "s/csi_volume_handle: \".*\"/csi_volume_handle: \"$csi_volume_handle\"/" "$values_yaml"

sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
sed -i "s/replicas: [0-9]*/replicas: $replicas/" "$values_yaml"
sed -i "s/^.*cpu:.*/  cpu: $cpu/" "$values_yaml"
sed -i "s/^.*memory:.*/  memory: $memory/" "$values_yaml"
sed -i "s/^.*storage_class:.*/  storage_class: \"$storage_class\"/" "$values_yaml"
sed -i '/opensearch:/,/^[a-z]/ s/enable: .*/enable: true/' "$values_yaml"
sed -i '/s3:/,/^[a-z]/ s/enable: .*/enable: '"$s3_enable"'/' "$values_yaml"
if [[ "$s3_enable" == "true" ]]; then
  sed -i '/^s3:/,/^[a-z]/ s#secret: ".*"#secret: "'"$S3_SECRET"'"#' "$values_yaml"
fi
# Grafana is required for the SaFE UI (training-workload dashboard, cluster
# GPU heatmap, log-based alerts overview). Default it to "on"; only opt-out
# when the operator explicitly sets grafana_enable=false in .env.
# The previous hard-coded `enable: false` was the reason every CD run kept
# deleting the live Grafana instance and left the /lens/grafana UI 504.
sed -i '/grafana:/,/^[a-z]/ s/enable: .*/enable: '"${grafana_enable:-true}"'/' "$values_yaml"
if [[ -n "${grafana_password:-}" ]]; then
  sed -i '/grafana:/,/^[a-z]/ s/password: .*/password: "'"$grafana_password"'"/' "$values_yaml"
fi
sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"
sed -i "s/ingress: \".*\"/ingress: \"$ingress\"/" "$values_yaml"
# Anchored at the indentation of sso's own keys: sso.login has an enable of its own.
sed -i '/sso:/,/^[a-z]/ s/^  enable: .*/  enable: '"$sso_enable"'/' "$values_yaml"
if [[ "$sso_enable" == "true" ]]; then
  sed -i '/^sso:/,/^[a-z]/ s#^  secret: ".*"#  secret: "'"${sso_secret-$SSO_SECRET}"'"#' "$values_yaml"
fi
if [[ -n "${sso_login_enable:-}" ]]; then
  sed -i -e '/^sso:/,/^[a-z]/ s|^    enable: .*|    enable: '"$sso_login_enable"'|' \
    -e '/^sso:/,/^[a-z]/ s|^    issuer: .*|    issuer: "'"${sso_login_issuer:-}"'"|' \
    -e '/^sso:/,/^[a-z]/ s|^    client_id: .*|    client_id: "'"${sso_login_client_id:-}"'"|' \
    -e '/^sso:/,/^[a-z]/ s|^    redirect_uri: .*|    redirect_uri: "'"${sso_login_redirect_uri:-}"'"|' "$values_yaml"
fi
sed -i '/^cd:/,/^[a-z]/ s/require_approval: .*/require_approval: '"$cd_require_approval"'/' "$values_yaml"

# Per-workspace optimization-task concurrency cap. Only override the chart
# default (1024) when optimize_max_concurrent is explicitly set in .env so a
# missing key leaves the default untouched.
if [[ -n "${optimize_max_concurrent:-}" ]]; then
  sed -i '/^model_optimization:/,/^[a-z]/ s/max_concurrent: .*/max_concurrent: '"$optimize_max_concurrent"'/' "$values_yaml"
fi

# Secret of extra CA certificates sandboxes must trust. Only set when
# sandbox_extra_ca_secret is in .env, so a site without it keeps the chart
# default and renders no extra CA.
if [[ -n "${sandbox_extra_ca_secret:-}" ]]; then
  sed -i '/^sandbox:/,/^[a-z]/ s/extra_ca_secret: .*/extra_ca_secret: "'"$sandbox_extra_ca_secret"'"/' "$values_yaml"
fi

# Secret whose keys sandboxes receive as environment variables. Synced whenever
# sandbox_env_secret is present in .env, so setting it empty turns the feature off.
if [[ -n "${sandbox_env_secret+x}" ]]; then
  # The only accepted name is the one the apiserver and resource-manager always reserve (the
  # chart refuses any other); checked here so values.yaml is never left holding one.
  if [[ -n "$sandbox_env_secret" && "$sandbox_env_secret" != "sandbox-env" ]]; then
    echo "Error: sandbox_env_secret '$sandbox_env_secret' is not supported: set it to sandbox-env or leave it empty" >&2
    exit 1
  fi
  sed -i '/^sandbox:/,/^[a-z]/ s/^  env_secret: .*/  env_secret: "'"$sandbox_env_secret"'"/' "$values_yaml"
fi

# Where the console's nginx redirects a browser opening the bare / of the console's host
# (web.root_redirect, e.g. /portal/). Only set when web_root_redirect is in .env, so a site
# without it keeps the console at /.
if [[ -n "${web_root_redirect:-}" ]]; then
  root_redirect=$(printf '%s\n' "$web_root_redirect" | sed 's/[&|\]/\\&/g')
  sed -i '/^web:/,/^[a-z]/ s|root_redirect: .*|root_redirect: "'"$root_redirect"'"|' "$values_yaml"
fi

# CICD runner concurrency (maxRunners per AutoscalingRunnerSet). Only override
# the chart default when cicd_max_runners is explicitly set in .env so a missing
# key leaves the chart default untouched. Takes effect for AutoscalingRunnerSets
# rendered/rebuilt after this deploy (job-manager re-renders from the template
# configmap); already-running runner sets keep their current cap until rebuild.
if [[ -n "${cicd_max_runners:-}" ]]; then
  sed -i '/^cicd:/,/^[a-z]/ s/max_runners: .*/max_runners: '"$cicd_max_runners"'/' "$values_yaml"
fi

# Prefer external_enable; keep external_execution_enabled as an alias.
if [[ -n "${external_enable:-${external_execution_enabled:-}}" ]]; then
  sed -i '/^external_execution:/,/^[a-z]/ s/enabled: .*/enabled: '"${external_enable:-$external_execution_enabled}"'/' "$values_yaml"
fi
if [[ -n "$USER_DIR_MOUNTS" ]]; then
  sed -i '/^external_execution:/,/^[a-z]/ s/user_dir_mounts: .*/user_dir_mounts: '"$USER_DIR_MOUNTS"'/' "$values_yaml"
fi

# Configure metrics port if defined in .env
if [[ -n "${metrics_port:-}" ]]; then
  sed -i '/^job_manager:/,/^[a-z]/ s/metrics_port: .*/metrics_port: '"$metrics_port"'/' "$values_yaml"
fi

# Configure tracing if defined in .env
if [[ "${tracing_enable:-false}" == "true" ]]; then
  sed -i '/^tracing:/,/^[a-z]/ s/enable: .*/enable: true/' "$values_yaml"
  if [[ -n "${tracing_mode:-}" ]]; then
    sed -i '/^tracing:/,/^[a-z]/ s/mode: .*/mode: "'"$tracing_mode"'"/' "$values_yaml"
  fi
  if [[ -n "${tracing_sampling_ratio:-}" ]]; then
    sed -i '/^tracing:/,/^[a-z]/ s/sampling_ratio: .*/sampling_ratio: "'"$tracing_sampling_ratio"'"/' "$values_yaml"
  fi
  if [[ -n "${tracing_otlp_endpoint:-}" ]]; then
    sed -i '/^tracing:/,/^[a-z]/ s#otlp_endpoint: .*#otlp_endpoint: "'"$tracing_otlp_endpoint"'"#' "$values_yaml"
  fi
fi

# Configure LLM Gateway secrets if defined in .env
if [[ -n "${llm_gateway_litellm_endpoint:-}" ]]; then
  sed -i "/^llm_gateway:/a\\
  litellm_endpoint: \"${llm_gateway_litellm_endpoint}\"\\
  litellm_admin_key: \"${llm_gateway_litellm_admin_key:-}\"\\
  litellm_team_id: \"${llm_gateway_litellm_team_id:-}\"\\
  shared_apim_key_hashes: \"${llm_gateway_shared_apim_key_hashes:-}\"\\
  shared_apim_user: \"${llm_gateway_shared_apim_user:-}\"" "$values_yaml"
fi

# Configure Langfuse proxy if defined in .env
if [[ "${langfuse_proxy_enable:-false}" == "true" ]]; then
  sed -i '/^langfuse_proxy:/,/^[a-z]/ s/enable: .*/enable: true/' "$values_yaml"
  if [[ -n "${langfuse_proxy_target:-}" ]]; then
    sed -i '/^langfuse_proxy:/,/^[a-z]/ s#target: .*#target: "'"$langfuse_proxy_target"'"#' "$values_yaml"
  fi
fi

# Configure proxy services if defined in .env
if [[ -n "${proxy_services:-}" ]]; then
  sed -i "/^proxy:/,/^[a-z_]*:/ { /^proxy:/! { /^[a-z_]*:/!d } }" "$values_yaml"
  sed -i "/^proxy:/a\\
  services: $proxy_services" "$values_yaml"
fi

write_auth_values "primus-safe/.auth-values.json"
write_sso_values "primus-safe/.sso-values.json"
write_console_values "primus-safe/.console-values.json"
if helm -n "$NAMESPACE" list | grep -q "^$chart_name "; then
  kubectl replace -f $chart_name/crds/ -n "$NAMESPACE" || kubectl create -f $chart_name/crds/ -n "$NAMESPACE"
  mkdir -p output
  helm template "$chart_name" -f "$values_yaml" -n "$NAMESPACE" "$chart_name" --output-dir ./output 1>/dev/null
  kubectl replace -f output/$chart_name/templates/rbac/role.yaml || kubectl create -f output/$chart_name/templates/rbac/role.yaml
  kubectl replace -f output/$chart_name/templates/webhooks/manifests.yaml || kubectl create -f output/$chart_name/templates/webhooks/manifests.yaml
  echo
  rm -rf output
fi
install_or_upgrade_helm_chart "$chart_name" "$values_yaml" "$AUTH_VALUES_FILE"
rm -f "$values_yaml" "$AUTH_VALUES_FILE"
sleep 10

echo
echo "========================================="
echo "🔧 Step 3: upgrade primus-safe cr"
echo "========================================="

src_values_yaml="primus-safe-cr/values.yaml"
if [ ! -f "$src_values_yaml" ]; then
  echo "Error: $src_values_yaml does not exist"
  exit 1
fi
values_yaml="primus-safe-cr/.values.yaml"
cp "$src_values_yaml" "${values_yaml}"

if [[ -n "${helm_registry:-}" ]]; then
  sed -i '/global:/,/^[a-z]/ s|helm_registry: .*|helm_registry: "'"$helm_registry"'"|' "$values_yaml"
fi
# proxy_image_registry may contain '/', so use '|' as the sed delimiter.
if [[ -n "${proxy_image_registry:-}" ]]; then
  sed -i '/global:/,/^[a-z]/ s|proxy_image_registry: .*|proxy_image_registry: "'"$proxy_image_registry"'"|' "$values_yaml"
fi
if [[ -n "${storage_class:-}" ]]; then
  sed -i '/global:/,/^[a-z]/ s|storage_class: .*|storage_class: "'"$storage_class"'"|' "$values_yaml"
fi
sed -i '/global:/,/^[a-z]/ s/sub_domain: .*/sub_domain: "'"$sub_domain"'"/' "$values_yaml"

install_or_upgrade_helm_chart "primus-safe-cr" "$values_yaml"
rm -f "$values_yaml"
cd ..


echo
echo "========================================="
echo "🔧 Step 4: upgrade primus-safe data plane"
echo "========================================="

if [[ "${CALLED_BY_CD:-false}" == "true" ]]; then
  echo "⏭️  Skipping node-agent upgrade (called by cd-deploy.sh)"
elif [[ "${install_node_agent:-y}" == "n" ]]; then
  echo "⏭️  Skipping node-agent upgrade (install_node_agent=n)"
else
  cd ./node-agent/charts/
  src_values_yaml="node-agent/values.yaml"
  if [ ! -f "$src_values_yaml" ]; then
    echo "Error: $src_values_yaml does not exist"
    exit 1
  fi
  values_yaml="node-agent/.values.yaml"
  cp "$src_values_yaml" "${values_yaml}"

  sed -i '/node_agent:/,/^[a-z]/ s/image_registry: .*/image_registry: "'"$safe_image"'"/' "$values_yaml"
  sed -i "s/nccl_socket_ifname: \".*\"/nccl_socket_ifname: \"$ethernet_nic\"/" "$values_yaml"
  sed -i "s/nccl_ib_hca: \".*\"/nccl_ib_hca: \"$rdma_nic\"/" "$values_yaml"
  sed -i "s/^.*sub_domain:.*/  sub_domain: \"$sub_domain\"/" "$values_yaml"
  sed -i "s/image_pull_secret: \".*\"/image_pull_secret: \"$IMAGE_PULL_SECRET\"/" "$values_yaml"
  sed -i "s/gpu_driver: \".*\"/gpu_driver: \"${node_agent_gpu_driver:-6.12.12}\"/" "$values_yaml"
  sed -i "s/rocm_version: \".*\"/rocm_version: \"${node_agent_rocm_version:-6.4}\"/" "$values_yaml"
  sed -i "s/net_bnxt_load_204: \".*\"/net_bnxt_load_204: \"${node_agent_toggle_net_bnxt_load_204:-off}\"/" "$values_yaml"
  sed -i "s/net_ainic_load_205: \".*\"/net_ainic_load_205: \"${node_agent_toggle_net_ainic_load_205:-off}\"/" "$values_yaml"
  sed -i "s/net_ainic_devices_208: \".*\"/net_ainic_devices_208: \"${node_agent_toggle_net_ainic_devices_208:-off}\"/" "$values_yaml"
  # WekaFS CSI container check defaults to "on" now that the script bug
  # (matched pod name, not container name) has been fixed upstream. Sites
  # that don't run WekaFS can disable it explicitly with
  # node_agent_toggle_sys_csi_wekafs_309=off in .env.
  sed -i "s/sys_csi_wekafs_309: \".*\"/sys_csi_wekafs_309: \"${node_agent_toggle_sys_csi_wekafs_309:-on}\"/" "$values_yaml"
  sed -i "s/disk_nfs_exist_check_402: \".*\"/disk_nfs_exist_check_402: \"${node_agent_toggle_disk_nfs_exist_check_402:-off}\"/" "$values_yaml"

  if [ -n "${node_agent_nfs_server:-}" ]; then
    sed -i "s|nfs_server: \".*\"|nfs_server: \"${node_agent_nfs_server}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_nfs_server_path:-}" ]; then
    sed -i "s|nfs_server_path: \".*\"|nfs_server_path: \"${node_agent_nfs_server_path}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_nfs_mount:-}" ]; then
    sed -i "s|nfs_mount: \".*\"|nfs_mount: \"${node_agent_nfs_mount}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_nfs_type:-}" ]; then
    sed -i "s|nfs_type: \".*\"|nfs_type: \"${node_agent_nfs_type}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_all_nfs_path:-}" ]; then
    sed -i "s|all_nfs_path: \".*\"|all_nfs_path: \"${node_agent_all_nfs_path}\"|" "$values_yaml"
  fi
  if [ -n "${node_agent_resolv_search_domain:-}" ]; then
    sed -i "s|resolv_search_domain: \".*\"|resolv_search_domain: \"${node_agent_resolv_search_domain}\"|" "$values_yaml"
  fi

  install_or_upgrade_helm_chart "node-agent" "$values_yaml"
  rm -f "$values_yaml"
fi

echo
echo "==============================="
echo "🔧 Step 5: All completed!"
echo "==============================="
