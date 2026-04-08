// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

import (
	"fmt"
	"os"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Registry maps cluster names to Robust API clients.
// In control-plane mode each cluster has its own client pointing to the
// Robust instance in that cluster. In data-plane mode there is only one client.
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

// GetRegistry returns the global Robust client registry.
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = &Registry{
			clients: make(map[string]*Client),
		}
		globalRegistry.initFromEnv()
	})
	return globalRegistry
}

// initFromEnv creates a client from the ROBUST_API_URL environment variable.
// This registers a single client for the current cluster (CLUSTER_NAME or "default").
//
// In multi-cluster deployments the remaining clusters are discovered dynamically
// from the control-plane database (cluster_config.robust_endpoint) by
// ClusterManager.syncRobustClientsFromDB during periodic sync.
func (r *Registry) initFromEnv() {
	if u := os.Getenv("ROBUST_API_URL"); u != "" {
		clusterName := os.Getenv("CLUSTER_NAME")
		if clusterName == "" {
			clusterName = "default"
		}
		r.clients[clusterName] = NewClient(u, clusterName)
		log.Infof("[robust] Registered client for cluster %q from ROBUST_API_URL: %s", clusterName, u)
	}
}

// Register adds or replaces a client for the given cluster.
func (r *Registry) Register(clusterName, baseURL string, opts ...Option) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[clusterName] = NewClient(baseURL, clusterName, opts...)
	log.Infof("[robust] Registered client for cluster %q -> %s", clusterName, baseURL)
}

// GetClient returns the Robust client for the given cluster.
func (r *Registry) GetClient(clusterName string) (*Client, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[clusterName]
	if !ok {
		return nil, fmt.Errorf("robust: no client registered for cluster %q", clusterName)
	}
	return c, nil
}

// GetClientOrDefault returns the client for the given cluster, falling back to
// CLUSTER_NAME env var, then "default".
func (r *Registry) GetClientOrDefault(clusterName string) (*Client, error) {
	if clusterName != "" {
		return r.GetClient(clusterName)
	}
	if name := os.Getenv("CLUSTER_NAME"); name != "" {
		return r.GetClient(name)
	}
	return r.GetClient("default")
}

// ListClusters returns all registered cluster names.
func (r *Registry) ListClusters() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clients))
	for k := range r.clients {
		names = append(names, k)
	}
	return names
}
