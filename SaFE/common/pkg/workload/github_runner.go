/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import "github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"

// githubRunnerProxySetup defines setup_github_proxy, which points the runner at
// the GitHub proxy. An authenticated proxy is reached through a local relay:
// the runner's .NET client only sends Proxy-Authorization after a 407 challenge,
// while the tunnel rejects the unauthenticated CONNECT with a 401 instead.
const githubRunnerProxySetup = `setup_github_proxy() {
  [ -n "${GITHUB_PROXY_URL:-}" ] || return 0
  export no_proxy="${GITHUB_PROXY_NO_PROXY:-localhost,127.0.0.1,::1,.svc,.cluster.local}"
  export NO_PROXY="${no_proxy}"
  PROXY_SECRET_FILE="` + common.SecretPath + `/${GITHUB_SECRET_ID:-}/github_proxy_password"
  PROXY_SECRET="${GITHUB_PROXY_PASSWORD:-}"
  if [ -z "${PROXY_SECRET}" ] && [ -f "${PROXY_SECRET_FILE}" ]; then
    PROXY_SECRET="$(cat "${PROXY_SECRET_FILE}")"
  fi
  if [ -z "${PROXY_SECRET}" ]; then
    export http_proxy="${GITHUB_PROXY_URL}" HTTP_PROXY="${GITHUB_PROXY_URL}"
    export https_proxy="${GITHUB_PROXY_URL}" HTTPS_PROXY="${GITHUB_PROXY_URL}"
    return 0
  fi
  RELAY_DIR="${RUNNER_TEMP:-/tmp}"
  RELAY_JS="${RELAY_DIR}/github-proxy-relay.js"
  RELAY_LOG="${RELAY_DIR}/github-proxy-relay.log"
  RELAY_PORT="${GITHUB_PROXY_RELAY_PORT:-3129}"
  NODE_BIN="$(find "${RUNNER_DIR}/externals" -type f -path '*/bin/node' 2>/dev/null | sort | head -n 1 || true)"
  if [ -z "${NODE_BIN}" ] || [ ! -x "${NODE_BIN}" ]; then
    echo "github proxy relay: node binary not found under ${RUNNER_DIR}/externals" >&2
    return 1
  fi
  cat >"${RELAY_JS}" <<'RELAY_EOF'
const net = require('net');
const upstream = new URL(process.env.RELAY_UPSTREAM);
const upstreamPort = Number(upstream.port) || 3128;
const credential = Buffer.from(
  process.env.RELAY_USER + ':' + process.env.RELAY_SECRET
).toString('base64');

function relay(client, header, body) {
  const lines = header
    .split('\r\n')
    .filter((line) => !/^proxy-authorization:/i.test(line));
  lines.splice(1, 0, 'Proxy-Authorization: Basic ' + credential);
  const server = net.connect(upstreamPort, upstream.hostname, () => {
    server.write(lines.join('\r\n') + '\r\n\r\n');
    if (body.length > 0) {
      server.write(body);
    }
    client.pipe(server);
    server.pipe(client);
    client.resume();
  });
  server.on('error', () => client.destroy());
}

net.createServer((client) => {
  let head = Buffer.alloc(0);
  const onData = (chunk) => {
    head = Buffer.concat([head, chunk]);
    const end = head.indexOf('\r\n\r\n');
    if (end < 0) {
      if (head.length > 65536) {
        client.destroy();
      }
      return;
    }
    client.pause();
    client.removeListener('data', onData);
    relay(client, head.slice(0, end).toString('latin1'), head.slice(end + 4));
  };
  client.on('data', onData);
  client.on('error', () => client.destroy());
}).listen(Number(process.env.RELAY_PORT), '127.0.0.1', () => {
  console.log('github proxy relay listening');
});
RELAY_EOF
  rm -f "${RELAY_LOG}"
  RELAY_UPSTREAM="${GITHUB_PROXY_URL}" RELAY_PORT="${RELAY_PORT}" \
    RELAY_USER="${GITHUB_PROXY_USERNAME:-github}" RELAY_SECRET="${PROXY_SECRET}" \
    "${NODE_BIN}" "${RELAY_JS}" >"${RELAY_LOG}" 2>&1 &
  RELAY_WAIT=0
  while [ "${RELAY_WAIT}" -lt 10 ]; do
    if grep -q 'github proxy relay listening' "${RELAY_LOG}" 2>/dev/null; then
      break
    fi
    RELAY_WAIT=$((RELAY_WAIT + 1))
    sleep 1
  done
  if ! grep -q 'github proxy relay listening' "${RELAY_LOG}" 2>/dev/null; then
    echo "github proxy relay failed to listen on 127.0.0.1:${RELAY_PORT}" >&2
    if [ -f "${RELAY_LOG}" ]; then
      cat "${RELAY_LOG}" >&2 || true
    fi
    return 1
  fi
  export http_proxy="http://127.0.0.1:${RELAY_PORT}" HTTP_PROXY="http://127.0.0.1:${RELAY_PORT}"
  export https_proxy="http://127.0.0.1:${RELAY_PORT}" HTTPS_PROXY="http://127.0.0.1:${RELAY_PORT}"
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
` + githubRunnerProxySetup + `setup_github_proxy
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
` + githubRunnerProxySetup + `should_deregister() {
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
  setup_github_proxy
  cd "${RUNNER_DIR}" && ./config.sh remove --unattended || true
  if [ -n "${GITHUB_RUNNER_STATE_ROOT:-}" ] && [ -n "${POD_NAME:-}" ]; then
    rm -rf "${STATE_DIR}"
  fi
fi
`
}
