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
	ClusterProvisionTotal.WithLabelValues("created").Inc()
	ClusterProvisionTotal.WithLabelValues("failed").Inc()
	ClusterProvisionDuration.WithLabelValues("created").Observe(120)
	ClusterDeprovisionTotal.WithLabelValues("deleted").Inc()
	ClusterReconcileErrorsTotal.WithLabelValues("default_addon").Inc()
	NodeManageTotal.WithLabelValues("managed").Inc()
	NodeManageTotal.WithLabelValues("failed").Inc()
	NodeUnmanageTotal.WithLabelValues("unmanaged").Inc()
	NodeMachineProbeTotal.WithLabelValues("ready").Inc()
	NodeMachineProbeTotal.WithLabelValues("ssh_failed").Inc()
	NodeBootstrapCommandTotal.WithLabelValues("authorize", "succeeded").Inc()
	NodeBootstrapCommandTotal.WithLabelValues("harbor_ca", "failed").Inc()
	ExporterSyncTotal.WithLabelValues("workload", "success").Inc()
	ExporterSyncDuration.WithLabelValues("workload").Observe(0.05)
	ExporterQueueDepth.WithLabelValues("fault").Set(2)
	ExporterTTLDroppedTotal.WithLabelValues("model").Inc()
	FaultCreatedTotal.WithLabelValues("gpu_xgmi").Inc()
	FaultTaintTotal.WithLabelValues("add", "success").Inc()
	FaultRetryExhaustedTotal.Inc()
	OpsJobPhaseTotal.WithLabelValues("reboot", "Failed", "timeout").Inc()
	OpsJobDuration.WithLabelValues("reboot").Observe(30)
	OpsJobTimeoutTotal.WithLabelValues("reboot").Inc()
	WorkspacePhaseTotal.WithLabelValues("Running").Inc()
	WorkspaceNodeBindingTotal.WithLabelValues("bind", "success").Inc()

	names := map[string]bool{
		"safe_cluster_provision_total":            false,
		"safe_cluster_provision_duration_seconds": false,
		"safe_cluster_deprovision_total":          false,
		"safe_cluster_reconcile_errors_total":     false,
		"safe_node_manage_total":                  false,
		"safe_node_unmanage_total":                false,
		"safe_node_machine_probe_total":           false,
		"safe_node_bootstrap_command_total":       false,
		"safe_exporter_sync_total":                false,
		"safe_exporter_sync_duration_seconds":     false,
		"safe_exporter_queue_depth":               false,
		"safe_exporter_ttl_dropped_total":         false,
		"safe_fault_created_total":                false,
		"safe_fault_taint_total":                  false,
		"safe_fault_retry_exhausted_total":        false,
		"safe_opsjob_phase_total":                 false,
		"safe_opsjob_duration_seconds":            false,
		"safe_opsjob_timeout_total":               false,
		"safe_workspace_phase_total":              false,
		"safe_workspace_node_binding_total":       false,
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
