/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package metrics

import (
	"testing"

	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestMetricsRegisteredAndUsable(t *testing.T) {
	// Exercise every metric so the WithLabelValues paths and registration are covered.
	ScheduleCycleDuration.Observe(0.1)
	SchedulerUnschedulableTotal.WithLabelValues(ReasonInsufficient).Inc()
	SchedulerUnschedulableTotal.WithLabelValues(ReasonDependency).Inc()
	SchedulerUnschedulableTotal.WithLabelValues(ReasonCronjob).Inc()
	SchedulerUnschedulableTotal.WithLabelValues(ReasonSource).Inc()
	SchedulerScheduledTotal.WithLabelValues("dev").Inc()
	SchedulerQueueDepth.WithLabelValues("dev").Set(3)
	SchedulerPreemptionsTotal.WithLabelValues("dev").Inc()
	DispatchTotal.WithLabelValues("success").Inc()
	DispatchTotal.WithLabelValues("error").Inc()
	DispatchDuration.Observe(0.2)
	WorkloadPhaseTotal.WithLabelValues("Running").Inc()
	WorkloadRescheduleTotal.Inc()

	names := map[string]bool{
		"safe_scheduler_schedule_cycle_duration_seconds": false,
		"safe_scheduler_unschedulable_total":             false,
		"safe_scheduler_scheduled_total":                 false,
		"safe_scheduler_queue_depth":                     false,
		"safe_scheduler_preemptions_total":               false,
		"safe_dispatcher_dispatch_total":                 false,
		"safe_dispatch_duration_seconds":                 false,
		"safe_workload_phase_total":                      false,
		"safe_workload_reschedule_total":                 false,
	}
	mfs, err := ctrlmetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if _, ok := names[mf.GetName()]; ok {
			names[mf.GetName()] = true
		}
	}
	for n, seen := range names {
		if !seen {
			t.Errorf("metric %s not registered on controller-runtime registry", n)
		}
	}
}
