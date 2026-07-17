/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package metrics defines shared apiserver business metrics (auth, authz, audit,
// long-lived streams, external dependency latency). They are registered on the
// controller-runtime registry so they are exposed on the apiserver's existing
// /metrics endpoint and collected via pull.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// AuthFailuresTotal counts authentication failures by reason.
	AuthFailuresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_apiserver_auth_failures_total",
		Help: "Total apiserver authentication failures, by reason.",
	}, []string{"reason"})

	// AuthzDeniedTotal counts authorization (RBAC) denials by resource and verb.
	AuthzDeniedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_apiserver_authz_denied_total",
		Help: "Total apiserver authorization denials, by resource and verb.",
	}, []string{"resource", "verb"})

	// AuditLogsDroppedTotal counts audit logs dropped because the buffer was full.
	AuditLogsDroppedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "safe_apiserver_audit_logs_dropped_total",
		Help: "Total audit logs dropped because the audit buffer was full.",
	})

	// StreamConnectionsActive tracks currently-open long-lived (SSE/WS) connections.
	StreamConnectionsActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_apiserver_stream_connections_active",
		Help: "Number of currently-open long-lived (SSE/WebSocket) connections, by endpoint.",
	}, []string{"endpoint"})

	// DependencyDuration measures latency of calls to external dependencies.
	DependencyDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "safe_apiserver_dependency_request_duration_seconds",
		Help:    "Latency of apiserver calls to external dependencies, by dependency and result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"dependency", "result"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		AuthFailuresTotal,
		AuthzDeniedTotal,
		AuditLogsDroppedTotal,
		StreamConnectionsActive,
		DependencyDuration,
	)
}

// ObserveDependency records the duration of an external dependency call. Use it
// as: defer metrics.ObserveDependency("opensearch", time.Now(), &err).
func ObserveDependency(dependency string, start time.Time, err *error) {
	result := "success"
	if err != nil && *err != nil {
		result = "error"
	}
	DependencyDuration.WithLabelValues(dependency, result).Observe(time.Since(start).Seconds())
}
