// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// initMultiCluster initializes multi-cluster clients for control plane
// This reads cluster configurations from the control plane database
// and creates clients for each remote cluster
func (cm *ClusterManager) initMultiCluster(ctx context.Context) error {
	log.Info("Initializing multi-cluster clients...")

	// Load K8S clients for all clusters if K8S is enabled
	if cm.loadK8SClient {
		if err := loadMultiClusterK8SClientSet(ctx); err != nil {
			log.Warnf("Failed to load multi-cluster K8S clients: %v", err)
			// Don't return error as multi-cluster config may not be ready yet
		}
		// Start periodic sync for K8S clients
		go doLoadMultiClusterK8SClientSet(ctx)
	}

	// Load Storage clients for all clusters if Storage is enabled
	if cm.loadStorageClient {
		if err := loadMultiClusterStorageClients(ctx); err != nil {
			log.Warnf("Failed to load multi-cluster storage clients: %v", err)
			// Don't return error as multi-cluster config may not be ready yet
		}
		// Start periodic sync for storage clients
		go cm.startMultiClusterStorageSync(ctx)
	}

	// Load all clusters into the cluster map
	if err := cm.loadAllClusters(ctx); err != nil {
		log.Warnf("Failed to load all clusters: %v", err)
	}

	// Start periodic sync for cluster map
	go cm.startPeriodicSync(ctx)

	log.Info("Multi-cluster initialization initiated")
	return nil
}

// startMultiClusterStorageSync starts periodic synchronization of storage clients
func (cm *ClusterManager) startMultiClusterStorageSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := loadMultiClusterStorageClients(ctx); err != nil {
				log.Errorf("Failed to reload multi-cluster storage clients: %v", err)
			}
		case <-ctx.Done():
			log.Info("Stopping multi-cluster storage sync")
			return
		}
	}
}
