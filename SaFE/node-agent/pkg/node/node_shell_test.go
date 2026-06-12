/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"testing"
	"time"

	"gotest.tools/assert"
)

// TestGetLocationFailsWithoutShell reports an error when host commands are unavailable.
func TestGetLocationFailsWithoutShell(t *testing.T) {
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	_, err := getLocation()
	assert.Assert(t, err != nil)
}

// TestGetUptimeFailsWithoutShell reports an error when uptime command is unavailable.
func TestGetUptimeFailsWithoutShell(t *testing.T) {
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	_, err := getUptime(time.UTC)
	assert.Assert(t, err != nil)
}

// TestUpdateStartTimeFailsWithoutShell propagates shell command failures.
func TestUpdateStartTimeFailsWithoutShell(t *testing.T) {
	saved := NSENTER
	NSENTER = ""
	defer func() { NSENTER = saved }()
	n, _ := newNode(t)
	err := n.updateStartTime()
	assert.Assert(t, err != nil)
}
