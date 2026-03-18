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
		stats := common.NewExecutionStats()
		stats.AddMessage("skipped: not in control plane mode")
		return stats, nil
	}

	log.Info("MultiClusterConfigSyncJob: starting multi-cluster config sync")

	syncer := NewConfigSyncer()
	if err := syncer.Initialize(ctx); err != nil {
		log.Errorf("MultiClusterConfigSyncJob: failed to initialize syncer: %v", err)
		stats := common.NewExecutionStats()
		stats.ErrorCount = 1
		stats.AddMessage("failed to initialize syncer: " + err.Error())
		return stats, err
	}

	syncStats, err := syncer.SyncAll(ctx)
	if err != nil {
		log.Errorf("MultiClusterConfigSyncJob: sync failed: %v", err)
		stats := common.NewExecutionStats()
		stats.ErrorCount = 1
		stats.AddMessage("sync failed: " + err.Error())
		return stats, err
	}

	log.Infof("MultiClusterConfigSyncJob: completed - synced %d clusters, created %d proxy services",
		syncStats.ClustersProcessed, syncStats.ProxyServicesCreated)

	stats := common.NewExecutionStats()
	stats.RecordsProcessed = int64(syncStats.ClustersProcessed)
	stats.ItemsCreated = int64(syncStats.ProxyServicesCreated)
	stats.AddCustomMetric("clusters_processed", syncStats.ClustersProcessed)
	stats.AddCustomMetric("proxy_services_created", syncStats.ProxyServicesCreated)
	return stats, nil
}

// Schedule returns the job schedule (every 30 seconds)
func (j *MultiClusterConfigSyncJob) Schedule() string {
	return "@every 30s"
}
