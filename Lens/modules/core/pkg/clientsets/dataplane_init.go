// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package clientsets

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// initDataPlane initializes clients for data plane components
// Data plane components only need access to the current cluster
func (cm *ClusterManager) initDataPlane(ctx context.Context) error {
	log.Info("Initializing data plane clients...")

	// Initialize K8S client for current cluster if required
	if cm.loadK8SClient {
		if err := initCurrentClusterK8SClientSet(ctx); err != nil {
			return err
		}
		log.Info("Data plane: K8S client initialized for current cluster")
	}

	// Initialize Storage client for current cluster if required
	if cm.loadStorageClient {
		if err := loadCurrentClusterStorageClients(ctx); err != nil {
			return err
		}
		log.Info("Data plane: Storage client initialized for current cluster")
	}

	// Initialize current cluster info
	if cm.loadK8SClient || cm.loadStorageClient {
		if err := cm.initializeCurrentCluster(); err != nil {
			return err
		}
	}

	log.Infof("Data plane initialization completed for cluster: %s", cm.GetCurrentClusterName())
	return nil
}
