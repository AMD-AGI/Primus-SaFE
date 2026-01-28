// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// initControlPlane initializes clients for control plane components
// Control plane components need access to the current cluster first,
// then load all remote clusters from cluster_config
func (cm *ClusterManager) initControlPlane(ctx context.Context) error {
	log.Info("Initializing control plane clients...")

	// Step 1: Initialize K8S client for current (control plane) cluster
	if cm.loadK8SClient {
		if err := initCurrentClusterK8SClientSet(ctx); err != nil {
			return err
		}
		log.Info("Control plane: K8S client initialized for current cluster")
	}

	// Step 2: Initialize Storage client for current (control plane) cluster
	// This is needed to read cluster_config from database
	if cm.loadStorageClient {
		if err := loadCurrentClusterStorageClients(ctx); err != nil {
			return err
		}
		log.Info("Control plane: Storage client initialized for current cluster")
	}

	// Step 3: Initialize current cluster info
	if cm.loadK8SClient || cm.loadStorageClient {
		if err := cm.initializeCurrentCluster(); err != nil {
			return err
		}
	}

	// Step 4: Load multi-cluster clients
	if err := cm.initMultiCluster(ctx); err != nil {
		log.Warnf("Control plane: Failed to initialize multi-cluster clients: %v", err)
		// Don't return error as multi-cluster config may not be ready yet
	}

	log.Infof("Control plane initialization completed, total clusters: %d", cm.GetClusterCount())
	return nil
}
