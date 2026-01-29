// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package jobs provides data plane job scheduling.
// This package contains jobs that run in data plane clusters for data collection.
//
// NOTE: Management jobs have been migrated to control-plane-controller module:
// - dataplane_installer -> control-plane-controller
// - multi_cluster_config_sync -> control-plane-controller
// - tracelens_cleanup -> control-plane-controller
// - gpu_usage_weekly_report -> control-plane-controller
package jobs

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/action_task_executor"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/cluster_overview"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/device_info"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_aggregation"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_aggregation_backfill"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_allocation"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_consumers"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_history_cache_1h"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_history_cache_24h"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_history_cache_6h"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_pod"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_realtime_cache"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/pyspy_task_dispatcher"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/stale_pod_cleanup"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/storage_scan"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/workload_statistic"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/workload_stats_backfill"
)

// Job interface defines the contract for all job implementations
type Job interface {
	Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error)
	Schedule() string
}

// JobRegistry manages job registration
type JobRegistry struct {
	jobs []Job
}

// Global job registry instance
var registry *JobRegistry

// InitJobs initializes data plane jobs
// NOTE: Management jobs have been migrated to control-plane-controller module
func InitJobs(cfg *config.JobsConfig) {
	registry = &JobRegistry{}
	registry.jobs = initDataPlaneJobs()

	log.Infof("Data plane jobs initialized: %d jobs", len(registry.jobs))
}

// initDataPlaneJobs initializes all data plane jobs
func initDataPlaneJobs() []Job {
	jobs := []Job{
		// Core GPU metrics collection
		&gpu_allocation.GpuAllocationJob{},
		&gpu_consumers.GpuConsumersJob{},
		&device_info.DeviceInfoJob{},
		&gpu_workload.GpuWorkloadJob{},
		&gpu_pod.GpuPodJob{},
		&storage_scan.StorageScanJob{},
		&cluster_overview.ClusterOverviewJob{},

		// GPU cache jobs - split into separate jobs for better performance
		&gpu_realtime_cache.GpuRealtimeCacheJob{},      // Every 30s - realtime metrics
		&gpu_history_cache_1h.GpuHistoryCache1hJob{},   // Every 1m - 1 hour history
		&gpu_history_cache_6h.GpuHistoryCache6hJob{},   // Every 5m - 6 hour history
		&gpu_history_cache_24h.GpuHistoryCache24hJob{}, // Every 10m - 24 hour history

		// GPU aggregation jobs
		gpu_aggregation.NewClusterGpuAggregationJob(),   // Every 5m - cluster-level
		gpu_aggregation.NewNamespaceGpuAggregationJob(), // Every 5m - namespace-level
		gpu_aggregation.NewWorkloadGpuAggregationJob(),  // Every 5m - workload-level
		gpu_aggregation.NewLabelGpuAggregationJob(),     // Every 5m - label/annotation-level

		// Backfill jobs
		gpu_aggregation_backfill.NewClusterGpuAggregationBackfillJob(),   // Every 5m
		gpu_aggregation_backfill.NewNamespaceGpuAggregationBackfillJob(), // Every 5m
		gpu_aggregation_backfill.NewLabelGpuAggregationBackfillJob(),     // Every 5m

		// Workload statistics
		workload_statistic.NewWorkloadStatisticJob(),          // Every 30s
		workload_stats_backfill.NewWorkloadStatsBackfillJob(), // Every 10m
	}

	// ActionTaskExecutor for cross-cluster action handling
	// Polls every 300ms to achieve <1s latency for task pickup
	jobs = append(jobs, action_task_executor.NewActionTaskExecutorJob())
	log.Info("Action task executor job registered (poll interval: 300ms)")

	// StalePodCleanupJob to clean up stale "Running" pods
	jobs = append(jobs, stale_pod_cleanup.NewStalePodCleanupJob())
	log.Info("Stale pod cleanup job registered (every 5m)")

	// Py-Spy task dispatcher - runs in data plane
	jobs = append(jobs, pyspy_task_dispatcher.NewPySpyTaskDispatcherJob())
	log.Info("Py-Spy task dispatcher job registered (every 5s)")

	return jobs
}

// GetJobs returns the list of jobs to run
func GetJobs() []Job {
	if registry == nil {
		log.Warn("Job registry not initialized, returning empty job list")
		return []Job{}
	}
	return registry.jobs
}
