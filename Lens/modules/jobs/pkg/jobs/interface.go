// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package jobs

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
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
	// NOTE: All GitHub workflow related jobs have been migrated to github-runners-exporter:
	// - github_runner_scanner -> replaced by EphemeralRunnerReconciler
	// - github_workflow_scanner -> replaced by EphemeralRunnerReconciler
	// - github_workflow_collector -> replaced by CollectionExecutor (TaskScheduler)
	// - github_workflow_backfill -> replaced by WorkflowBackfillRunner
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_usage_weekly_report"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/action_task_executor"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/dataplane_installer"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/multi_cluster_config_sync"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/pyspy_task_dispatcher"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/stale_pod_cleanup"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/storage_scan"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/tracelens_cleanup"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/workload_statistic"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/workload_stats_backfill"
)

// JobMode defines the running mode of jobs service
type JobMode string

const (
	JobModeData       JobMode = "data"       // Data cluster mode: only run data collection jobs
	JobModeManagement JobMode = "management" // Management cluster mode: only run management jobs
	JobModeStandalone JobMode = "standalone" // Standalone mode: run all jobs (default)
)

// Job interface defines the contract for all job implementations
type Job interface {
	Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error)
	Schedule() string
}

// JobRegistry manages job registration and filtering based on running mode
type JobRegistry struct {
	dataJobs       []Job
	managementJobs []Job
	mode           JobMode
}

// Global job registry instance
var registry *JobRegistry

// InitJobs initializes jobs based on configuration mode
func InitJobs(cfg *config.JobsConfig) {
	mode := JobModeStandalone // default mode
	if cfg != nil && cfg.Mode != "" {
		mode = JobMode(cfg.Mode)
	}

	registry = &JobRegistry{
		mode: mode,
	}

	// Initialize jobs based on mode
	switch mode {
	case JobModeData:
		// Data mode: only initialize data collection jobs
		registry.dataJobs = initDataJobs()
		registry.managementJobs = []Job{}

	case JobModeManagement:
		// Management mode: only initialize management jobs
		registry.dataJobs = []Job{}
		registry.managementJobs = initManagementJobs(cfg)

	case JobModeStandalone:
		// Standalone mode: initialize both
		registry.dataJobs = initDataJobs()
		registry.managementJobs = initManagementJobs(cfg)

	default:
		// Unknown mode, default to standalone
		log.Warnf("Unknown job mode: %s, defaulting to standalone", mode)
		registry.dataJobs = initDataJobs()
		registry.managementJobs = initManagementJobs(cfg)
	}

	log.Infof("Jobs initialized in mode: %s", mode)
	log.Infof("Data jobs count: %d", len(registry.dataJobs))
	log.Infof("Management jobs count: %d", len(registry.managementJobs))
}

// initDataJobs initializes all data collection jobs
func initDataJobs() []Job {
	jobs := []Job{
		&gpu_allocation.GpuAllocationJob{},
		&gpu_consumers.GpuConsumersJob{},
		&device_info.DeviceInfoJob{},
		&gpu_workload.GpuWorkloadJob{},
		&gpu_pod.GpuPodJob{},
		&storage_scan.StorageScanJob{},
		&cluster_overview.ClusterOverviewJob{},
		// GPU cache jobs - split into separate jobs for better performance
		&gpu_realtime_cache.GpuRealtimeCacheJob{},                        // Every 30s - realtime metrics
		&gpu_history_cache_1h.GpuHistoryCache1hJob{},                     // Every 1m - 1 hour history
		&gpu_history_cache_6h.GpuHistoryCache6hJob{},                     // Every 5m - 6 hour history
		&gpu_history_cache_24h.GpuHistoryCache24hJob{},                   // Every 10m - 24 hour history
		gpu_aggregation.NewClusterGpuAggregationJob(),                    // Every 5m - cluster-level GPU aggregation
		gpu_aggregation.NewNamespaceGpuAggregationJob(),                  // Every 5m - namespace-level GPU aggregation
		gpu_aggregation.NewWorkloadGpuAggregationJob(),                   // Every 5m - workload-level GPU aggregation
		gpu_aggregation.NewLabelGpuAggregationJob(),                      // Every 5m - label/annotation-level GPU aggregation
		gpu_aggregation_backfill.NewClusterGpuAggregationBackfillJob(),   // Every 5m - backfill missing cluster aggregation data
		gpu_aggregation_backfill.NewNamespaceGpuAggregationBackfillJob(), // Every 5m - backfill missing namespace aggregation data
		gpu_aggregation_backfill.NewLabelGpuAggregationBackfillJob(),     // Every 5m - backfill missing label aggregation data
		workload_statistic.NewWorkloadStatisticJob(),                     // Every 30s - workload GPU utilization statistics
		workload_stats_backfill.NewWorkloadStatsBackfillJob(),            // Every 10m - backfill missing workload hourly stats
	}

	// Add ActionTaskExecutor for cross-cluster action handling
	// Polls every 300ms to achieve <1s latency for task pickup
	jobs = append(jobs, action_task_executor.NewActionTaskExecutorJob())
	log.Info("Action task executor job registered (poll interval: 300ms)")

	// Add StalePodCleanupJob to clean up stale "Running" pods that no longer exist in K8s
	// This handles cases where exporter's reconcile loop misses pod deletion events
	jobs = append(jobs, stale_pod_cleanup.NewStalePodCleanupJob())
	log.Info("Stale pod cleanup job registered (every 5m)")

	// Add Py-Spy task dispatcher job - runs in data plane to dispatch profiling tasks
	// This needs access to local workload_task_state table and node-exporter
	jobs = append(jobs, pyspy_task_dispatcher.NewPySpyTaskDispatcherJob())
	log.Info("Py-Spy task dispatcher job registered (data plane, every 5s)")

	return jobs
}

// initManagementJobs initializes all management jobs
func initManagementJobs(cfg *config.JobsConfig) []Job {
	var jobs []Job

	// Add TraceLens cleanup job - runs every 5 minutes to clean up expired sessions
	jobs = append(jobs, tracelens_cleanup.NewTraceLensCleanupJob())
	log.Info("TraceLens cleanup job registered")

	// NOTE: All GitHub workflow related jobs have been migrated to github-runners-exporter:
	// - EphemeralRunnerReconciler: real-time discovery of workflow runs
	// - CollectionExecutor (TaskScheduler): real-time metrics collection
	// - WorkflowBackfillRunner: historical data backfill
	// See: github-runners-exporter/pkg/{reconciler,executor,collector,backfill}/

	// Add weekly report job if configured
	if cfg != nil {
		jobs = append(jobs, gpu_usage_weekly_report.NewGpuUsageWeeklyReportJob(cfg.WeeklyReport))
		log.Info("Weekly report job registered")

		// Add weekly report backfill job
		backfillConfig := &gpu_usage_weekly_report.GpuUsageWeeklyReportBackfillConfig{
			Enabled:            cfg.WeeklyReport != nil && cfg.WeeklyReport.Enabled,
			MaxWeeksToBackfill: 0, // No limit
			WeeklyReportConfig: cfg.WeeklyReport,
		}
		// Apply backfill-specific config if available
		if cfg.WeeklyReportBackfill != nil {
			backfillConfig.Enabled = cfg.WeeklyReportBackfill.Enabled
			backfillConfig.Cron = cfg.WeeklyReportBackfill.Cron
			if cfg.WeeklyReportBackfill.MaxWeeksToBackfill > 0 {
				backfillConfig.MaxWeeksToBackfill = cfg.WeeklyReportBackfill.MaxWeeksToBackfill
			}
		}
		jobs = append(jobs, gpu_usage_weekly_report.NewGpuUsageWeeklyReportBackfillJob(backfillConfig))
		log.Info("Weekly report backfill job registered")
	}

	// Add Dataplane Installer job - only runs in control plane mode
	// Polls every 10s to execute dataplane installation tasks
	jobs = append(jobs, dataplane_installer.NewDataplaneInstallerJob())
	log.Info("Dataplane installer job registered (control plane mode only)")

	// Add Multi-Cluster Config Sync job - only runs in control plane mode
	// Syncs storage configs and creates proxy services every 30s
	// Replaces multi-cluster-config-exporter component
	jobs = append(jobs, multi_cluster_config_sync.NewMultiClusterConfigSyncJob())
	log.Info("Multi-cluster config sync job registered (control plane mode only, every 30s)")

	// Add more management jobs here in the future
	// e.g., report distributor, cleanup jobs, etc.

	return jobs
}

// GetJobs returns the list of jobs to run based on the current mode
func GetJobs() []Job {
	if registry == nil {
		log.Warn("Job registry not initialized, returning empty job list")
		return []Job{}
	}

	switch registry.mode {
	case JobModeData:
		// Data mode: only run data collection jobs
		log.Debugf("Running in data mode: %d jobs", len(registry.dataJobs))
		return registry.dataJobs

	case JobModeManagement:
		// Management mode: only run management jobs
		log.Debugf("Running in management mode: %d jobs", len(registry.managementJobs))
		return registry.managementJobs

	case JobModeStandalone:
		// Standalone mode: run all jobs
		allJobs := append([]Job{}, registry.dataJobs...)
		allJobs = append(allJobs, registry.managementJobs...)
		log.Debugf("Running in standalone mode: %d jobs", len(allJobs))
		return allJobs

	default:
		// Unknown mode, default to standalone
		log.Warnf("Unknown job mode: %s, defaulting to standalone", registry.mode)
		allJobs := append([]Job{}, registry.dataJobs...)
		allJobs = append(allJobs, registry.managementJobs...)
		return allJobs
	}
}

// GetMode returns the current running mode
func GetMode() JobMode {
	if registry == nil {
		return JobModeStandalone
	}
	return registry.mode
}
