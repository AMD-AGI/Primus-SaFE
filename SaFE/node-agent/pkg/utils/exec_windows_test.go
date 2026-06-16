//go:build windows

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
)

// TestExec verifies Exec wires context cancellation on Windows.
func TestExec(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := Exec(ctx, "cmd", "/C", "exit", "0")
	assert.Assert(t, cmd != nil)
	assert.Equal(t, cmd.WaitDelay, 5*time.Second)
	assert.NilError(t, cmd.Run())
}
