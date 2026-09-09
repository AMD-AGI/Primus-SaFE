/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestGithubRunnerProxySetupFailsClosed(t *testing.T) {
	start := GithubRunnerStartScript()
	stop := GithubRunnerStopScript()
	assert.Assert(t, strings.Contains(start, "node binary not found"))
	assert.Assert(t, strings.Contains(start, "failed to listen"))
	assert.Assert(t, strings.Contains(start, "github-proxy-relay.pid"))
	assert.Assert(t, strings.Contains(start, "tls.connect"))
	assert.Assert(t, strings.Contains(start, "protocol === 'https:' ? 443 : 80"))
	assert.Assert(t, strings.Contains(start, "http.createServer"))
	assert.Assert(t, strings.Contains(start, "Proxy-Authorization"))
	assert.Assert(t, strings.Contains(start, "client.on('close'"))
	assert.Assert(t, strings.Contains(start, "setTimeout"))
	assert.Assert(t, strings.Contains(start, ".register_failed"))
	assert.Assert(t, strings.Contains(start, "FAILED_SECRET_ID"))
	assert.Assert(t, strings.Contains(stop, "node binary not found"))
	assert.Assert(t, strings.Contains(stop, "kill -0"))
	assert.Assert(t, strings.Contains(stop, "JSON.parse"))
	assert.Assert(t, strings.Contains(stop, "keeping ${STATE_DIR}"))
}

// TestGithubRunnerProxyRelaySyntax verifies the generated relay remains valid JavaScript.
func TestGithubRunnerProxyRelaySyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	script := GithubRunnerStartScript()
	start := strings.Index(script, "const http = require('http');")
	assert.Assert(t, start >= 0)
	end := strings.Index(script[start:], "\nRELAY_EOF")
	assert.Assert(t, end > 0)
	path := filepath.Join(t.TempDir(), "github-proxy-relay.js")
	assert.NilError(t, os.WriteFile(path, []byte(script[start:start+end]), 0o600))
	output, err := exec.Command(node, "--check", path).CombinedOutput()
	assert.NilError(t, err, string(output))
}
