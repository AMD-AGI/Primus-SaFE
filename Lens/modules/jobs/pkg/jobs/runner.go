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

// Start initializes and starts all jobs based on configuration
func Start(ctx context.Context, cfg *config.JobsConfig) error {
	// Initialize job registry with configuration
	InitJobs(cfg)

	// Get jobs based on current mode
	jobsToRun := GetJobs()

	if len(jobsToRun) == 0 {
		log.Warn("No jobs to run, service will continue without scheduled tasks")
		return nil
	}

	// Use SkipIfStillRunning to prevent concurrent execution of the same job
	// If a job is still running when the next scheduled time arrives, the new execution will be skipped
	c := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(cron.DefaultLogger),
	))
	cm := clientsets.GetClusterManager()
	currentCluster := cm.GetCurrentClusterClients()

	// Register all jobs with metrics collection
	for _, job := range jobsToRun {
		// Capture job variable for closure
		jobToRun := job
		jobName := getJobName(jobToRun)

		_, err := c.AddFunc(job.Schedule(), func() {
			runJobWithMetrics(ctx, jobToRun, currentCluster.K8SClientSet, currentCluster.StorageClientSet)
		})

		if err != nil {
			log.Errorf("Failed to schedule job %s: %v", jobName, err)
			return err
		}

		log.Infof("Registered job: %s with schedule: %s (mode: %s)", jobName, job.Schedule(), GetMode())
	}

	c.Start()
	log.Infof("Started %d jobs in %s mode", len(jobsToRun), GetMode())
	return nil
}
