/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import "github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"

// githubRunnerFindNode locates the Node binary shipped with the runner image.
const githubRunnerFindNode = `find_runner_node() {
  find "${RUNNER_DIR}/externals" -type f -path '*/bin/node' 2>/dev/null | sort | head -n 1 || true
}
`

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
` + githubRunnerFindNode + `should_deregister() {
  [ -n "${POD_NAME:-}" ] || return 1
  TOKEN_FILE="/var/run/secrets/kubernetes.io/serviceaccount/token"
  CA_FILE="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
  NS_FILE="/var/run/secrets/kubernetes.io/serviceaccount/namespace"
  if [ ! -f "${TOKEN_FILE}" ] || [ ! -f "${NS_FILE}" ]; then
    echo "github runner deregistration: service account token or namespace is unavailable" >&2
    return 1
  fi
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
    echo "github runner deregistration: StatefulSet lookup returned HTTP ${CODE}" >&2
    return 1
  fi
  NODE_BIN="$(find_runner_node)"
  if [ -z "${NODE_BIN}" ] || [ ! -x "${NODE_BIN}" ]; then
    echo "github runner deregistration: node binary not found under ${RUNNER_DIR}/externals" >&2
    return 1
  fi
  STS_STATE="$("${NODE_BIN}" -e '
const fs = require("fs");
const statefulSet = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
if (statefulSet.metadata && statefulSet.metadata.deletionTimestamp) {
  process.stdout.write("deleting");
} else {
  const value = statefulSet.spec.replicas;
  process.stdout.write(String(value == null ? 1 : value));
}
' /tmp/github-runner-sts.json 2>/dev/null)"
  if [ "${STS_STATE}" = "deleting" ]; then
    return 0
  fi
  REPLICAS="${STS_STATE}"
  if [ -z "${REPLICAS}" ]; then
    echo "github runner deregistration: failed to read StatefulSet replicas" >&2
    return 1
  fi
  [ "${ORDINAL}" -ge "${REPLICAS}" ]
}
if should_deregister; then
  if [ ! -f "${RUNNER_DIR}/.credentials" ] || [ ! -f "${RUNNER_DIR}/.runner" ]; then
    echo "github runner deregistration: runner credentials are incomplete; keeping ${STATE_DIR}" >&2
    exit 0
  fi
  cd "${RUNNER_DIR}" && timeout 150 ./config.sh remove --unattended
  REMOVE_STATUS=$?
  if [ "${REMOVE_STATUS}" -eq 0 ]; then
    if [ -n "${GITHUB_RUNNER_STATE_ROOT:-}" ] && [ -n "${POD_NAME:-}" ]; then
      rm -rf "${STATE_DIR}"
    fi
  else
    echo "github runner deregister failed; keeping ${STATE_DIR}" >&2
  fi
fi
`
}
