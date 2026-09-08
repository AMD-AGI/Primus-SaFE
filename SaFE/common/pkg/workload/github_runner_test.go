/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"strings"
	"testing"

	"gotest.tools/assert"
)

func TestGithubRunnerProxySetupFailsClosed(t *testing.T) {
	start := GithubRunnerStartScript()
	stop := GithubRunnerStopScript()
	assert.Assert(t, strings.Contains(start, "node binary not found"))
	assert.Assert(t, strings.Contains(start, "failed to listen"))
	assert.Assert(t, strings.Contains(start, "client.pause()"))
	assert.Assert(t, strings.Contains(start, "client.resume()"))
	assert.Assert(t, strings.Contains(start, "github-proxy-relay.pid"))
	assert.Assert(t, strings.Contains(start, "tls.connect"))
	assert.Assert(t, strings.Contains(start, "protocol === 'https:' ? 443 : 80"))
	assert.Assert(t, strings.Contains(start, "Connection: close"))
	assert.Assert(t, strings.Contains(start, ".register_failed"))
	assert.Assert(t, strings.Contains(stop, "node binary not found"))
	assert.Assert(t, strings.Contains(stop, "kill -0"))
	assert.Assert(t, strings.Contains(stop, "keeping ${STATE_DIR}"))
}
