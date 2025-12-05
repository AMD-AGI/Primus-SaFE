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
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_usage_weekly_report"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/gpu_workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/storage_scan"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs/workload_statistic"
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
	return []Job{
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
		gpu_aggregation.NewGpuAggregationJob(),
		gpu_aggregation_backfill.NewGpuAggregationBackfillJob(), // Every hour at :30 - backfill missing aggregation data
		workload_statistic.NewWorkloadStatisticJob(),            // Every 30s - workload GPU utilization statistics
	}
}

// initManagementJobs initializes all management jobs
func initManagementJobs(cfg *config.JobsConfig) []Job {
	var jobs []Job

	// Add weekly report job if configured
	if cfg != nil && cfg.WeeklyReport != nil && cfg.WeeklyReport.Enabled {
		jobs = append(jobs, gpu_usage_weekly_report.NewGpuUsageWeeklyReportJob(cfg.WeeklyReport))
		log.Info("Weekly report job registered")
	}

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
