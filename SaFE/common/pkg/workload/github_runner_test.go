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

func TestGithubRunnerScriptsKeepRegistrationGuards(t *testing.T) {
	start := GithubRunnerStartScript()
	stop := GithubRunnerStopScript()
	assert.Assert(t, strings.Contains(start, ".register_failed"))
	assert.Assert(t, strings.Contains(start, "FAILED_SECRET_ID"))
	assert.Assert(t, strings.Contains(stop, "node binary not found"))
	assert.Assert(t, strings.Contains(stop, "JSON.parse"))
	assert.Assert(t, strings.Contains(stop, "keeping ${STATE_DIR}"))
	assert.Assert(t, !strings.Contains(start, "github-proxy-relay"))
	assert.Assert(t, !strings.Contains(stop, "setup_github_proxy"))
}
