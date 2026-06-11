/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMetrics ensures the collectors are initialized and writable.
func TestMetrics(t *testing.T) {
	RequestsTotal.WithLabelValues("caller", "target", "skill", "success").Inc()
	if got := testutil.ToFloat64(RequestsTotal.WithLabelValues("caller", "target", "skill", "success")); got != 1 {
		t.Errorf("RequestsTotal = %v, want 1", got)
	}

	RequestDuration.WithLabelValues("caller", "target", "skill").Observe(0.5)

	ServicesRegistered.WithLabelValues("healthy").Set(3)
	if got := testutil.ToFloat64(ServicesRegistered.WithLabelValues("healthy")); got != 3 {
		t.Errorf("ServicesRegistered = %v, want 3", got)
	}
}
