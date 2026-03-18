// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package jobs

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/robfig/cron/v3"
)

// Start initializes and starts all data plane jobs
// NOTE: Management jobs have been migrated to control-plane-controller module
func Start(ctx context.Context, cfg *config.JobsConfig) error {
	// Initialize job registry with configuration
	InitJobs(cfg)

	// Get jobs to run
	jobsToRun := GetJobs()

	if len(jobsToRun) == 0 {
		log.Warn("No jobs to run, service will continue without scheduled tasks")
		return nil
	}

	// Use SkipIfStillRunning to prevent concurrent execution of the same job
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	cm := clientsets.GetClusterManager()
	currentCluster := cm.GetCurrentClusterClients()

	// Check if current cluster is available
	if currentCluster == nil {
		log.Warn("Current cluster not initialized. Jobs will be skipped until cluster is configured.")
	}

	// Register all jobs with metrics collection
	for _, job := range jobsToRun {
		jobToRun := job
		jobName := getJobName(jobToRun)

		_, err := c.AddFunc(job.Schedule(), func() {
			// Re-fetch current cluster each time as it may be initialized later
			cluster := cm.GetCurrentClusterClients()
			if cluster == nil {
				log.Warnf("Skipping job %s: current cluster not initialized", getJobName(jobToRun))
				return
			}
			runJobWithMetrics(ctx, jobToRun, cluster.K8SClientSet, cluster.StorageClientSet)
		})

		if err != nil {
			log.Errorf("Failed to schedule job %s: %v", jobName, err)
			return err
		}

		log.Infof("Registered job: %s with schedule: %s", jobName, job.Schedule())
	}

	c.Start()
	log.Infof("Started %d data plane jobs", len(jobsToRun))
	return nil
}
