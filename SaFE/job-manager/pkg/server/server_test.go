/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"testing"
)

// TestServerStartNotInited verifies Start is a no-op when the server is not initialized.
func TestServerStartNotInited(t *testing.T) {
	s := &Server{isInited: false}
	// Should log and return without panicking (jobManager is nil).
	s.Start()
}

// TestServerStop verifies Stop flushes logs without error.
func TestServerStop(t *testing.T) {
	s := &Server{}
	s.Stop()
}
