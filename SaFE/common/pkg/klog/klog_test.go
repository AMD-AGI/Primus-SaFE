/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package klog

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "test.log")
	// logFileSize != 0 branch is exercised here.
	assert.NoError(t, Init(logFile, 10))
}
