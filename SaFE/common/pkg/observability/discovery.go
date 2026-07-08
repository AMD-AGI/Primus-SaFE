/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package observability

import (
	"context"
	"strings"
	"sync"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MetricsDiscovery watches Cluster CRs and keeps a MetricsRegistry populated
// with each ready cluster's Prometheus-compatible metrics endpoint.
//
// Endpoint resolution priority for a cluster:
//  1. Cluster CR annotation (default key primus-safe.amd.com/metrics-endpoint).
//     Use this for cross-cluster setups where vmselect is exposed via
//     NodePort / LoadBalancer / Ingress on the data cluster.
//  2. A shared default endpoint (e.g. the in-cluster vmselect Service DNS)
//     applied to every ready cluster. Works when the management plane and the
//     metrics backend live in the same K8s cluster, or when a single central
//     VictoriaMetrics stores all clusters' data.
type MetricsDiscovery struct {
	k8sClient       client.Client
	registry        *MetricsRegistry
	interval        time.Duration
	annotationKey   string
	defaultEndpoint string
	stopOnce        sync.Once
	stopCh          chan struct{}
}

// MetricsDiscoveryConfig configures endpoint resolution.
type MetricsDiscoveryConfig struct {
	// Interval between reconcile passes.
	Interval time.Duration
	// AnnotationKey is the Cluster CR annotation carrying a per-cluster
	// endpoint override. Empty falls back to the package default.
	AnnotationKey string
	// DefaultEndpoint is applied to ready clusters without an annotation.
	// Empty means such clusters are skipped.
	DefaultEndpoint string
}

// NewMetricsDiscovery creates a discovery loop bound to a registry.
func NewMetricsDiscovery(k8sClient client.Client, registry *MetricsRegistry, cfg MetricsDiscoveryConfig) *MetricsDiscovery {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	annotationKey := cfg.AnnotationKey
	if annotationKey == "" {
		annotationKey = "primus-safe.amd.com/metrics-endpoint"
	}
	return &MetricsDiscovery{
		k8sClient:       k8sClient,
		registry:        registry,
		interval:        interval,
		annotationKey:   annotationKey,
		defaultEndpoint: cfg.DefaultEndpoint,
		stopCh:          make(chan struct{}),
	}
}

// Start runs the reconcile loop in a background goroutine.
func (d *MetricsDiscovery) Start(ctx context.Context) {
	go func() {
		d.syncOnce(ctx)
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.stopCh:
				return
			case <-ticker.C:
				d.syncOnce(ctx)
			}
		}
	}()
	klog.Infof("[observability] metrics discovery started (interval=%s)", d.interval)
}

// Stop halts the reconcile loop.
func (d *MetricsDiscovery) Stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
}

func (d *MetricsDiscovery) syncOnce(ctx context.Context) {
	clusterList := &v1.ClusterList{}
	if err := d.k8sClient.List(ctx, clusterList); err != nil {
		klog.Warningf("[observability] list clusters failed: %v", err)
		return
	}

	seen := make(map[string]bool, len(clusterList.Items))
	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		name := cluster.Name
		if !cluster.IsReady() {
			continue
		}
		endpoint := d.resolveEndpoint(cluster)
		if endpoint == "" {
			continue
		}
		seen[name] = true

		existing := d.registry.ForCluster(name)
		if existing != nil && existing.BaseURL() == strings.TrimRight(endpoint, "/") {
			continue
		}
		d.registry.RegisterCluster(name, endpoint)
		klog.Infof("[observability] discovered metrics endpoint %s -> %s", name, endpoint)
	}

	for _, name := range d.registry.ClusterNames() {
		if !seen[name] {
			d.registry.RemoveCluster(name)
			klog.Infof("[observability] removed stale metrics endpoint %s", name)
		}
	}
}

func (d *MetricsDiscovery) resolveEndpoint(cluster *v1.Cluster) string {
	if ep, ok := cluster.Annotations[d.annotationKey]; ok && ep != "" {
		return ep
	}
	return d.defaultEndpoint
}
