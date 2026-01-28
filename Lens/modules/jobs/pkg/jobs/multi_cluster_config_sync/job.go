// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package multi_cluster_config_sync

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
)

// MultiClusterConfigSyncJob syncs storage configs from all clusters in control plane DB
// and creates proxy services for cross-cluster access.
// This replaces the multi-cluster-config-exporter component.
type MultiClusterConfigSyncJob struct{}

// NewMultiClusterConfigSyncJob creates a new MultiClusterConfigSyncJob
func NewMultiClusterConfigSyncJob() *MultiClusterConfigSyncJob {
	return &MultiClusterConfigSyncJob{}
}

// Run executes the multi-cluster config sync job
func (j *MultiClusterConfigSyncJob) Run(ctx context.Context, clientSet *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Only run in control plane mode
	if !clientsets.IsControlPlaneMode() {
		log.Debug("MultiClusterConfigSyncJob: skipping - not in control plane mode")
		return &common.ExecutionStats{
			JobName:    "MultiClusterConfigSyncJob",
			Success:    true,
			SkipReason: "not in control plane mode",
		}, nil
	}

	log.Info("MultiClusterConfigSyncJob: starting multi-cluster config sync")

	syncer := NewConfigSyncer()
	if err := syncer.Initialize(ctx); err != nil {
		log.Errorf("MultiClusterConfigSyncJob: failed to initialize syncer: %v", err)
		return &common.ExecutionStats{
			JobName: "MultiClusterConfigSyncJob",
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	stats, err := syncer.SyncAll(ctx)
	if err != nil {
		log.Errorf("MultiClusterConfigSyncJob: sync failed: %v", err)
		return &common.ExecutionStats{
			JobName: "MultiClusterConfigSyncJob",
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	log.Infof("MultiClusterConfigSyncJob: completed - synced %d clusters, created %d proxy services",
		stats.ClustersProcessed, stats.ProxyServicesCreated)

	return &common.ExecutionStats{
		JobName: "MultiClusterConfigSyncJob",
		Success: true,
	}, nil
}

// Schedule returns the job schedule (every 30 seconds)
func (j *MultiClusterConfigSyncJob) Schedule() string {
	return "@every 30s"
}
