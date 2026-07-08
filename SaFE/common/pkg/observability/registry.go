/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package observability

import (
	"sync"

	"k8s.io/klog/v2"
)

// MetricsRegistry holds one MetricsClient per data cluster. It mirrors the
// shape of the legacy robustclient.Client (RegisterCluster / RemoveCluster /
// ForCluster / ClusterNames) so callers that previously fanned out over
// robust-analyzer endpoints can be repointed at each cluster's own vmselect
// with a minimal diff.
type MetricsRegistry struct {
	mu       sync.RWMutex
	clusters map[string]*MetricsClient
	defaults MetricsClientConfig
}

// NewMetricsRegistry builds an empty registry. defaults carries per-client
// settings (timeout, TLS) applied to every cluster endpoint registered later.
func NewMetricsRegistry(defaults MetricsClientConfig) *MetricsRegistry {
	return &MetricsRegistry{
		clusters: make(map[string]*MetricsClient),
		defaults: defaults,
	}
}

// RegisterCluster adds or updates the metrics endpoint for a cluster.
func (r *MetricsRegistry) RegisterCluster(clusterName, endpoint string) {
	cfg := r.defaults
	cfg.BaseURL = endpoint

	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusters[clusterName] = NewMetricsClient(cfg)
	klog.V(2).Infof("[observability] registered metrics endpoint %s -> %s", clusterName, endpoint)
}

// RemoveCluster drops a cluster's metrics endpoint.
func (r *MetricsRegistry) RemoveCluster(clusterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clusters, clusterName)
}

// ForCluster returns the metrics client for a cluster, or nil if unregistered.
func (r *MetricsRegistry) ForCluster(clusterName string) *MetricsClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clusters[clusterName]
}

// ClusterNames lists all registered cluster names.
func (r *MetricsRegistry) ClusterNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clusters))
	for name := range r.clusters {
		names = append(names, name)
	}
	return names
}
