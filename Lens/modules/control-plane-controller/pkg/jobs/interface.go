// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package jobs provides control plane specific job scheduling.
// This package contains jobs that run in the control plane cluster.
//
// Jobs are categorized into two types:
// 1. Pure CP Jobs - Only need CP database + K8S client (no data plane storage)
// 2. Multi-Cluster Jobs - Need access to multiple clusters' data plane storage
package jobs

import (
	"context"
	"reflect"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/control-plane-controller/pkg/jobs/dataplane_installer"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/control-plane-controller/pkg/jobs/gpu_usage_weekly_report"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/control-plane-controller/pkg/jobs/multi_cluster_config_sync"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/control-plane-controller/pkg/jobs/tracelens_cleanup"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Job interface defines the contract for control plane jobs
type Job interface {
	Run(ctx context.Context, k8sClient *clientsets.K8SClientSet, storageClient *clientsets.StorageClientSet) (*common.ExecutionStats, error)
	Schedule() string
}

// Metrics for job execution
var (
	jobExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cp_controller_job_duration_seconds",
			Help:    "Duration of control plane job execution",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		[]string{"job_name"},
	)

	jobExecutionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cp_controller_job_executions_total",
			Help: "Total number of control plane job executions",
		},
		[]string{"job_name", "status"},
	)
)

// registeredJobs contains all control plane jobs
var registeredJobs []Job

// jobsConfig stores the configuration for jobs that need it
var jobsConfig *config.JobsConfig

// InitJobs initializes control plane jobs with configuration
func InitJobs(cfg *config.JobsConfig) {
	jobsConfig = cfg
	registeredJobs = []Job{}

	// ========================================
	// Pure CP Jobs (only need CP DB + K8S)
	// These jobs do NOT require data plane storage
	// ========================================

	// Dataplane Installer - manages dataplane deployment tasks from CP database
	// Dependencies: CP Database, K8S client (for creating installer Jobs)
	registeredJobs = append(registeredJobs, dataplane_installer.NewDataplaneInstallerJob())
	log.Info("Registered: DataplaneInstallerJob (pure CP job)")

	// Multi-Cluster Config Sync - syncs cluster configs and creates proxy services
	// Dependencies: CP Database, K8S client (for creating Services/Endpoints)
	registeredJobs = append(registeredJobs, multi_cluster_config_sync.NewMultiClusterConfigSyncJob())
	log.Info("Registered: MultiClusterConfigSyncJob (pure CP job)")

	// ========================================
	// Multi-Cluster Jobs (need multi-cluster storage access)
	// These jobs access data plane databases across clusters
	// They will gracefully skip if storage is not available
	// ========================================

	// TraceLens Cleanup - cleans up expired sessions across all clusters
	// Dependencies: Multi-cluster data plane DB (for session data), K8S client (for pod deletion)
	registeredJobs = append(registeredJobs, tracelens_cleanup.NewTraceLensCleanupJob())
	log.Info("Registered: TraceLensCleanupJob (multi-cluster job)")

	// GPU Usage Weekly Report - generates reports for all clusters
	// Dependencies: Multi-cluster data plane DB (for GPU stats), configuration
	if cfg != nil {
		registeredJobs = append(registeredJobs, gpu_usage_weekly_report.NewGpuUsageWeeklyReportJob(cfg.WeeklyReport))
		log.Info("Registered: GpuUsageWeeklyReportJob (multi-cluster job)")

		// Weekly report backfill job
		backfillConfig := &gpu_usage_weekly_report.GpuUsageWeeklyReportBackfillConfig{
			Enabled:            cfg.WeeklyReport != nil && cfg.WeeklyReport.Enabled,
			MaxWeeksToBackfill: 0, // No limit
			WeeklyReportConfig: cfg.WeeklyReport,
		}
		if cfg.WeeklyReportBackfill != nil {
			backfillConfig.Enabled = cfg.WeeklyReportBackfill.Enabled
			backfillConfig.Cron = cfg.WeeklyReportBackfill.Cron
			if cfg.WeeklyReportBackfill.MaxWeeksToBackfill > 0 {
				backfillConfig.MaxWeeksToBackfill = cfg.WeeklyReportBackfill.MaxWeeksToBackfill
			}
		}
		registeredJobs = append(registeredJobs, gpu_usage_weekly_report.NewGpuUsageWeeklyReportBackfillJob(backfillConfig))
		log.Info("Registered: GpuUsageWeeklyReportBackfillJob (multi-cluster job)")
	}

	log.Infof("Control plane controller: %d jobs registered", len(registeredJobs))
	for _, job := range registeredJobs {
		log.Infof("  - %s (schedule: %s)", getJobName(job), job.Schedule())
	}
}

// GetJobs returns all registered jobs
func GetJobs() []Job {
	return registeredJobs
}

// getJobName extracts the job name from its type
func getJobName(job Job) string {
	t := reflect.TypeOf(job)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// runJobWithMetrics executes a job and records metrics
func runJobWithMetrics(ctx context.Context, job Job, k8sClient *clientsets.K8SClientSet, storageClient *clientsets.StorageClientSet) {
	jobName := getJobName(job)
	start := time.Now()

	log.Debugf("Starting job: %s", jobName)

	// Execute job
	stats, err := job.Run(ctx, k8sClient, storageClient)

	duration := time.Since(start)
	jobExecutionDuration.WithLabelValues(jobName).Observe(duration.Seconds())

	if err != nil {
		jobExecutionTotal.WithLabelValues(jobName, "error").Inc()
		log.Errorf("Job %s failed after %v: %v", jobName, duration, err)
	} else {
		jobExecutionTotal.WithLabelValues(jobName, "success").Inc()
		if stats != nil && len(stats.Messages) > 0 {
			log.Infof("Job %s completed in %v: %s", jobName, duration, stats.Messages[0])
		} else {
			log.Debugf("Job %s completed in %v", jobName, duration)
		}
	}
}
