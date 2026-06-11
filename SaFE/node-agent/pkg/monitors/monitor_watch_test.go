/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"path/filepath"
	"testing"
	"time"
)

// TestMonitorManagerUpdateConfigRetries backs off when the config path cannot be watched.
func TestMonitorManagerUpdateConfigRetries(t *testing.T) {
	manager := newMonitorManager(t)
	manager.configPath = filepath.Join(t.TempDir(), "missing-subdir")

	done := make(chan struct{})
	go func() {
		manager.updateConfig()
		close(done)
	}()

	time.Sleep(1200 * time.Millisecond)
	manager.tomb.Stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("updateConfig did not stop")
	}
}
