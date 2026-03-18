// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// initMultiCluster initializes multi-cluster clients for control plane
// This reads cluster configurations from the control plane database
// and creates clients for each remote cluster
func (cm *ClusterManager) initMultiCluster(ctx context.Context) error {
	log.Info("Initializing multi-cluster clients...")

	// Initial load of K8S clients for all clusters
	// For control plane, always load multi-cluster clients regardless of config
	if cm.componentType.IsControlPlane() || cm.loadK8SClient {
		if err := loadMultiClusterK8SClientSet(ctx); err != nil {
			log.Warnf("Failed to load multi-cluster K8S clients: %v", err)
			// Don't return error as multi-cluster config may not be ready yet
		}
	}

	// Initial load of Storage clients for all clusters
	// For control plane, always load multi-cluster clients regardless of config
	if cm.componentType.IsControlPlane() || cm.loadStorageClient {
		if err := loadMultiClusterStorageClients(ctx); err != nil {
			log.Warnf("Failed to load multi-cluster storage clients: %v", err)
			// Don't return error as multi-cluster config may not be ready yet
		}
	}

	// Initial load of all clusters into the cluster map
	if err := cm.loadAllClusters(ctx); err != nil {
		log.Warnf("Failed to load all clusters: %v", err)
	}

	// Start unified periodic sync for K8S, Storage, and cluster map
	// This ensures correct ordering: K8S -> Storage -> cluster map
	go cm.startPeriodicSync(ctx)

	log.Info("Multi-cluster initialization initiated")
	return nil
}

// NOTE: startMultiClusterStorageSync is no longer used as storage sync
// is now handled by the unified startPeriodicSync in cluster_manager.go
