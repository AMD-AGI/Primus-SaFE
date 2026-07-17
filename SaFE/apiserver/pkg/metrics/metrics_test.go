/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package metrics

import (
	"errors"
	"testing"
	"time"

	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestApiserverMetricsRegistered(t *testing.T) {
	AuthFailuresTotal.WithLabelValues("missing").Inc()
	AuthzDeniedTotal.WithLabelValues("workload", "delete").Inc()
	AuditLogsDroppedTotal.Inc()
	StreamConnectionsActive.WithLabelValues("playground_chat").Inc()
	StreamConnectionsActive.WithLabelValues("playground_chat").Dec()

	err := errors.New("boom")
	ObserveDependency("opensearch", time.Now().Add(-time.Millisecond), &err)
	var noErr error
	ObserveDependency("opensearch", time.Now().Add(-time.Millisecond), &noErr)
	ObserveDependency("litellm", time.Now(), nil)

	families, gErr := ctrlmetrics.Registry.Gather()
	if gErr != nil {
		t.Fatalf("gather: %v", gErr)
	}
	names := map[string]bool{
		"safe_apiserver_auth_failures_total":                 false,
		"safe_apiserver_authz_denied_total":                  false,
		"safe_apiserver_audit_logs_dropped_total":            false,
		"safe_apiserver_stream_connections_active":           false,
		"safe_apiserver_dependency_request_duration_seconds": false,
	}
	for _, mf := range families {
		if _, ok := names[mf.GetName()]; ok {
			names[mf.GetName()] = true
		}
	}
	for name, seen := range names {
		if !seen {
			t.Errorf("metric %s not registered/exposed", name)
		}
	}
}
