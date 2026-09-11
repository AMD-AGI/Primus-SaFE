/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import "github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"

// GithubRunnerStartScript registers the runner on first boot and then listens for jobs.
// Credentials are copied to workspace storage so a pod restart does not need a new token.
func GithubRunnerStartScript() string {
	return `set -eu
umask 077
RUNNER_DIR="${RUNNER_DIR:-/home/runner}"
if [ -z "${GITHUB_RUNNER_STATE_ROOT:-}" ] || [ -z "${POD_NAME:-}" ]; then
  echo "GITHUB_RUNNER_STATE_ROOT and POD_NAME are required" >&2
  exit 1
fi
STATE_DIR="${GITHUB_RUNNER_STATE_ROOT}/${POD_NAME}"
mkdir -p "${STATE_DIR}"
chmod 700 "${STATE_DIR}"
LABELS="${RUNNER_LABELS:-${DISPLAY_NAME}}"
SECRET_ROOT="${GITHUB_SECRET_ROOT:-` + common.SecretPath + `}"
TOKEN_FILE="${SECRET_ROOT}/${GITHUB_SECRET_ID}/github_token"
cd "${RUNNER_DIR}"
if [ ! -f "${STATE_DIR}/.credentials" ] || [ ! -f "${STATE_DIR}/.runner" ]; then
  if [ -f "${STATE_DIR}/.register_failed" ]; then
    FAILED_SECRET_ID="$(cat "${STATE_DIR}/.register_failed" 2>/dev/null || true)"
    if [ "${FAILED_SECRET_ID}" = "${GITHUB_SECRET_ID}" ]; then
      echo "github runner registration already failed for the current secret; patch a new githubAuth.token" >&2
      exit 1
    fi
    rm -f "${STATE_DIR}/.register_failed"
  fi
  if ! ./config.sh --unattended --url "${GITHUB_CONFIG_URL}" --token "$(cat "${TOKEN_FILE}")" --name "${POD_NAME}" --labels "${LABELS}" --replace --work _work; then
    printf '%s\n' "${GITHUB_SECRET_ID}" >"${STATE_DIR}/.register_failed"
    echo "github runner registration failed" >&2
    exit 1
  fi
  cp -f .runner "${STATE_DIR}/.runner.tmp"
  if [ -f .credentials_rsaparams ]; then
    cp -f .credentials_rsaparams "${STATE_DIR}/.credentials_rsaparams.tmp"
  fi
  cp -f .credentials "${STATE_DIR}/.credentials.tmp"
  chmod 600 "${STATE_DIR}/.runner.tmp" "${STATE_DIR}/.credentials.tmp"
  if [ -f "${STATE_DIR}/.credentials_rsaparams.tmp" ]; then
    chmod 600 "${STATE_DIR}/.credentials_rsaparams.tmp"
  fi
  mv -f "${STATE_DIR}/.runner.tmp" "${STATE_DIR}/.runner"
  if [ -f "${STATE_DIR}/.credentials_rsaparams.tmp" ]; then
    mv -f "${STATE_DIR}/.credentials_rsaparams.tmp" "${STATE_DIR}/.credentials_rsaparams"
  fi
  mv -f "${STATE_DIR}/.credentials.tmp" "${STATE_DIR}/.credentials"
  rm -f "${STATE_DIR}/.register_failed"
else
  cp -f "${STATE_DIR}/.credentials" "${RUNNER_DIR}/.credentials"
  cp -f "${STATE_DIR}/.runner" "${RUNNER_DIR}/.runner"
  if [ -f "${STATE_DIR}/.credentials_rsaparams" ]; then
    cp -f "${STATE_DIR}/.credentials_rsaparams" "${RUNNER_DIR}/.credentials_rsaparams"
  fi
fi
exec ./run.sh
`
}

// GithubRunnerStopScript unregisters the runner when this ordinal is leaving
// the pool. Rolling restarts keep the ordinal and must not remove credentials.
func GithubRunnerStopScript() string {
	return `set +e
RUNNER_DIR="${RUNNER_DIR:-/home/runner}"
STATE_DIR="${GITHUB_RUNNER_STATE_ROOT:-}/${POD_NAME:-}"
should_deregister() {
  [ -n "${POD_NAME:-}" ] || return 1
  SERVICEACCOUNT_ROOT="${KUBERNETES_SERVICEACCOUNT_ROOT:-/var/run/secrets/kubernetes.io/serviceaccount}"
  TOKEN_FILE="${SERVICEACCOUNT_ROOT}/token"
  CA_FILE="${SERVICEACCOUNT_ROOT}/ca.crt"
  NS_FILE="${SERVICEACCOUNT_ROOT}/namespace"
  if [ ! -f "${TOKEN_FILE}" ] || [ ! -f "${NS_FILE}" ]; then
    echo "github runner deregistration: service account token or namespace is unavailable" >&2
    return 2
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "github runner deregistration: curl is unavailable" >&2
    return 2
  fi
  NS="$(cat "${NS_FILE}")"
  ORDINAL="${POD_NAME##*-}"
  case "${ORDINAL}" in
    ''|*[!0-9]*)
      echo "github runner deregistration: invalid pod ordinal ${ORDINAL}" >&2
      return 2
      ;;
  esac
  STS_NAME="${POD_NAME%-*}"
  HOST="${KUBERNETES_SERVICE_HOST:-kubernetes.default.svc}"
  PORT="${KUBERNETES_SERVICE_PORT:-443}"
  URL="https://${HOST}:${PORT}/apis/apps/v1/namespaces/${NS}/statefulsets/${STS_NAME}"
  STS_FILE="/tmp/github-runner-sts.json"
  CODE=""
  ATTEMPT=1
  while [ "${ATTEMPT}" -le 3 ]; do
    CODE="$(curl -sS --connect-timeout 3 --max-time 10 -o "${STS_FILE}" -w "%{http_code}" \
      --cacert "${CA_FILE}" -H "Authorization: Bearer $(cat "${TOKEN_FILE}")" "${URL}")"
    CURL_STATUS=$?
    if [ "${CURL_STATUS}" -eq 0 ] && [ "${CODE}" = "404" ]; then
      return 0
    fi
    if [ "${CURL_STATUS}" -eq 0 ] && [ "${CODE}" = "200" ]; then
      break
    fi
    echo "github runner deregistration: StatefulSet lookup attempt ${ATTEMPT} returned HTTP ${CODE:-000}" >&2
    if [ "${ATTEMPT}" -lt 3 ]; then
      sleep 2
    fi
    ATTEMPT=$((ATTEMPT + 1))
  done
  if [ "${CODE}" != "200" ]; then
    echo "github runner deregistration: StatefulSet lookup returned HTTP ${CODE}" >&2
    return 2
  fi
  if ! command -v jq >/dev/null 2>&1; then
    echo "github runner deregistration: jq is unavailable" >&2
    return 2
  fi
  STS_STATE="$(jq -er '
    if .metadata.deletionTimestamp != null then
      "deleting"
    elif .spec.replicas == null then
      "1"
    elif (.spec.replicas | type) == "number" then
      (.spec.replicas | tostring)
    else
      error("invalid StatefulSet replicas")
    end
  ' "${STS_FILE}" 2>/dev/null)"
  if [ $? -ne 0 ]; then
    echo "github runner deregistration: failed to parse StatefulSet state" >&2
    return 2
  fi
  if [ "${STS_STATE}" = "deleting" ]; then
    return 0
  fi
  REPLICAS="${STS_STATE}"
  if [ -z "${REPLICAS}" ]; then
    echo "github runner deregistration: failed to read StatefulSet replicas" >&2
    return 2
  fi
  [ "${ORDINAL}" -ge "${REPLICAS}" ]
}
should_deregister
DEREGISTER_DECISION=$?
if [ "${DEREGISTER_DECISION}" -eq 0 ]; then
  if [ ! -f "${RUNNER_DIR}/.credentials" ] || [ ! -f "${RUNNER_DIR}/.runner" ]; then
    echo "github runner deregistration: runner credentials are incomplete; keeping ${STATE_DIR}" >&2
    exit 1
  fi
  cd "${RUNNER_DIR}" && timeout 150 ./config.sh remove --unattended
  REMOVE_STATUS=$?
  if [ "${REMOVE_STATUS}" -eq 0 ]; then
    if [ -n "${GITHUB_RUNNER_STATE_ROOT:-}" ] && [ -n "${POD_NAME:-}" ]; then
      rm -rf "${STATE_DIR}"
    fi
  else
    echo "github runner deregister failed; keeping ${STATE_DIR}" >&2
    exit "${REMOVE_STATUS}"
  fi
elif [ "${DEREGISTER_DECISION}" -ne 1 ]; then
  exit 1
fi
`
}
