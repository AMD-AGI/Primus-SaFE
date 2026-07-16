/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package metrics defines job-manager business metrics (scheduler, dispatcher,
// syncer). They are registered on the controller-runtime registry so they are
// exposed on the existing /metrics endpoint and collected via pull.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Unschedulable reason keys (bounded cardinality).
const (
	ReasonInsufficient = "insufficient"
	ReasonDependency   = "dependency"
	ReasonCronjob      = "cronjob"
	ReasonSource       = "source"
)

var (
	// ScheduleCycleDuration measures one full scheduleWorkloads cycle.
	ScheduleCycleDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "safe_scheduler_schedule_cycle_duration_seconds",
		Help:    "Duration of a scheduler scheduling cycle in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// SchedulerUnschedulableTotal counts unschedulable decisions by reason.
	SchedulerUnschedulableTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_scheduler_unschedulable_total",
		Help: "Total workloads that could not be scheduled, by reason.",
	}, []string{"reason"})

	// SchedulerScheduledTotal counts workloads marked as scheduled.
	SchedulerScheduledTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_scheduler_scheduled_total",
		Help: "Total workloads marked as scheduled, by workspace.",
	}, []string{"workspace"})

	// SchedulerQueueDepth reports the number of queued (unscheduled) workloads.
	SchedulerQueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_scheduler_queue_depth",
		Help: "Number of workloads waiting in the scheduling queue, by workspace.",
	}, []string{"workspace"})

	// SchedulerPreemptionsTotal counts preemptions performed to free resources.
	SchedulerPreemptionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_scheduler_preemptions_total",
		Help: "Total workloads preempted to free resources, by workspace.",
	}, []string{"workspace"})

	// DispatchTotal counts dispatch reconcile outcomes.
	DispatchTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_dispatcher_dispatch_total",
		Help: "Total workload dispatch reconcile results, by result (success/error/unrecoverable).",
	}, []string{"result"})

	// DispatchDuration measures dispatching a workload to the data plane.
	DispatchDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "safe_dispatch_duration_seconds",
		Help:    "Duration of dispatching a workload to the data plane in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// WorkloadPhaseTotal counts workload phase assignments observed by the syncer.
	WorkloadPhaseTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_workload_phase_total",
		Help: "Total workload phase assignments observed by the syncer, by phase.",
	}, []string{"phase"})

	// WorkloadRescheduleTotal counts syncer-triggered reschedules.
	WorkloadRescheduleTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "safe_workload_reschedule_total",
		Help: "Total workload reschedules triggered by the syncer.",
	})
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		ScheduleCycleDuration,
		SchedulerUnschedulableTotal,
		SchedulerScheduledTotal,
		SchedulerQueueDepth,
		SchedulerPreemptionsTotal,
		DispatchTotal,
		DispatchDuration,
		WorkloadPhaseTotal,
		WorkloadRescheduleTotal,
	)
}
