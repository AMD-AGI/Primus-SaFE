/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package health defines SaFE control-plane self-health metrics
// (component readiness, subsystem/cluster reachability). The metrics are
// registered on the controller-runtime metrics registry so they are exposed on
// the component's existing /metrics endpoint and collected by the monitoring
// infrastructure (vmagent/Prometheus) via pull — the component never pushes.
package health

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// SubsystemDatabase is the subsystem label value for the DB health gauge.
	SubsystemDatabase = "database"
)

var (
	// ComponentUp is 1 when a control-plane component is fully healthy
	// (desired > 0 and ready >= desired), else 0.
	ComponentUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_component_up",
		Help: "1 if the SaFE control-plane component is fully healthy (all desired replicas ready), else 0.",
	}, []string{"component", "kind"})

	// ComponentReplicasDesired reports the desired replica/scheduling count.
	ComponentReplicasDesired = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_component_replicas_desired",
		Help: "Desired replicas (Deployment) or desired scheduled pods (DaemonSet) of a SaFE component.",
	}, []string{"component", "kind"})

	// ComponentReplicasReady reports the ready replica/scheduling count.
	ComponentReplicasReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_component_replicas_ready",
		Help: "Ready replicas (Deployment) or ready pods (DaemonSet) of a SaFE component.",
	}, []string{"component", "kind"})

	// SubsystemUp is 1 when a shared subsystem (e.g. database) is reachable.
	SubsystemUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_subsystem_up",
		Help: "1 if the SaFE subsystem dependency is reachable, else 0.",
	}, []string{"subsystem"})

	// ClusterReady is 1 when a managed data-plane Cluster CR is in Ready phase.
	ClusterReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_cluster_ready",
		Help: "1 if the managed data-plane cluster CR is in Ready phase, else 0.",
	}, []string{"target_cluster"})

	// ClusterUp is 1 when the managed data-plane cluster API is actually
	// reachable using SaFE's stored credentials.
	ClusterUp = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_cluster_up",
		Help: "1 if the managed data-plane cluster API server is reachable from SaFE, else 0.",
	}, []string{"target_cluster"})

	// BuildInfo is a constant 1 gauge carrying the reporting component name.
	BuildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "safe_build_info",
		Help: "Constant 1 series identifying the SaFE component reporting self-health.",
	}, []string{"component"})
)

func init() {
	// Register on the controller-runtime registry so these series are exposed on
	// the component's existing /metrics endpoint (no extra port, no push).
	ctrlmetrics.Registry.MustRegister(
		ComponentUp,
		ComponentReplicasDesired,
		ComponentReplicasReady,
		SubsystemUp,
		ClusterReady,
		ClusterUp,
		BuildInfo,
	)
}

// ResetScanned clears the vec metrics that are rebuilt from a full scan on every
// cycle, so series for deleted components/clusters do not linger as stale values.
func ResetScanned() {
	ComponentUp.Reset()
	ComponentReplicasDesired.Reset()
	ComponentReplicasReady.Reset()
	SubsystemUp.Reset()
	ClusterReady.Reset()
	ClusterUp.Reset()
}

// SetBool sets a gauge to 1 for true and 0 for false.
func SetBool(g prometheus.Gauge, ok bool) {
	if ok {
		g.Set(1)
	} else {
		g.Set(0)
	}
}
