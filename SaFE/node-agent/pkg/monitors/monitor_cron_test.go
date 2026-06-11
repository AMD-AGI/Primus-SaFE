//go:build !windows

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"os"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
)

// TestStartCronJobInvalidSchedule exits when cron expression parsing fails.
func TestStartCronJobInvalidSchedule(t *testing.T) {
	path := "./unit-bad-cron.sh"
	assert.NilError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0777))
	defer os.Remove(path)

	q := unitTestQueue(t)
	n := unitTestNode(t)
	conf := newMonitorConfig("safe.bad-cron", "unit-bad-cron.sh")
	conf.Cronjob = "invalid cron"
	m := NewMonitor(conf, &q, n, ".")
	assert.Assert(t, m != nil)
	m.tomb = channel.NewTomb()
	done := make(chan struct{})
	go func() {
		m.startCronJob()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("startCronJob did not return on invalid schedule")
	}
}
