/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestGithubRunnerScriptsKeepRegistrationGuards(t *testing.T) {
	start := GithubRunnerStartScript()
	stop := GithubRunnerStopScript()
	assert.Assert(t, strings.Contains(start, ".register_failed"))
	assert.Assert(t, strings.Contains(start, "FAILED_SECRET_ID"))
	assert.Assert(t, !strings.Contains(start, `TOKEN="$(cat "${TOKEN_FILE}")"`))
	assert.Assert(t, strings.Contains(start, `--token "$(cat "${TOKEN_FILE}")"`))
	assert.Assert(t, strings.Contains(start, `chmod 700 "${STATE_DIR}"`))
	assert.Assert(t, strings.Contains(start, `LABELS="${RUNNER_LABELS:-${DISPLAY_NAME:-}}"`))
	assert.Assert(t, strings.Contains(stop, "python3 and jq are unavailable"))
	assert.Assert(t, strings.Contains(stop, "service account token or namespace is unavailable"))
	assert.Assert(t, strings.Contains(stop, "StatefulSet lookup returned HTTP"))
	assert.Assert(t, strings.Contains(stop, "runner credentials are incomplete"))
	assert.Assert(t, strings.Contains(stop, "timeout 150"))
	assert.Assert(t, strings.Contains(stop, "keeping ${STATE_DIR}"))
	assert.Assert(t, !strings.Contains(start, "github-proxy-relay"))
	assert.Assert(t, !strings.Contains(stop, "setup_github_proxy"))
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	assert.NilError(t, os.WriteFile(path, []byte(content), 0o755))
}

func TestGithubRunnerStartScriptCreatesPrivateStateAndRegistersAgain(t *testing.T) {
	root := t.TempDir()
	runnerDir := filepath.Join(root, "runner")
	stateRoot := filepath.Join(root, "state")
	secretRoot := filepath.Join(root, "secrets")
	assert.NilError(t, os.MkdirAll(filepath.Join(secretRoot, "runner-secret"), 0o755))
	assert.NilError(t, os.MkdirAll(runnerDir, 0o755))
	assert.NilError(t, os.WriteFile(
		filepath.Join(secretRoot, "runner-secret", "github_token"), []byte("token"), 0o600))
	registerLog := filepath.Join(root, "register.log")
	writeExecutable(t, filepath.Join(runnerDir, "config.sh"), `#!/bin/sh
printf '%s\n' "$*" >"${REGISTER_LOG}"
printf '{}\n' >.runner
printf '{}\n' >.credentials
printf '{}\n' >.credentials_rsaparams
`)
	writeExecutable(t, filepath.Join(runnerDir, "run.sh"), "#!/bin/sh\nexit 0\n")

	cmd := exec.Command("/bin/sh", "-c", GithubRunnerStartScript())
	cmd.Env = append(os.Environ(),
		"RUNNER_DIR="+runnerDir,
		"GITHUB_RUNNER_STATE_ROOT="+stateRoot,
		"GITHUB_SECRET_ROOT="+secretRoot,
		"GITHUB_SECRET_ID=runner-secret",
		"GITHUB_CONFIG_URL=https://github.com/test/repo",
		"RUNNER_LABELS=pool",
		"POD_NAME=runner-0",
		"REGISTER_LOG="+registerLog,
	)
	output, err := cmd.CombinedOutput()
	assert.NilError(t, err, string(output))

	info, err := os.Stat(filepath.Join(stateRoot, "runner-0"))
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm(), os.FileMode(0o700))
	assert.Assert(t, strings.Contains(string(mustReadFile(t, registerLog)), "--token token"))
	for _, name := range []string{".credentials", ".runner", ".credentials_rsaparams"} {
		path := filepath.Join(stateRoot, "runner-0", name)
		assert.Assert(t, fileExists(path))
		info, err = os.Stat(path)
		assert.NilError(t, err)
		assert.Equal(t, info.Mode().Perm(), os.FileMode(0o600))
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	assert.NilError(t, err)
	return data
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runGithubRunnerStopScript(t *testing.T, code, body, podName string) (string, error, string, string) {
	t.Helper()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	runnerDir := filepath.Join(root, "runner")
	stateRoot := filepath.Join(root, "state")
	serviceAccountRoot := filepath.Join(root, "serviceaccount")
	assert.NilError(t, os.MkdirAll(binDir, 0o755))
	assert.NilError(t, os.MkdirAll(runnerDir, 0o755))
	assert.NilError(t, os.MkdirAll(filepath.Join(stateRoot, podName), 0o700))
	assert.NilError(t, os.MkdirAll(serviceAccountRoot, 0o755))
	for name, value := range map[string]string{
		"token": "token", "ca.crt": "ca", "namespace": "test-namespace",
	} {
		assert.NilError(t, os.WriteFile(filepath.Join(serviceAccountRoot, name), []byte(value), 0o600))
	}
	assert.NilError(t, os.WriteFile(filepath.Join(runnerDir, ".credentials"), []byte("{}"), 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(runnerDir, ".runner"), []byte("{}"), 0o600))
	bodyFile := filepath.Join(root, "response.json")
	assert.NilError(t, os.WriteFile(bodyFile, []byte(body), 0o600))
	removeLog := filepath.Join(root, "remove.log")
	writeExecutable(t, filepath.Join(runnerDir, "config.sh"), `#!/bin/sh
printf '%s\n' "$*" >"${REMOVE_LOG}"
`)
	writeExecutable(t, filepath.Join(binDir, "sleep"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(binDir, "curl"), `#!/bin/sh
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    output="$1"
  fi
  shift
done
if [ -n "${output}" ]; then
  cp "${CURL_BODY}" "${output}"
fi
printf '%s' "${CURL_CODE}"
exit "${CURL_EXIT:-0}"
`)

	cmd := exec.Command("/bin/sh", "-c", GithubRunnerStopScript())
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"RUNNER_DIR="+runnerDir,
		"GITHUB_RUNNER_STATE_ROOT="+stateRoot,
		"KUBERNETES_SERVICEACCOUNT_ROOT="+serviceAccountRoot,
		"KUBERNETES_SERVICE_HOST=kubernetes.test",
		"KUBERNETES_SERVICE_PORT=443",
		"POD_NAME="+podName,
		"CURL_CODE="+code,
		"CURL_BODY="+bodyFile,
		"REMOVE_LOG="+removeLog,
	)
	output, err := cmd.CombinedOutput()
	return string(output), err, removeLog, filepath.Join(stateRoot, podName)
}

func TestGithubRunnerStopScriptDeregistrationDecisions(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		body      string
		podName   string
		remove    bool
		wantError bool
	}{
		{name: "missing StatefulSet", code: "404", body: `{}`, podName: "runner-0", remove: true},
		{name: "deleting StatefulSet", code: "200",
			body:    `{"metadata":{"deletionTimestamp":"2026-09-11T00:00:00Z"},"spec":{"replicas":1}}`,
			podName: "runner-0", remove: true},
		{name: "ordinal removed by scale down", code: "200",
			body: `{"metadata":{},"spec":{"replicas":2}}`, podName: "runner-2", remove: true},
		{name: "ordinal retained by restart", code: "200",
			body: `{"metadata":{},"spec":{"replicas":2}}`, podName: "runner-1"},
		{name: "lookup failure", code: "000", body: `{}`, podName: "runner-0", wantError: true},
		{name: "invalid response", code: "200", body: `{not-json`, podName: "runner-0", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err, removeLog, stateDir := runGithubRunnerStopScript(
				t, tt.code, tt.body, tt.podName)
			if tt.wantError {
				assert.Assert(t, err != nil, output)
			} else {
				assert.NilError(t, err, output)
			}
			assert.Equal(t, fileExists(removeLog), tt.remove)
			assert.Equal(t, fileExists(stateDir), !tt.remove)
			if tt.remove {
				assert.Equal(t, strings.TrimSpace(string(mustReadFile(t, removeLog))),
					"remove --unattended")
			}
		})
	}
}

func TestGithubRunnerStopScriptRetriesLookup(t *testing.T) {
	output, err, _, _ := runGithubRunnerStopScript(t, "500", `{}`, "runner-0")
	assert.Assert(t, err != nil)
	for attempt := 1; attempt <= 3; attempt++ {
		assert.Assert(t, strings.Contains(output,
			fmt.Sprintf("lookup attempt %d returned HTTP 500", attempt)))
	}
}
