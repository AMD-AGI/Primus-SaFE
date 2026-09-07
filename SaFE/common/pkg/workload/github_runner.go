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
STATE_DIR="${GITHUB_RUNNER_STATE_ROOT}/${POD_NAME}"
if [ -z "${GITHUB_RUNNER_STATE_ROOT:-}" ] || [ -z "${POD_NAME:-}" ]; then
  echo "GITHUB_RUNNER_STATE_ROOT and POD_NAME are required" >&2
  exit 1
fi
mkdir -p "${STATE_DIR}"
LABELS="${RUNNER_LABELS:-${DISPLAY_NAME}}"
TOKEN_FILE="` + common.SecretPath + `/${GITHUB_SECRET_ID}/github_token"
cd "${RUNNER_DIR}"
if [ ! -f "${STATE_DIR}/.credentials" ]; then
  TOKEN="$(cat "${TOKEN_FILE}")"
  ./config.sh --unattended --url "${GITHUB_CONFIG_URL}" --token "${TOKEN}" --name "${POD_NAME}" --labels "${LABELS}" --replace --work _work
  cp -f .credentials "${STATE_DIR}/.credentials"
  cp -f .runner "${STATE_DIR}/.runner"
  if [ -f .credentials_rsaparams ]; then
    cp -f .credentials_rsaparams "${STATE_DIR}/.credentials_rsaparams"
  fi
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
