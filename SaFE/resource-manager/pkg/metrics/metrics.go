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
	)
}
