// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	intentWorkloadsDispatched = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "intent_workloads_dispatched_total",
			Help: "Total number of workloads dispatched to Python intent-service via WorkloadJSON",
		},
	)

	intentEvidenceGatherDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "intent_evidence_gather_duration_seconds",
			Help:    "Time spent gathering evidence in handleEvaluating",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
	)
)
