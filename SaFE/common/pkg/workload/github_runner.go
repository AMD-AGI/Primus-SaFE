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
RUNNER_DIR="${RUNNER_DIR:-/home/runner}"
if [ -z "${GITHUB_RUNNER_STATE_ROOT:-}" ] || [ -z "${POD_NAME:-}" ]; then
  echo "GITHUB_RUNNER_STATE_ROOT and POD_NAME are required" >&2
  exit 1
fi
STATE_DIR="${GITHUB_RUNNER_STATE_ROOT}/${POD_NAME}"
mkdir -p "${STATE_DIR}"
LABELS="${RUNNER_LABELS:-${DISPLAY_NAME}}"
TOKEN_FILE="` + common.SecretPath + `/${GITHUB_SECRET_ID}/github_token"
if [ -n "${GITHUB_PROXY_URL:-}" ]; then
  PROXY_PASSWORD_FILE="` + common.SecretPath + `/${GITHUB_SECRET_ID}/github_proxy_password"
  NODE_BIN="$(find "${RUNNER_DIR}/externals" -type f -path '*/bin/node' | sort | head -n 1)"
  encode_proxy_component() {
    printf '%s' "$1" | "${NODE_BIN}" -e 'let s="";process.stdin.on("data",d=>s+=d);process.stdin.on("end",()=>process.stdout.write(encodeURIComponent(s)))'
  }
  PROXY_USER="$(encode_proxy_component "${GITHUB_PROXY_USERNAME}")"
  PROXY_PASSWORD="$(encode_proxy_component "$(cat "${PROXY_PASSWORD_FILE}")")"
  PROXY_SCHEME="${GITHUB_PROXY_URL%%://*}"
  PROXY_AUTHORITY="${GITHUB_PROXY_URL#*://}"
  PROXY="${PROXY_SCHEME}://${PROXY_USER}:${PROXY_PASSWORD}@${PROXY_AUTHORITY}"
  export http_proxy="${PROXY}" HTTP_PROXY="${PROXY}"
  export https_proxy="${PROXY}" HTTPS_PROXY="${PROXY}"
  export no_proxy="${GITHUB_PROXY_NO_PROXY:-}" NO_PROXY="${GITHUB_PROXY_NO_PROXY:-}"
fi
cd "${RUNNER_DIR}"
if [ ! -f "${STATE_DIR}/.credentials" ] || [ ! -f "${STATE_DIR}/.runner" ]; then
  TOKEN="$(cat "${TOKEN_FILE}")"
  ./config.sh --unattended --url "${GITHUB_CONFIG_URL}" --token "${TOKEN}" --name "${POD_NAME}" --labels "${LABELS}" --replace --work _work
  cp -f .runner "${STATE_DIR}/.runner.tmp"
  if [ -f .credentials_rsaparams ]; then
    cp -f .credentials_rsaparams "${STATE_DIR}/.credentials_rsaparams.tmp"
  fi
  cp -f .credentials "${STATE_DIR}/.credentials.tmp"
  mv -f "${STATE_DIR}/.runner.tmp" "${STATE_DIR}/.runner"
  if [ -f "${STATE_DIR}/.credentials_rsaparams.tmp" ]; then
    mv -f "${STATE_DIR}/.credentials_rsaparams.tmp" "${STATE_DIR}/.credentials_rsaparams"
  fi
  mv -f "${STATE_DIR}/.credentials.tmp" "${STATE_DIR}/.credentials"
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
  TOKEN_FILE="/var/run/secrets/kubernetes.io/serviceaccount/token"
  CA_FILE="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
  NS_FILE="/var/run/secrets/kubernetes.io/serviceaccount/namespace"
  [ -f "${TOKEN_FILE}" ] && [ -f "${NS_FILE}" ] || return 1
  NS="$(cat "${NS_FILE}")"
  ORDINAL="${POD_NAME##*-}"
  STS_NAME="${POD_NAME%-*}"
  HOST="${KUBERNETES_SERVICE_HOST:-kubernetes.default.svc}"
  PORT="${KUBERNETES_SERVICE_PORT:-443}"
  URL="https://${HOST}:${PORT}/apis/apps/v1/namespaces/${NS}/statefulsets/${STS_NAME}"
  CODE="$(curl -sS -o /tmp/github-runner-sts.json -w "%{http_code}" --cacert "${CA_FILE}" -H "Authorization: Bearer $(cat "${TOKEN_FILE}")" "${URL}" || echo 000)"
  if [ "${CODE}" = "404" ]; then
    return 0
  fi
  if [ "${CODE}" != "200" ]; then
    return 1
  fi
  REPLICAS="$(tr -d ' \n' < /tmp/github-runner-sts.json | sed -n 's/.*"spec":{"replicas":\([0-9][0-9]*\).*/\1/p')"
  [ -n "${REPLICAS}" ] || return 1
  [ "${ORDINAL}" -ge "${REPLICAS}" ]
}
if should_deregister; then
  cd "${RUNNER_DIR}" && ./config.sh remove --unattended || true
  if [ -n "${GITHUB_RUNNER_STATE_ROOT:-}" ] && [ -n "${POD_NAME:-}" ]; then
    rm -rf "${STATE_DIR}"
  fi
fi
`
}
