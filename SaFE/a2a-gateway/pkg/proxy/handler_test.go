/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package proxy

import (
	"testing"
	"time"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler(nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.httpClient == nil {
		t.Fatal("expected non-nil http client")
	}
	if h.httpClient.Timeout != 60*time.Second {
		t.Errorf("expected 60s timeout, got %v", h.httpClient.Timeout)
	}
}
