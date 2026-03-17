// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package dag

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Informer: workloads discovered needing intent analysis
	intentWorkloadsDiscovered = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "intent_workloads_discovered_total",
			Help: "Total number of workloads discovered needing intent analysis",
		},
	)

	// Informer: pipeline tasks created
	intentPipelineTasksCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "intent_pipeline_tasks_created_total",
			Help: "Total number of analysis_pipeline tasks created by the informer",
		},
	)

	// Pipeline: WorkloadJSON dispatched to intent-service
	intentWorkloadsDispatched = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "intent_workloads_dispatched_total",
			Help: "Total number of workloads dispatched to Python intent-service",
		},
	)

	// Pipeline: state transitions
	intentPipelineTransitions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "intent_pipeline_state_transitions_total",
			Help: "Pipeline state machine transitions",
		},
		[]string{"from_state", "to_state"},
	)

	// Pipeline: evidence collection duration
	intentEvidenceCollectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "intent_evidence_collection_duration_seconds",
			Help:    "Time spent gathering evidence for WorkloadJSON assembly",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
	)

	// Image analysis: inline OCI analysis
	intentImageAnalysisDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "intent_image_analysis_duration_seconds",
			Help:    "Time spent on inline OCI image layer analysis",
			Buckets: []float64{1, 5, 10, 30, 60, 120},
		},
	)

	intentImageAnalysisTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "intent_image_analysis_total",
			Help: "Total inline image analyses performed",
		},
		[]string{"status"},
	)

	// DAG scheduler: active tasks gauge
	intentActiveDAGTasks = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "intent_active_dag_tasks",
			Help: "Number of currently active DAG master tasks in memory",
		},
	)

	// Informer scan interval
	intentInformerScansTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "intent_informer_scans_total",
			Help: "Total number of informer scan cycles executed",
		},
	)
)
