/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

// TestMonitorRunSkipsUnknownStatus ignores non-reportable script exit codes.
func TestMonitorRunSkipsUnknownStatus(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "unknown.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := &Monitor{
		config:     newMonitorConfig("safe.unknown", "unknown.sh"),
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()
	assert.Equal(t, q.Len(), 0)
}

// TestMonitorRunProcessesArguments expands reserved words before executing the script.
func TestMonitorRunProcessesArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "args.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	conf := newMonitorConfig("safe.args", "args.sh")
	conf.Arguments = []string{"$Node", "plain"}
	m := &Monitor{
		config:     conf,
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()
	assert.Equal(t, q.Len(), 0)
}
