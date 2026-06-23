/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/config"
)

// TestNew verifies New returns an error when the database client cannot be
// initialized (no database is configured in the unit-test environment).
func TestNew(t *testing.T) {
	cfg := &config.Config{ServerPort: 8089, MetricsPort: 9090}
	srv, err := New(cfg)
	if err == nil {
		// A database is unexpectedly reachable; the server must still be valid.
		if srv == nil {
			t.Fatal("New returned nil server without error")
		}
		if srv.engine == nil {
			t.Error("expected non-nil gin engine")
		}
		return
	}
	if srv != nil {
		t.Errorf("expected nil server on error, got %v", srv)
	}
}
