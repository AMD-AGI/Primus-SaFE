//go:build windows

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestExecuteCommandMissingShell reports failure when bash is unavailable on Windows.
func TestExecuteCommandMissingShell(t *testing.T) {
	statusCode, output := ExecuteCommand("echo hi", time.Second)
	assert.Assert(t, statusCode != types.StatusOk)
	assert.Assert(t, output != "")
}

// TestExecuteScriptMissingShell reports failure when the script runner is unavailable.
func TestExecuteScriptMissingShell(t *testing.T) {
	statusCode, output := ExecuteScript([]string{"missing.sh"}, time.Second)
	assert.Assert(t, statusCode != types.StatusOk)
	assert.Assert(t, output != "")
}
