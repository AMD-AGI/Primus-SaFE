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
const githubRunnerProxySetup = `find_runner_node() {
  find "${RUNNER_DIR}/externals" -type f -path '*/bin/node' 2>/dev/null | sort | head -n 1 || true
}
setup_github_proxy() {
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
  RELAY_PID_FILE="${RELAY_DIR}/github-proxy-relay.pid"
  RELAY_PORT="${GITHUB_PROXY_RELAY_PORT:-3129}"
  if [ -f "${RELAY_PID_FILE}" ]; then
    RELAY_PID="$(cat "${RELAY_PID_FILE}")"
    if [ -n "${RELAY_PID}" ] && kill -0 "${RELAY_PID}" 2>/dev/null; then
      export http_proxy="http://127.0.0.1:${RELAY_PORT}" HTTP_PROXY="http://127.0.0.1:${RELAY_PORT}"
      export https_proxy="http://127.0.0.1:${RELAY_PORT}" HTTPS_PROXY="http://127.0.0.1:${RELAY_PORT}"
      return 0
    fi
  fi
  NODE_BIN="$(find_runner_node)"
  if [ -z "${NODE_BIN}" ] || [ ! -x "${NODE_BIN}" ]; then
    echo "github proxy relay: node binary not found under ${RUNNER_DIR}/externals" >&2
    return 1
  fi
  cat >"${RELAY_JS}" <<'RELAY_EOF'
const http = require('http');
const https = require('https');
const net = require('net');
const tls = require('tls');
const upstream = new URL(process.env.RELAY_UPSTREAM);
const credential = Buffer.from(
  process.env.RELAY_USER + ':' + process.env.RELAY_SECRET
).toString('base64');
const authorization = 'Basic ' + credential;
const connectTimeoutMs = 10000;
const requestTimeoutMs = 60000;

function upstreamPort() {
  if (upstream.port) {
    return Number(upstream.port);
  }
  return upstream.protocol === 'https:' ? 443 : 80;
}

function connectUpstream(onConnect) {
  const port = upstreamPort();
  const host = upstream.hostname;
  let connected = false;
  let socket;
  if (upstream.protocol === 'https:') {
    socket = tls.connect({ host: host, port: port, servername: host }, connectedCallback);
  } else {
    socket = net.connect(port, host, connectedCallback);
  }
  function connectedCallback() {
    connected = true;
    socket.setTimeout(0);
    onConnect();
  }
  socket.setTimeout(connectTimeoutMs, () => {
    if (!connected) {
      socket.destroy(new Error('upstream proxy connect timeout'));
    }
  });
  return socket;
}

function proxyHeaders(headers) {
  const result = { ...headers };
  delete result['proxy-authorization'];
  result['proxy-authorization'] = authorization;
  return result;
}

function relayConnect(request, client, body) {
  const server = connectUpstream(() => {
    const lines = [
      'CONNECT ' + request.url + ' HTTP/' + request.httpVersion,
      'Proxy-Authorization: ' + authorization,
    ];
    for (let i = 0; i < request.rawHeaders.length; i += 2) {
      if (request.rawHeaders[i].toLowerCase() !== 'proxy-authorization') {
        lines.push(request.rawHeaders[i] + ': ' + request.rawHeaders[i + 1]);
      }
    }
    server.write(lines.join('\r\n') + '\r\n\r\n');
    if (body.length > 0) {
      server.write(body);
    }
    client.pipe(server);
    server.pipe(client);
  });
  client.on('close', () => server.destroy());
  client.on('error', () => server.destroy());
  server.on('error', () => client.destroy());
  server.on('close', () => client.destroy());
}

const relay = http.createServer((request, response) => {
  const transport = upstream.protocol === 'https:' ? https : http;
  const upstreamRequest = transport.request({
    hostname: upstream.hostname,
    port: upstreamPort(),
    method: request.method,
    path: request.url,
    headers: proxyHeaders(request.headers),
    agent: false,
  }, (upstreamResponse) => {
    response.writeHead(upstreamResponse.statusCode, upstreamResponse.headers);
    upstreamResponse.pipe(response);
  });
  upstreamRequest.setTimeout(requestTimeoutMs, () => {
    upstreamRequest.destroy(new Error('upstream proxy request timeout'));
  });
  const closeUpstream = () => upstreamRequest.destroy();
  request.socket.once('close', closeUpstream);
  upstreamRequest.on('close', () => request.socket.removeListener('close', closeUpstream));
  upstreamRequest.on('error', () => {
    if (!response.headersSent) {
      response.writeHead(502);
    }
    response.end();
  });
  request.pipe(upstreamRequest);
});
relay.on('connect', relayConnect);
relay.on('clientError', (_error, socket) => socket.destroy());
relay.listen(Number(process.env.RELAY_PORT), '127.0.0.1', () => {
  console.log('github proxy relay listening');
});
RELAY_EOF
  rm -f "${RELAY_LOG}"
  RELAY_UPSTREAM="${GITHUB_PROXY_URL}" RELAY_PORT="${RELAY_PORT}" \
    RELAY_USER="${GITHUB_PROXY_USERNAME:-github}" RELAY_SECRET="${PROXY_SECRET}" \
    "${NODE_BIN}" "${RELAY_JS}" >"${RELAY_LOG}" 2>&1 &
  echo $! >"${RELAY_PID_FILE}"
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
  if [ -f "${STATE_DIR}/.register_failed" ]; then
    FAILED_SECRET_ID="$(cat "${STATE_DIR}/.register_failed" 2>/dev/null || true)"
    if [ "${FAILED_SECRET_ID}" = "${GITHUB_SECRET_ID}" ]; then
      echo "github runner registration already failed for the current secret; patch a new githubAuth.token" >&2
      exit 1
    fi
    rm -f "${STATE_DIR}/.register_failed"
  fi
  TOKEN="$(cat "${TOKEN_FILE}")"
  if ! ./config.sh --unattended --url "${GITHUB_CONFIG_URL}" --token "${TOKEN}" --name "${POD_NAME}" --labels "${LABELS}" --replace --work _work; then
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
  NODE_BIN="$(find_runner_node)"
  if [ -z "${NODE_BIN}" ] || [ ! -x "${NODE_BIN}" ]; then
    echo "github runner deregistration: node binary not found under ${RUNNER_DIR}/externals" >&2
    return 1
  fi
  REPLICAS="$("${NODE_BIN}" -e '
const fs = require("fs");
const value = JSON.parse(fs.readFileSync(process.argv[1], "utf8")).spec.replicas;
process.stdout.write(String(value == null ? 1 : value));
' /tmp/github-runner-sts.json 2>/dev/null)"
  if [ -z "${REPLICAS}" ]; then
    echo "github runner deregistration: failed to read StatefulSet replicas" >&2
    return 1
  fi
  [ "${ORDINAL}" -ge "${REPLICAS}" ]
}
if should_deregister; then
  if ! setup_github_proxy; then
    echo "github proxy setup failed; keeping ${STATE_DIR}" >&2
  else
    cd "${RUNNER_DIR}" && ./config.sh remove --unattended
    REMOVE_STATUS=$?
    if [ "${REMOVE_STATUS}" -eq 0 ]; then
      if [ -n "${GITHUB_RUNNER_STATE_ROOT:-}" ] && [ -n "${POD_NAME:-}" ]; then
        rm -rf "${STATE_DIR}"
      fi
    else
      echo "github runner deregister failed; keeping ${STATE_DIR}" >&2
    fi
  fi
fi
`
}
