/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "a2a_gateway_requests_total",
			Help: "Total number of A2A gateway requests",
		},
		[]string{"caller", "target", "skill", "status"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "a2a_gateway_request_duration_seconds",
			Help:    "Duration of A2A gateway requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"caller", "target", "skill"},
	)

	ServicesRegistered = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "a2a_services_registered_total",
			Help: "Number of registered A2A services by health status",
		},
		[]string{"health"},
	)
)
