//go:build !windows

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

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestMonitorRun enqueues a message when the script exits successfully.
func TestMonitorRun(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ok.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/bash\necho ok\nexit 0"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := &Monitor{
		config:     newMonitorConfig("safe.run", "ok.sh"),
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()

	msg, shutdown := q.Get()
	assert.Equal(t, shutdown, false)
	assert.Equal(t, msg.Id, "safe.run")
	assert.Equal(t, msg.StatusCode, types.StatusOk)
	q.Done(msg)
}

// TestMonitorRunConsecutiveError waits until consecutive failures reach the threshold.
func TestMonitorRunConsecutiveError(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/bash\necho err\nexit 1"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	conf := newMonitorConfig("safe.fail", "fail.sh")
	conf.ConsecutiveCount = 2
	m := &Monitor{
		config:     conf,
		queue:      &q,
		scriptPath: script,
		node:       n,
	}
	m.Run()
	assert.Equal(t, q.Len(), 0)
	m.Run()
	msg, shutdown := q.Get()
	assert.Equal(t, shutdown, false)
	assert.Equal(t, msg.StatusCode, types.StatusError)
	q.Done(msg)
}

// TestGenerateNodeInfo builds node metadata from labels and allocatable resources.
func TestGenerateNodeInfo(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "node.sh")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/bash\nexit 0"), 0777))

	q := unitTestQueue(t)
	n := unitTestNode(t)
	m := NewMonitor(newMonitorConfig("safe.info", "node.sh"), &q, n, dir)
	assert.Assert(t, m != nil)
	info := m.generateNodeInfo()
	assert.Assert(t, info != nil)
	assert.Equal(t, info.NodeName, "unit-node")
	assert.Equal(t, info.ObservedGpuCount, 2)
}

// TestGenerateNodeInfoNilNode returns nil when the node reference is missing.
func TestGenerateNodeInfoNilNode(t *testing.T) {
	m := &Monitor{node: nil}
	assert.Assert(t, m.generateNodeInfo() == nil)
}
