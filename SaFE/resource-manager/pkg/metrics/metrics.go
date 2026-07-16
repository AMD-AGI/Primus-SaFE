/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package metrics defines resource-manager business metrics (cluster
// provisioning, node management). They are registered on the controller-runtime
// registry so they are exposed on the existing /metrics endpoint (pull).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ClusterProvisionTotal counts data-plane cluster create outcomes.
	ClusterProvisionTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_cluster_provision_total",
		Help: "Total cluster provision (create) outcomes, by result (created/failed).",
	}, []string{"result"})

	// ClusterProvisionDuration measures cluster create wall time (kubespray run).
	ClusterProvisionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "safe_cluster_provision_duration_seconds",
		Help:    "Cluster provision (create) duration in seconds, by result.",
		Buckets: []float64{60, 180, 300, 600, 1200, 1800, 3600, 7200},
	}, []string{"result"})

	// ClusterDeprovisionTotal counts data-plane cluster delete/reset outcomes.
	ClusterDeprovisionTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_cluster_deprovision_total",
		Help: "Total cluster deprovision (delete/reset) outcomes, by result (deleted/failed).",
	}, []string{"result"})

	// ClusterReconcileErrorsTotal counts which reconcile guarantee step failed.
	ClusterReconcileErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_cluster_reconcile_errors_total",
		Help: "Total cluster reconcile errors, by failing step.",
	}, []string{"step"})

	// NodeManageTotal counts node scale-up (manage) outcomes.
	NodeManageTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_node_manage_total",
		Help: "Total node manage (scale-up) outcomes, by result (managed/failed).",
	}, []string{"result"})

	// NodeUnmanageTotal counts node scale-down (unmanage) outcomes.
	NodeUnmanageTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_node_unmanage_total",
		Help: "Total node unmanage (scale-down) outcomes, by result (unmanaged/failed).",
	}, []string{"result"})

	// NodeMachineProbeTotal counts machine-level (SSH/hostname) probe outcomes.
	NodeMachineProbeTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_node_machine_probe_total",
		Help: "Total node machine probe outcomes, by result (ready/ssh_failed/hostname_failed).",
	}, []string{"result"})

	// NodeBootstrapCommandTotal counts bootstrap command outcomes (authorize, harbor cert).
	NodeBootstrapCommandTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_node_bootstrap_command_total",
		Help: "Total node bootstrap command outcomes, by command and result.",
	}, []string{"command", "result"})

	// --- exporter (CRD -> PostgreSQL) ---

	// ExporterSyncTotal counts DB sync outcomes per resource kind.
	ExporterSyncTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_exporter_sync_total",
		Help: "Total resource DB-export sync outcomes, by kind and result.",
	}, []string{"kind", "result"})

	// ExporterSyncDuration measures DB sync latency per resource kind.
	ExporterSyncDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "safe_exporter_sync_duration_seconds",
		Help:    "Resource DB-export sync duration in seconds, by kind.",
		Buckets: prometheus.DefBuckets,
	}, []string{"kind"})

	// ExporterQueueDepth reports the exporter work queue depth per kind.
	ExporterQueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_exporter_queue_depth",
		Help: "Exporter work-queue depth, by kind.",
	}, []string{"kind"})

	// ExporterTTLDroppedTotal counts records dropped after the export TTL expires.
	ExporterTTLDroppedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_exporter_ttl_dropped_total",
		Help: "Total records permanently dropped by the exporter after TTL, by kind.",
	}, []string{"kind"})

	// --- fault ---

	// FaultCreatedTotal counts fault CR creations by monitor id (fault type).
	FaultCreatedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_fault_created_total",
		Help: "Total faults created, by monitor id.",
	}, []string{"monitor_id"})

	// FaultTaintTotal counts node taint add/remove outcomes for faults.
	FaultTaintTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_fault_taint_total",
		Help: "Total fault node taint operations, by action and result.",
	}, []string{"action", "result"})

	// FaultRetryExhaustedTotal counts faults that exhausted their retry budget.
	FaultRetryExhaustedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "safe_fault_retry_exhausted_total",
		Help: "Total faults that exceeded the maximum retry count.",
	})

	// --- opsjob ---

	// OpsJobPhaseTotal counts opsjob terminal phases by type and reason.
	OpsJobPhaseTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_opsjob_phase_total",
		Help: "Total opsjob completions, by type, phase and reason.",
	}, []string{"type", "phase", "reason"})

	// OpsJobDuration measures opsjob run duration by type.
	OpsJobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "safe_opsjob_duration_seconds",
		Help:    "OpsJob run duration in seconds, by type.",
		Buckets: []float64{5, 15, 30, 60, 120, 300, 600, 1800, 3600},
	}, []string{"type"})

	// OpsJobTimeoutTotal counts opsjob timeouts by type.
	OpsJobTimeoutTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_opsjob_timeout_total",
		Help: "Total opsjob timeouts, by type.",
	}, []string{"type"})

	// --- workspace ---

	// WorkspacePhaseTotal counts workspace phase transitions.
	WorkspacePhaseTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_workspace_phase_total",
		Help: "Total workspace phase transitions, by phase.",
	}, []string{"phase"})

	// WorkspaceNodeBindingTotal counts node bind/unbind outcomes for workspaces.
	WorkspaceNodeBindingTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_workspace_node_binding_total",
		Help: "Total workspace node bind/unbind operations, by action and result.",
	}, []string{"action", "result"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		ClusterProvisionTotal,
		ClusterProvisionDuration,
		ClusterDeprovisionTotal,
		ClusterReconcileErrorsTotal,
		NodeManageTotal,
		NodeUnmanageTotal,
		NodeMachineProbeTotal,
		NodeBootstrapCommandTotal,
		ExporterSyncTotal,
		ExporterSyncDuration,
		ExporterQueueDepth,
		ExporterTTLDroppedTotal,
		FaultCreatedTotal,
		FaultTaintTotal,
		FaultRetryExhaustedTotal,
		OpsJobPhaseTotal,
		OpsJobDuration,
		OpsJobTimeoutTotal,
		WorkspacePhaseTotal,
		WorkspaceNodeBindingTotal,
	)
}
