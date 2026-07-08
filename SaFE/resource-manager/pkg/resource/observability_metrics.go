/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/observability"
)

// SetupObservabilityMetrics wires the SaFE-native metrics path: it discovers
// each ready cluster's VictoriaMetrics / Prometheus endpoint and provisions a
// Grafana Prometheus datasource that points directly at it (no primus-robust
// vm-proxy hop). It is a no-op unless observability.metrics.enable is true, so
// existing robust-backed deployments are unaffected until they opt in.
//
// It returns the populated MetricsRegistry so other components (stats syncers,
// API handlers) can issue PromQL against the same per-cluster endpoints.
func SetupObservabilityMetrics(ctx context.Context, mgr manager.Manager) (*observability.MetricsRegistry, error) {
	if !commonconfig.IsObservabilityMetricsEnable() {
		klog.Info("[observability-metrics] disabled, skipping")
		return nil, nil
	}

	registry := observability.NewMetricsRegistry(observability.MetricsClientConfig{
		InsecureSkipVerify: commonconfig.GetObservabilityMetricsInsecureSkipVerify(),
	})

	discovery := observability.NewMetricsDiscovery(mgr.GetClient(), registry, observability.MetricsDiscoveryConfig{
		Interval:        30 * time.Second,
		AnnotationKey:   commonconfig.GetObservabilityMetricsEndpointAnnotation(),
		DefaultEndpoint: commonconfig.GetObservabilityMetricsEndpoint(),
	})
	discovery.Start(ctx)

	if gs, err := NewGrafanaDatasourceSyncer(mgr.GetConfig(), "primus-safe"); err != nil {
		klog.Warningf("[observability-metrics] grafana datasource syncer init failed (non-blocking): %v", err)
	} else {
		go runMetricsDatasourceProvisioner(ctx, registry, gs, 60*time.Second)
	}

	klog.Info("[observability-metrics] enabled: metrics discovery + grafana datasource provisioner started")
	return registry, nil
}

// runMetricsDatasourceProvisioner periodically reconciles Grafana Prometheus
// datasources against the discovered per-cluster metrics endpoints. Applying a
// datasource is idempotent, so re-running on an interval simply repairs drift
// and picks up newly discovered clusters.
func runMetricsDatasourceProvisioner(ctx context.Context, registry *observability.MetricsRegistry, gs *GrafanaDatasourceSyncer, interval time.Duration) {
	reconcile := func() {
		for _, name := range registry.ClusterNames() {
			client := registry.ForCluster(name)
			if client == nil {
				continue
			}
			gs.SyncClusterMetricsDatasource(ctx, name, client.BaseURL())
		}
	}

	reconcile()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		}
	}
}
