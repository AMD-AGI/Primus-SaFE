/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"runtime"
	"testing"
	"time"

	"gotest.tools/assert"
)

// failingNSENTERPrefix forces the shell pipeline to exit non-zero before host tools run.
// Empty NSENTER runs commands on the local host; CI runners often have timedatectl/uptime,
// so we cannot assert failure from an empty prefix.
const failingNSENTERPrefix = `false && `

// TestGetLocationFailsWhenHostCommandFails returns an error when the wrapped command fails.
func TestGetLocationFailsWhenHostCommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = failingNSENTERPrefix
	defer func() { NSENTER = saved }()
	_, err := getLocation()
	assert.Assert(t, err != nil)
}

// TestGetUptimeFailsWhenHostCommandFails returns an error when uptime cannot be read.
func TestGetUptimeFailsWhenHostCommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = failingNSENTERPrefix
	defer func() { NSENTER = saved }()
	_, err := getUptime(time.UTC)
	assert.Assert(t, err != nil)
}

// TestUpdateStartTimeFailsWhenHostCommandFails propagates failures from getLocation/getUptime.
func TestUpdateStartTimeFailsWhenHostCommandFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires unix shell commands")
	}
	saved := NSENTER
	NSENTER = failingNSENTERPrefix
	defer func() { NSENTER = saved }()
	n, _ := newNode(t)
	err := n.updateStartTime()
	assert.Assert(t, err != nil)
}
